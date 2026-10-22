package service

import (
	"sort"

	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

// PoolPricingGap is one billable spec of a pool model without a usable rule.
type PoolPricingGap struct {
	Model          string `json:"model"`
	Modality       string `json:"modality"`
	Operation      string `json:"operation"`
	Unit           string `json:"unit"`
	ResolutionTier string `json:"resolutionTier"`
	Draft          bool   `json:"draft"` // a draft rule exists and awaits a price
	// Providers serving this spec. Informational only: the price is per
	// logical model + spec and the same for every provider.
	Providers []string `json:"providers"`
}

const poolDraftRemark = "账号池草稿：请填写价格后启用"

// poolPricingSpecs lists the specs 幻图 bills for each pool model, matching
// the PricingRequest built for each request path.
func poolPricingSpecs(aliases []model.PoolModelAlias) []PoolPricingGap {
	channel, ok := builtinPoolChannel(aliases)
	if !ok {
		return nil
	}
	routes := map[string][]gateway.PoolRoute{}
	for _, item := range builtinPoolModels() {
		routes[item.ID] = item.Routes
	}
	// Aliases are priced under their public name and served by the target.
	for _, item := range channel.Models {
		if item.UpstreamModel != "" && item.UpstreamModel != item.Model {
			routes[item.Model] = routes[item.UpstreamModel]
		}
	}
	var specs []PoolPricingGap
	defer func() {
		for i := range specs {
			specs[i].Providers = specProviders(routes[specs[i].Model], specs[i])
		}
	}()
	for _, item := range channel.Models {
		specs = append(specs, channelModelPricingSpecs(item)...)
	}
	return specs
}

// channelModelPricingSpecs lists the specs billed for one source model.
func channelModelPricingSpecs(item model.ChannelModel) []PoolPricingGap {
	var specs []PoolPricingGap
	switch item.Modality {
	case "image":
		for _, operation := range []string{"generation", "edit"} {
			if !containsString(item.Operations, operation) {
				continue
			}
			for _, tier := range item.ResolutionTiers {
				specs = append(specs, PoolPricingGap{Model: item.Model, Modality: "image", Operation: operation, Unit: "image", ResolutionTier: normalizeResolutionTier(tier)})
			}
		}
	case "video":
		for _, tier := range item.ResolutionTiers {
			specs = append(specs, PoolPricingGap{Model: item.Model, Modality: "video", Operation: "generation", Unit: "second", ResolutionTier: normalizeResolutionTier(tier)})
		}
	case "text":
		specs = append(specs, PoolPricingGap{Model: item.Model, Modality: "text", Operation: "completion", Unit: "request"})
	}
	return specs
}

// specProviders lists the providers whose route supports the spec.
func specProviders(routes []gateway.PoolRoute, spec PoolPricingGap) []string {
	providers := []string{}
	for _, route := range routes {
		if len(route.Operations) > 0 && !containsString(route.Operations, spec.Operation) {
			continue
		}
		if spec.ResolutionTier != "" {
			supported := false
			for _, tier := range route.ResolutionTiers {
				supported = supported || normalizeResolutionTier(tier) == spec.ResolutionTier
			}
			if !supported {
				continue
			}
		}
		if !containsString(providers, route.Provider) {
			providers = append(providers, route.Provider)
		}
	}
	sort.Strings(providers)
	return providers
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func poolPricingGaps(rules []model.PricingRule, aliases []model.PoolModelAlias) []PoolPricingGap {
	priced := map[string]bool{}
	drafted := map[string]bool{}
	for _, rule := range rules {
		key := pricingSpecKey(rule.Model, rule.Modality, rule.Operation, rule.Unit, rule.ResolutionTier)
		if rule.Draft {
			drafted[key] = true
		} else if rule.Enabled {
			priced[key] = true
		}
	}
	gaps := []PoolPricingGap{}
	seen := map[string]bool{}
	for _, spec := range poolPricingSpecs(aliases) {
		key := pricingSpecKey(spec.Model, spec.Modality, spec.Operation, spec.Unit, spec.ResolutionTier)
		if priced[key] || seen[key] {
			continue
		}
		seen[key] = true
		spec.Draft = drafted[key]
		gaps = append(gaps, spec)
	}
	sort.SliceStable(gaps, func(i, j int) bool { return gaps[i].Model < gaps[j].Model })
	return gaps
}

// PoolPricingGaps lists pool model specs that cannot be billed yet.
func PoolPricingGaps() ([]PoolPricingGap, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return nil, err
	}
	return poolPricingGaps(normalizePricingRules(settings.Public.ModelChannel.PricingRules), settings.Private.PoolModelAliases), nil
}

// CreatePoolPricingDrafts adds a disabled draft rule (no price) for every
// unpriced pool spec that has no rule yet. Drafts are never billed.
func CreatePoolPricingDrafts() (int, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return 0, err
	}
	rules := normalizePricingRules(settings.Public.ModelChannel.PricingRules)
	existing := map[string]bool{}
	for _, rule := range normalizePricingRules(rules) {
		existing[pricingSpecKey(rule.Model, rule.Modality, rule.Operation, rule.Unit, rule.ResolutionTier)] = true
	}
	created := 0
	for _, gap := range poolPricingGaps(normalizePricingRules(rules), settings.Private.PoolModelAliases) {
		key := pricingSpecKey(gap.Model, gap.Modality, gap.Operation, gap.Unit, gap.ResolutionTier)
		if gap.Draft || existing[key] {
			continue
		}
		rules = append(rules, model.PricingRule{Model: gap.Model, Modality: gap.Modality, Operation: gap.Operation, Unit: gap.Unit,
			ResolutionTier: gap.ResolutionTier, BillingMode: "fixed", Draft: true, Enabled: false, Remark: poolDraftRemark})
		existing[key] = true
		created++
	}
	if created == 0 {
		return 0, nil
	}
	settings.Public.ModelChannel.PricingRules = rules
	if _, err := SaveSettings(settings); err != nil {
		return 0, err
	}
	return created, nil
}
