package service

import (
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

const MaxUserPricingDiscounts = 500

type PricingSource string

const (
	PricingSourceUserSpec PricingSource = "user_spec"
	PricingSourceGroup    PricingSource = "group"
	PricingSourceDefault  PricingSource = "default"
)

type PricingResult struct {
	Credits        int                   `json:"credits"`
	EffectiveRatio float64               `json:"effectiveRatio"`
	Source         PricingSource         `json:"source"`
	Snapshot       model.PricingSnapshot `json:"snapshot"`
}

// PricingResolver keeps the settings and user discounts used by one request together.
type PricingResolver struct {
	modelChannel model.PublicModelChannelSetting
	userGroup    string
	discounts    map[string]model.UserPricingDiscount
}

func NewPricingResolver(userID string, userGroup string) (*PricingResolver, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return nil, err
	}
	resolver := &PricingResolver{
		modelChannel: normalizePublicSetting(settings.Public).ModelChannel,
		userGroup:    normalizePricingToken(userGroup),
		discounts:    map[string]model.UserPricingDiscount{},
	}
	if resolver.userGroup == "" {
		resolver.userGroup = "default"
	}
	if strings.TrimSpace(userID) == "" {
		return resolver, nil
	}
	items, err := repository.ListUserPricingDiscounts(strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if validPricingRatio(item.Ratio) {
			resolver.discounts[pricingSpecKey(item.Model, item.Modality, item.Operation, item.Unit, item.ResolutionTier)] = item
		}
	}
	return resolver, nil
}

func (resolver *PricingResolver) Resolve(request PricingRequest) (PricingResult, error) {
	request = normalizePricingRequest(request)
	if request.Model == "" || request.Modality == "" || request.Operation == "" || request.Unit == "" {
		return PricingResult{}, safeMessageError{message: "模型计费参数不完整"}
	}
	rule, ok := selectPricingRule(resolver.modelChannel.PricingRules, request)
	if !ok {
		return PricingResult{}, safeMessageError{message: "该模型或当前规格未设置价格"}
	}
	groupRatio, source := resolver.groupRatio()
	effectiveRatio := groupRatio
	var userSpecRatio *float64
	key := pricingSpecKey(rule.Model, rule.Modality, rule.Operation, rule.Unit, rule.ResolutionTier)
	if discount, exists := resolver.discounts[key]; exists && validPricingRatio(discount.Ratio) {
		effectiveRatio, source = discount.Ratio, PricingSourceUserSpec
		userSpecRatio = &effectiveRatio
	}
	credits := calculateRuleCredits(rule, request.Quantity, effectiveRatio)
	if rule.MinCredits > credits {
		credits = rule.MinCredits
	}
	snapshot := model.PricingSnapshot{
		Model:           rule.Model,
		Modality:        rule.Modality,
		Operation:       rule.Operation,
		Unit:            rule.Unit,
		ResolutionTier:  rule.ResolutionTier,
		Quantity:        request.Quantity,
		BillingMode:     rule.BillingMode,
		RuleCredits:     rule.Credits,
		MinCredits:      rule.MinCredits,
		ModelRatio:      rule.ModelRatio,
		CompletionRatio: rule.CompletionRatio,
		RuleEnabled:     rule.Enabled,
		UserGroup:       resolver.userGroup,
		GroupRatio:      groupRatio,
		UserSpecRatio:   userSpecRatio,
		EffectiveRatio:  effectiveRatio,
		Source:          string(source),
		Credits:         credits,
	}
	return PricingResult{Credits: credits, EffectiveRatio: effectiveRatio, Source: source, Snapshot: snapshot}, nil
}

func (resolver *PricingResolver) Group() string {
	return resolver.userGroup
}

func (resolver *PricingResolver) BaseGroupRatio() float64 {
	ratio, _ := resolver.groupRatio()
	return ratio
}

// ResolveAll returns one quantity-one result for every enabled exact pricing specification.
func (resolver *PricingResolver) ResolveAll() ([]PricingResult, error) {
	results := make([]PricingResult, 0, len(resolver.modelChannel.PricingRules))
	seen := map[string]bool{}
	for _, rule := range resolver.modelChannel.PricingRules {
		if !rule.Enabled {
			continue
		}
		key := pricingSpecKey(rule.Model, rule.Modality, rule.Operation, rule.Unit, rule.ResolutionTier)
		if seen[key] {
			continue
		}
		seen[key] = true
		result, err := resolver.Resolve(PricingRequest{
			Model: rule.Model, Modality: rule.Modality, Operation: rule.Operation,
			Unit: rule.Unit, ResolutionTier: rule.ResolutionTier, Quantity: 1,
		})
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool {
		left, right := results[i].Snapshot, results[j].Snapshot
		return pricingSpecKey(left.Model, left.Modality, left.Operation, left.Unit, left.ResolutionTier) < pricingSpecKey(right.Model, right.Modality, right.Operation, right.Unit, right.ResolutionTier)
	})
	return results, nil
}

func (resolver *PricingResolver) groupRatio() (float64, PricingSource) {
	if resolver.userGroup != "default" {
		if ratio := resolver.modelChannel.GroupRatios[resolver.userGroup]; ratio > 0 {
			return ratio, PricingSourceGroup
		}
	}
	if ratio := resolver.modelChannel.GroupRatios["default"]; ratio > 0 {
		return ratio, PricingSourceDefault
	}
	return 1, PricingSourceDefault
}

func ListUserPricingDiscounts(userID string) ([]model.UserPricingDiscount, error) {
	if _, ok, err := repository.GetUserByID(strings.TrimSpace(userID)); err != nil {
		return nil, err
	} else if !ok {
		return nil, safeMessageError{message: "用户不存在"}
	}
	items, err := repository.ListUserPricingDiscounts(strings.TrimSpace(userID))
	if items == nil {
		items = []model.UserPricingDiscount{}
	}
	return items, err
}

func ReplaceUserPricingDiscounts(userID string, items []model.UserPricingDiscount) ([]model.UserPricingDiscount, error) {
	userID = strings.TrimSpace(userID)
	if len(items) > MaxUserPricingDiscounts {
		return nil, safeMessageError{message: "单个用户最多配置 500 条专属优惠"}
	}
	if _, ok, err := repository.GetUserByID(userID); err != nil {
		return nil, err
	} else if !ok {
		return nil, safeMessageError{message: "用户不存在"}
	}
	settings, err := repository.GetSettings()
	if err != nil {
		return nil, err
	}
	existing, err := repository.ListUserPricingDiscounts(userID)
	if err != nil {
		return nil, err
	}
	existingBySpec := make(map[string]model.UserPricingDiscount, len(existing))
	for _, item := range existing {
		existingBySpec[pricingSpecKey(item.Model, item.Modality, item.Operation, item.Unit, item.ResolutionTier)] = item
	}
	rules := normalizePublicSetting(settings.Public).ModelChannel.PricingRules
	normalized := make([]model.UserPricingDiscount, 0, len(items))
	seen := make(map[string]bool, len(items))
	timestamp := now()
	for _, item := range items {
		item.ID = newID("pricing-discount")
		item.UserID = userID
		item.Model = strings.TrimSpace(item.Model)
		item.Modality = normalizePricingToken(item.Modality)
		item.Operation = normalizePricingToken(item.Operation)
		item.Unit = normalizePricingToken(item.Unit)
		item.ResolutionTier = normalizeResolutionTier(item.ResolutionTier)
		item.Remark = strings.TrimSpace(item.Remark)
		if !validPricingRatio(item.Ratio) {
			return nil, safeMessageError{message: "专属优惠倍率必须大于 0 且不超过 1"}
		}
		if utf8.RuneCountInString(item.Remark) > 255 {
			return nil, safeMessageError{message: "专属优惠备注不能超过 255 个字符"}
		}
		key := pricingSpecKey(item.Model, item.Modality, item.Operation, item.Unit, item.ResolutionTier)
		if seen[key] {
			return nil, safeMessageError{message: "专属优惠包含重复规格"}
		}
		if !hasExactEnabledPricingRule(rules, item) {
			previous, exists := existingBySpec[key]
			if !exists || previous.Ratio != item.Ratio || previous.Remark != item.Remark {
				return nil, safeMessageError{message: "失效的专属优惠只能原样保留或删除"}
			}
		}
		seen[key] = true
		item.CreatedAt, item.UpdatedAt = timestamp, timestamp
		normalized = append(normalized, item)
	}
	return repository.ReplaceUserPricingDiscounts(userID, normalized)
}

func hasExactEnabledPricingRule(rules []model.PricingRule, item model.UserPricingDiscount) bool {
	key := pricingSpecKey(item.Model, item.Modality, item.Operation, item.Unit, item.ResolutionTier)
	for _, rule := range rules {
		if rule.Enabled && pricingSpecKey(rule.Model, rule.Modality, rule.Operation, rule.Unit, rule.ResolutionTier) == key {
			return true
		}
	}
	return false
}

func pricingSpecKey(modelName string, modality string, operation string, unit string, resolutionTier string) string {
	return strings.TrimSpace(modelName) + "\x00" + normalizePricingToken(modality) + "\x00" + normalizePricingToken(operation) + "\x00" + normalizePricingToken(unit) + "\x00" + normalizeResolutionTier(resolutionTier)
}

func validPricingRatio(ratio float64) bool {
	return ratio > 0 && ratio <= 1 && !math.IsNaN(ratio) && !math.IsInf(ratio, 0) && math.Abs(ratio*10000-math.Round(ratio*10000)) < 1e-9
}
