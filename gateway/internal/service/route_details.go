package service

import (
	"context"
	"time"

	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
)

// RouteDetail is one provider route with its effective schedule and account
// health, for the main site's unified model overview.
type RouteDetail struct {
	LogicalModelID    string
	LogicalEnabled    bool
	RouteID           string
	Provider          string
	UpstreamModel     string
	Enabled           bool
	ProviderEnabled   bool
	Priority          *int
	Weight            *int
	EffectivePriority int
	EffectiveWeight   int
	PriorityInherited bool
	WeightInherited   bool
	// BoundAccounts have an enabled, entitled binding to the route;
	// AvailableAccounts are the bound ones that are active, alive and not
	// cooling down on this route; CooldownAccounts are healthy but cooling down.
	BoundAccounts     int64
	AvailableAccounts int64
	CooldownAccounts  int64
}

type routeHealthRow struct {
	RouteID  string
	Total    int64
	Healthy  int64
	Cooldown int64
}

// RouteDetails lists every route of every logical model (enabled or not).
func (s *AdminConsoleService) RouteDetails(ctx context.Context) ([]RouteDetail, error) {
	items, err := s.models.Routes().ListLogical(ctx, false)
	if err != nil {
		return nil, err
	}
	routesByModel, err := s.models.Routes().ListRoutesByLogical(ctx, logicalModelIDs(items), false)
	if err != nil {
		return nil, err
	}
	health := s.routeHealth(ctx)
	providerState := map[string]bool{}
	out := []RouteDetail{}
	for _, item := range items {
		routes := routesByModel[item.ID]
		schedules, err := routeSchedules(ctx, s.settingsReader(), routes)
		if err != nil {
			return nil, err
		}
		for i, route := range routes {
			enabled, known := providerState[route.Provider]
			if !known {
				enabled, err = providerEnabled(ctx, s.settingsReader(), route.Provider)
				if err != nil {
					return nil, err
				}
				enabled = enabled && !isDeferredAccountProvider(route.Provider)
				providerState[route.Provider] = enabled
			}
			row := health[route.ID]
			out = append(out, RouteDetail{
				LogicalModelID: item.ID, LogicalEnabled: item.Enabled,
				RouteID: route.ID, Provider: route.Provider, UpstreamModel: route.UpstreamModel,
				Enabled: route.Enabled, ProviderEnabled: enabled,
				Priority: route.Priority, Weight: route.Weight,
				EffectivePriority: schedules[i].Priority, EffectiveWeight: schedules[i].Weight,
				PriorityInherited: schedules[i].PriorityInherited, WeightInherited: schedules[i].WeightInherited,
				BoundAccounts: row.Total, AvailableAccounts: row.Healthy - row.Cooldown, CooldownAccounts: row.Cooldown,
			})
		}
	}
	return out, nil
}

// routeHealth counts bound, healthy and cooling-down accounts per route in
// one grouped query. Errors degrade to zero counts like routeAccountCounts.
func (s *AdminConsoleService) routeHealth(ctx context.Context) map[string]routeHealthRow {
	out := map[string]routeHealthRow{}
	if s.db == nil || !s.db.Migrator().HasTable(&model.AccountModelRoute{}) {
		return out
	}
	healthy := "provider_accounts.status = 'active' AND provider_accounts.dead = false"
	var rows []routeHealthRow
	err := s.db.WithContext(ctx).Model(&model.TokenAccount{}).
		Select("amr.model_route_id AS route_id, COUNT(*) AS total, "+
			"COUNT(*) FILTER (WHERE "+healthy+") AS healthy, "+
			"COUNT(*) FILTER (WHERE "+healthy+" AND amr.cooldown_until > ?) AS cooldown", time.Now()).
		Joins("JOIN account_model_routes amr ON amr.account_id = provider_accounts.id").
		Where("amr.enabled = ? AND amr.entitled = ?", true, true).
		Group("amr.model_route_id").
		Scan(&rows).Error
	if err != nil {
		return out
	}
	for _, row := range rows {
		out[row.RouteID] = row
	}
	return out
}
