package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

// ModelOverview is the unified admin view: every public model with its
// ordered sources, availability and pricing.
type ModelOverview struct {
	Models   []ModelOverviewItem     `json:"models"`
	Pool     PoolSourceOverview      `json:"pool"`
	Channels []ChannelSourceOverview `json:"channels"`
}

// PoolSourceOverview is the built-in pool as one source.
type PoolSourceOverview struct {
	Running         bool `json:"running"`
	Enabled         bool `json:"enabled"`
	Priority        int  `json:"priority"`
	Weight          int  `json:"weight"`
	DefaultPriority int  `json:"defaultPriority"`
	// RoutesKnown is false when the gateway route listing failed.
	RoutesKnown bool `json:"routesKnown"`
}

// ChannelSourceOverview summarises one saved third-party channel.
type ChannelSourceOverview struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Enabled  bool   `json:"enabled"`
	Priority int    `json:"priority"`
	Weight   int    `json:"weight"`
	Models   int    `json:"models"`
}

// ModelOverviewItem is one public model.
type ModelOverviewItem struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Modality   string                 `json:"modality"`
	Managed    bool                   `json:"managed"`
	Enabled    bool                   `json:"enabled"`
	Definition *model.ModelDefinition `json:"definition,omitempty"`
	Available  bool                   `json:"available"`
	Reasons    []string               `json:"reasons"`
	Warnings   []string               `json:"warnings"`
	Sources    []ModelSourceOverview  `json:"sources"`
	Pricing    []model.PricingRule    `json:"pricingRules"`
	Gaps       []PoolPricingGap       `json:"pricingGaps"`
}

// ModelSourceOverview is one source of a model in effective order.
type ModelSourceOverview struct {
	Kind             string              `json:"kind"` // channel | pool
	ChannelID        string              `json:"channelId,omitempty"`
	ChannelName      string              `json:"channelName"`
	UpstreamModel    string              `json:"upstreamModel"`
	AliasTarget      string              `json:"aliasTarget,omitempty"`
	Enabled          bool                `json:"enabled"`
	Active           bool                `json:"active"`
	Reasons          []string            `json:"reasons"`
	Priority         int                 `json:"priority"`
	Weight           int                 `json:"weight"`
	PriorityOverride *int                `json:"priorityOverride,omitempty"`
	WeightOverride   *int                `json:"weightOverride,omitempty"`
	ChannelPriority  int                 `json:"channelPriority"`
	ChannelWeight    int                 `json:"channelWeight"`
	Rank             int                 `json:"rank"` // 1 = tried first; 0 = inactive
	Capabilities     *model.ChannelModel `json:"capabilities,omitempty"`
	Routes           []PoolRouteOverview `json:"routes,omitempty"`
}

// PoolRouteOverview is one provider route under the pool source.
type PoolRouteOverview struct {
	LogicalModelID    string   `json:"logicalModelId"`
	RouteID           string   `json:"routeId"`
	Provider          string   `json:"provider"`
	UpstreamModel     string   `json:"upstreamModel"`
	Enabled           bool     `json:"enabled"`
	ProviderEnabled   bool     `json:"providerEnabled"`
	Priority          *int     `json:"priority"`
	Weight            *int     `json:"weight"`
	EffectivePriority int      `json:"effectivePriority"`
	EffectiveWeight   int      `json:"effectiveWeight"`
	BoundAccounts     int64    `json:"boundAccounts"`
	AvailableAccounts int64    `json:"availableAccounts"`
	CooldownAccounts  int64    `json:"cooldownAccounts"`
	Available         bool     `json:"available"`
	Reasons           []string `json:"reasons"`
}

// poolRouteDetails is replaceable in tests. ok=false means unknown.
var poolRouteDetails = func() ([]gateway.RouteDetail, bool) {
	m := gateway.Current()
	if m == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	items, err := m.RouteDetails(ctx)
	if err != nil {
		logPoolModelsError(err)
		return nil, false
	}
	return items, true
}

// AdminModelOverview builds the unified overview from saved settings.
func AdminModelOverview() (ModelOverview, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return ModelOverview{}, err
	}
	routes, routesKnown := poolRouteDetails()
	return buildModelOverview(normalizeSettings(settings), routes, routesKnown), nil
}

func buildModelOverview(settings model.Settings, routes []gateway.RouteDetail, routesKnown bool) ModelOverview {
	private := settings.Private
	poolEnabled, poolPriority, poolWeight := builtinPoolSourceDefaults(private.BuiltinPool)
	poolOverrides := builtinPoolModelOverrides(private.BuiltinPool)
	rawChannels := runtimeChannels(private.Channels, private.PoolModelAliases)
	var poolChannel *model.ModelChannel
	for i := range rawChannels {
		if rawChannels[i].ID == BuiltinPoolChannelID {
			poolChannel = &rawChannels[i]
		}
	}
	overview := ModelOverview{
		Pool: PoolSourceOverview{Running: gateway.Current() != nil || poolChannel != nil, Enabled: poolEnabled,
			Priority: poolPriority, Weight: poolWeight, DefaultPriority: DefaultBuiltinPoolPriority, RoutesKnown: routesKnown},
		Channels: []ChannelSourceOverview{},
		Models:   []ModelOverviewItem{},
	}
	items := map[string]*ModelOverviewItem{}
	order := []string{}
	get := func(id string) *ModelOverviewItem {
		if item, ok := items[id]; ok {
			return item
		}
		item := &ModelOverviewItem{ID: id, Name: id, Reasons: []string{}, Warnings: []string{}, Sources: []ModelSourceOverview{}, Pricing: []model.PricingRule{}, Gaps: []PoolPricingGap{}}
		items[id] = item
		order = append(order, id)
		return item
	}
	for i := range settings.Public.ModelChannel.Models {
		definition := settings.Public.ModelChannel.Models[i]
		item := get(definition.ID)
		item.Definition = &definition
		item.Managed, item.Enabled, item.Name, item.Modality = true, definition.Enabled, firstNonEmpty(definition.Name, definition.ID), definition.Modality
	}

	// Third-party channels (and a legacy saved pool channel).
	for _, channel := range private.Channels {
		overview.Channels = append(overview.Channels, ChannelSourceOverview{ID: channel.ID, Name: channel.Name, Protocol: channel.Protocol,
			Enabled: channel.Enabled, Priority: channel.Priority, Weight: channel.Weight, Models: len(channel.Models)})
		legacyPool := isBuiltinPoolChannel(channel)
		for _, cm := range channel.Models {
			capabilities := cm
			source := ModelSourceOverview{Kind: "channel", ChannelID: channel.ID, ChannelName: channel.Name, UpstreamModel: cm.UpstreamModel,
				Enabled: channel.Enabled && !cm.Disabled, PriorityOverride: cm.Priority, WeightOverride: cm.Weight,
				ChannelPriority: channel.Priority, ChannelWeight: channel.Weight, Capabilities: &capabilities, Reasons: []string{}}
			if legacyPool {
				source.ChannelPriority, source.ChannelWeight = poolPriority, poolWeight
			}
			source.Priority, source.Weight = effectiveSource(source)
			if !channel.Enabled {
				source.Reasons = append(source.Reasons, "渠道已停用")
			}
			if cm.Disabled {
				source.Reasons = append(source.Reasons, "该来源已停用")
			}
			if channel.BaseURL == "" || channel.APIKey == "" {
				source.Reasons = append(source.Reasons, "渠道未配置地址或密钥")
			}
			source.Active = len(source.Reasons) == 0
			item := get(cm.Model)
			if item.Modality == "" {
				item.Modality = cm.Modality
			}
			item.Sources = append(item.Sources, source)
		}
	}

	// The built-in pool: usable models come from the pool channel; the
	// route listing adds models the pool knows but cannot serve right now.
	routesByLogical := map[string][]gateway.RouteDetail{}
	logicalOrder := []string{}
	for _, route := range routes {
		if _, ok := routesByLogical[route.LogicalModelID]; !ok {
			logicalOrder = append(logicalOrder, route.LogicalModelID)
		}
		routesByLogical[route.LogicalModelID] = append(routesByLogical[route.LogicalModelID], route)
	}
	usable := map[string]model.ChannelModel{}
	if poolChannel != nil {
		for _, cm := range poolChannel.Models {
			usable[cm.Model] = cm
		}
	}
	poolNames := []string{}
	targets := map[string]string{}
	for _, id := range logicalOrder {
		poolNames = append(poolNames, id)
		targets[id] = id
	}
	if poolChannel != nil {
		for _, cm := range poolChannel.Models {
			if _, ok := targets[cm.Model]; !ok {
				poolNames = append(poolNames, cm.Model)
				targets[cm.Model] = cm.UpstreamModel
			}
		}
	}
	for _, alias := range private.PoolModelAliases {
		if _, ok := targets[alias.Name]; !ok && alias.Enabled {
			poolNames = append(poolNames, alias.Name)
			targets[alias.Name] = alias.Target
		}
	}
	hasLegacyPool := false
	for _, channel := range private.Channels {
		hasLegacyPool = hasLegacyPool || isBuiltinPoolChannel(channel)
	}
	for _, name := range poolNames {
		if hasLegacyPool {
			break
		}
		target := targets[name]
		override, hasOverride := poolOverrides[name]
		source := ModelSourceOverview{Kind: "pool", ChannelID: BuiltinPoolChannelID, ChannelName: BuiltinPoolChannelName, UpstreamModel: target,
			Enabled: poolEnabled && !(hasOverride && override.Disabled), ChannelPriority: poolPriority, ChannelWeight: poolWeight, Reasons: []string{}}
		if name != target {
			source.AliasTarget = target
		}
		if hasOverride {
			source.PriorityOverride, source.WeightOverride = cloneIntPtr(override.Priority), cloneIntPtr(override.Weight)
		}
		source.Priority, source.Weight = effectiveSource(source)
		if cm, ok := usable[name]; ok {
			capabilities := cm
			source.Capabilities = &capabilities
		}
		anyRoute := false
		for _, route := range routesByLogical[target] {
			row := poolRouteOverview(route)
			anyRoute = anyRoute || row.Available
			source.Routes = append(source.Routes, row)
		}
		if !poolEnabled {
			source.Reasons = append(source.Reasons, "账号池来源已停用")
		}
		if hasOverride && override.Disabled {
			source.Reasons = append(source.Reasons, "该来源已停用")
		}
		if _, ok := usable[name]; !ok {
			switch {
			case poolChannel == nil && !routesKnown:
				source.Reasons = append(source.Reasons, "账号池未运行")
			case len(routesByLogical[target]) == 0:
				source.Reasons = append(source.Reasons, "账号池中不存在目标模型 "+target)
			case !anyRoute:
				source.Reasons = append(source.Reasons, "没有可用的平台路由")
			default:
				source.Reasons = append(source.Reasons, "账号池暂未提供该模型")
			}
		}
		source.Active = len(source.Reasons) == 0
		item := get(name)
		if item.Modality == "" && source.Capabilities != nil {
			item.Modality = source.Capabilities.Modality
		}
		item.Sources = append(item.Sources, source)
	}

	// Pricing rules, gaps and availability.
	rules := normalizePricingRules(settings.Public.ModelChannel.PricingRules)
	priced := map[string]bool{}
	drafted := map[string]bool{}
	for _, rule := range rules {
		key := pricingSpecKey(rule.Model, rule.Modality, rule.Operation, rule.Unit, rule.ResolutionTier)
		if rule.Draft {
			drafted[key] = true
		} else if rule.Enabled {
			priced[key] = true
		}
		if item, ok := items[rule.Model]; ok {
			item.Pricing = append(item.Pricing, rule)
		}
	}
	public := map[string]bool{}
	for _, id := range settings.Public.ModelChannel.AvailableModels {
		public[id] = true
	}
	for _, id := range order {
		item := items[id]
		sortSources(item.Sources)
		seen := map[string]bool{}
		for _, source := range item.Sources {
			if !source.Active || source.Capabilities == nil {
				continue
			}
			capabilities := *source.Capabilities
			capabilities.Model = id
			for _, spec := range channelModelPricingSpecs(capabilities) {
				key := pricingSpecKey(spec.Model, spec.Modality, spec.Operation, spec.Unit, spec.ResolutionTier)
				if priced[key] || seen[key] {
					continue
				}
				seen[key] = true
				spec.Draft = drafted[key]
				spec.Providers = []string{}
				item.Gaps = append(item.Gaps, spec)
			}
		}
		active := false
		for _, source := range item.Sources {
			active = active || source.Active
		}
		switch {
		case !item.Managed:
			item.Reasons = append(item.Reasons, "未加入模型目录")
		case !item.Enabled:
			item.Reasons = append(item.Reasons, "模型未开放")
		}
		if len(item.Sources) == 0 {
			item.Reasons = append(item.Reasons, "没有接入来源")
		} else if !active {
			item.Reasons = append(item.Reasons, "没有可用来源")
		}
		if len(item.Gaps) > 0 {
			item.Warnings = append(item.Warnings, "存在未定价规格，相应请求无法计费")
		}
		item.Available = item.Managed && item.Enabled && active && public[id]
		overview.Models = append(overview.Models, *item)
	}
	sort.SliceStable(overview.Models, func(i, j int) bool {
		a, b := overview.Models[i], overview.Models[j]
		if a.Managed != b.Managed {
			return a.Managed
		}
		if a.Managed && a.Definition.Sort != b.Definition.Sort {
			return a.Definition.Sort < b.Definition.Sort
		}
		return a.ID < b.ID
	})
	return overview
}

func effectiveSource(source ModelSourceOverview) (priority, weight int) {
	priority, weight = source.ChannelPriority, source.ChannelWeight
	if source.PriorityOverride != nil {
		priority = *source.PriorityOverride
	}
	if source.WeightOverride != nil {
		weight = *source.WeightOverride
	}
	return
}

// sortSources orders active sources the way selection tries them (priority
// desc, then weight desc) and ranks them; inactive sources go last.
func sortSources(sources []ModelSourceOverview) {
	sort.SliceStable(sources, func(i, j int) bool {
		a, b := sources[i], sources[j]
		if a.Active != b.Active {
			return a.Active
		}
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		return a.Weight > b.Weight
	})
	rank, last := 0, -1
	for i := range sources {
		if !sources[i].Active {
			continue
		}
		if sources[i].Priority != last {
			rank++
			last = sources[i].Priority
		}
		sources[i].Rank = rank
	}
}

func poolRouteOverview(route gateway.RouteDetail) PoolRouteOverview {
	row := PoolRouteOverview{LogicalModelID: route.LogicalModelID, RouteID: route.RouteID, Provider: route.Provider, UpstreamModel: route.UpstreamModel,
		Enabled: route.Enabled, ProviderEnabled: route.ProviderEnabled, Priority: route.Priority, Weight: route.Weight,
		EffectivePriority: route.EffectivePriority, EffectiveWeight: route.EffectiveWeight,
		BoundAccounts: route.BoundAccounts, AvailableAccounts: route.AvailableAccounts, CooldownAccounts: route.CooldownAccounts, Reasons: []string{}}
	if !route.LogicalEnabled {
		row.Reasons = append(row.Reasons, "网关模型已停用")
	}
	if !route.Enabled {
		row.Reasons = append(row.Reasons, "路由已停用")
	}
	if !route.ProviderEnabled {
		row.Reasons = append(row.Reasons, "平台已关闭")
	}
	switch {
	case route.BoundAccounts == 0:
		row.Reasons = append(row.Reasons, "没有绑定账号")
	case route.AvailableAccounts <= 0 && route.CooldownAccounts > 0:
		row.Reasons = append(row.Reasons, "账号均在冷却")
	case route.AvailableAccounts <= 0:
		row.Reasons = append(row.Reasons, "没有可用账号")
	}
	row.Available = len(row.Reasons) == 0
	return row
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
