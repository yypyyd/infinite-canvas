package service

import (
	"context"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"

	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
)

// Channel (provider) scheduling order is configured in one place: the
// per-provider defaults below, overridable in site settings
// (provider.<pool>.priority / provider.<pool>.weight, edited on the account
// pool settings page). A model route inherits them unless its own nullable
// priority/weight columns are set (edited per route on the models page).
//
// Effective order for a request: higher priority first; routes with equal
// priority are shuffled by weight (weighted random without replacement);
// dispatch then fails over down that list.

const (
	routePriorityMin   = -1000
	routePriorityMax   = 1000
	routeWeightMin     = 1
	routeWeightMax     = 1000
	defaultRouteWeight = 1
)

// defaultProviderPriority applies only when no site setting is stored.
var defaultProviderPriority = map[string]int{
	"chatgpt":  100,
	"grok":     100,
	"byteplus": 90,
	"adobe":    80,
}

func providerPrioritySettingKey(pool string) string { return "provider." + pool + ".priority" }
func providerWeightSettingKey(pool string) string   { return "provider." + pool + ".weight" }

// ProviderSchedule is a provider's default route ordering.
type ProviderSchedule struct {
	Priority int
	Weight   int
}

// RouteSchedule is a route's effective ordering and whether each value is
// inherited from its provider.
type RouteSchedule struct {
	Priority          int
	Weight            int
	PriorityInherited bool
	WeightInherited   bool
}

func providerSchedule(ctx context.Context, settings settingValueReader, pool string) (ProviderSchedule, error) {
	pool = normalizePool(pool)
	out := ProviderSchedule{Priority: defaultProviderPriority[pool], Weight: defaultRouteWeight}
	if settings == nil || pool == "" {
		return out, nil
	}
	raw, err := settings.GetValue(ctx, providerPrioritySettingKey(pool))
	if err != nil {
		return out, err
	}
	if value, ok := parseBoundedInt(raw, routePriorityMin, routePriorityMax); ok {
		out.Priority = value
	}
	raw, err = settings.GetValue(ctx, providerWeightSettingKey(pool))
	if err != nil {
		return out, err
	}
	if value, ok := parseBoundedInt(raw, routeWeightMin, routeWeightMax); ok {
		out.Weight = value
	}
	return out, nil
}

func parseBoundedInt(raw string, min, max int) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < min || value > max {
		return 0, false
	}
	return value, true
}

func effectiveRouteSchedule(route model.ModelRoute, provider ProviderSchedule) RouteSchedule {
	out := RouteSchedule{Priority: provider.Priority, Weight: provider.Weight, PriorityInherited: true, WeightInherited: true}
	if route.Priority != nil {
		out.Priority, out.PriorityInherited = *route.Priority, false
	}
	if route.Weight != nil {
		out.Weight, out.WeightInherited = *route.Weight, false
	}
	if out.Weight < routeWeightMin {
		out.Weight = routeWeightMin
	}
	return out
}

// routeSchedules resolves the effective schedule of every route, reading each
// provider's settings once.
func routeSchedules(ctx context.Context, settings settingValueReader, routes []model.ModelRoute) ([]RouteSchedule, error) {
	providers := map[string]ProviderSchedule{}
	out := make([]RouteSchedule, len(routes))
	for i, route := range routes {
		provider, seen := providers[route.Provider]
		if !seen {
			var err error
			provider, err = providerSchedule(ctx, settings, route.Provider)
			if err != nil {
				return nil, err
			}
			providers[route.Provider] = provider
		}
		out[i] = effectiveRouteSchedule(route, provider)
	}
	return out, nil
}

// routeShuffle is replaceable in tests. It returns a uniform float in [0,1).
var routeShuffle = rand.Float64

var defaultRouteShuffleForTest = rand.Float64

// orderRoutes sorts routes by effective priority (descending) and shuffles
// each equal-priority group by weight.
func orderRoutes(ctx context.Context, settings settingValueReader, routes []model.ModelRoute) ([]model.ModelRoute, error) {
	if len(routes) < 2 {
		return routes, nil
	}
	schedules, err := routeSchedules(ctx, settings, routes)
	if err != nil {
		return nil, err
	}
	type entry struct {
		route    model.ModelRoute
		schedule RouteSchedule
	}
	entries := make([]entry, len(routes))
	for i := range routes {
		entries[i] = entry{routes[i], schedules[i]}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].schedule.Priority != entries[j].schedule.Priority {
			return entries[i].schedule.Priority > entries[j].schedule.Priority
		}
		return entries[i].route.ID < entries[j].route.ID
	})
	out := make([]model.ModelRoute, 0, len(entries))
	for start := 0; start < len(entries); {
		end := start + 1
		for end < len(entries) && entries[end].schedule.Priority == entries[start].schedule.Priority {
			end++
		}
		group := append([]entry(nil), entries[start:end]...)
		for len(group) > 0 {
			total := 0
			for _, item := range group {
				total += item.schedule.Weight
			}
			hit := routeShuffle() * float64(total)
			pick := len(group) - 1
			for i, item := range group {
				hit -= float64(item.schedule.Weight)
				if hit < 0 {
					pick = i
					break
				}
			}
			out = append(out, group[pick].route)
			group = append(group[:pick], group[pick+1:]...)
		}
		start = end
	}
	return out, nil
}
