package service

import (
	"context"

	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
)

func logicalModelIDs(items []model.LogicalModel) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

type routeAccountCountRow struct {
	RouteID string
	Pool    string
	Total   int64
	Healthy int64
}

type routeAccountKey struct{ route, pool string }

// routeAccountCountIndex answers per-route account counts from a single
// grouped query. When account_model_routes does not exist, counts are keyed by
// pool only, matching the previous per-route query semantics.
type routeAccountCountIndex struct {
	byRoute bool
	totals  map[routeAccountKey][2]int64
}

func newRouteAccountCountIndex(byRoute bool, rows []routeAccountCountRow) routeAccountCountIndex {
	idx := routeAccountCountIndex{byRoute: byRoute, totals: make(map[routeAccountKey][2]int64, len(rows))}
	for _, row := range rows {
		key := routeAccountKey{pool: row.Pool}
		if byRoute {
			key.route = row.RouteID
		}
		current := idx.totals[key]
		idx.totals[key] = [2]int64{current[0] + row.Total, current[1] + row.Healthy}
	}
	return idx
}

func (idx routeAccountCountIndex) lookup(routeID, pool string) (total, healthy int64) {
	key := routeAccountKey{pool: pool}
	if idx.byRoute {
		key.route = routeID
	}
	counts := idx.totals[key]
	return counts[0], counts[1]
}

// routeAccountCounts replaces two COUNT queries per route. Errors degrade to
// zero counts, as the previous implementation ignored count errors too.
func (s *AdminConsoleService) routeAccountCounts(ctx context.Context) routeAccountCountIndex {
	byRoute := s.db.Migrator().HasTable(&model.AccountModelRoute{})
	healthy := "COUNT(*) FILTER (WHERE provider_accounts.status = 'active' AND provider_accounts.dead = false) AS healthy"
	var rows []routeAccountCountRow
	query := s.db.WithContext(ctx).Model(&model.TokenAccount{})
	if byRoute {
		query = query.Select("amr.model_route_id AS route_id, provider_accounts.pool AS pool, COUNT(*) AS total, "+healthy).
			Joins("JOIN account_model_routes amr ON amr.account_id = provider_accounts.id").
			Where("amr.enabled = ? AND amr.entitled = ?", true, true).
			Group("amr.model_route_id, provider_accounts.pool")
	} else {
		query = query.Select("provider_accounts.pool AS pool, COUNT(*) AS total, " + healthy).
			Group("provider_accounts.pool")
	}
	if err := query.Scan(&rows).Error; err != nil {
		rows = nil
	}
	return newRouteAccountCountIndex(byRoute, rows)
}

// usableCatalogRoutes keeps only routes the dispatcher could serve right now:
// the provider pool is enabled and at least one account is dispatchable for
// the route. /v1/models therefore lists only models that currently work; the
// admin console listing is intentionally unaffected.
func (s *V1Service) usableCatalogRoutes(ctx context.Context, item model.LogicalModel, routes []model.ModelRoute) ([]model.ModelRoute, error) {
	if len(routes) == 0 {
		return routes, nil
	}
	enabled, err := filterEnabledProviderRoutes(ctx, s.settings, routes)
	if err != nil {
		return nil, err
	}
	return filterRoutesWithAccounts(enabled, func(route model.ModelRoute) (bool, error) {
		routeCtx := withDispatchRoute(ctx, route, defaultRouteCost(route, model.RouteRequirements{}))
		return s.routeHasAccount(routeCtx, route, item.Kind)
	})
}

func filterRoutesWithAccounts(routes []model.ModelRoute, hasAccount func(model.ModelRoute) (bool, error)) ([]model.ModelRoute, error) {
	out := make([]model.ModelRoute, 0, len(routes))
	for _, route := range routes {
		ok, err := hasAccount(route)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, route)
		}
	}
	return out, nil
}
