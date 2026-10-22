package repo

import (
	"context"
	"strings"

	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
)

// ListRoutesByLogical returns the routes of several logical models with one
// query, grouped by logical model ID. Each group has exactly the content and
// order ListRoutes would return for that ID; unknown IDs get no entry.
func (r *ModelRouteRepository) ListRoutesByLogical(ctx context.Context, logicalIDs []string, enabledOnly bool) (map[string][]model.ModelRoute, error) {
	ids := make([]string, 0, len(logicalIDs))
	seen := make(map[string]bool, len(logicalIDs))
	for _, id := range logicalIDs {
		id = strings.TrimSpace(id)
		if !model.IsCanonicalModelID(id) || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	out := make(map[string][]model.ModelRoute, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	if !r.hasRoutingTables() {
		for _, id := range ids {
			routes, err := r.ListRoutes(ctx, id, enabledOnly)
			if err != nil {
				return nil, err
			}
			out[id] = routes
		}
		return out, nil
	}
	var items []model.ModelRoute
	query := r.db.WithContext(ctx).Where("logical_model_id IN ?", ids).Where("id IN ?", canonicalRouteIDs())
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	if err := query.Order("priority desc, weight desc, id asc").Find(&items).Error; err != nil {
		return nil, err
	}
	return groupCanonicalRoutes(ids, items), nil
}

func groupCanonicalRoutes(ids []string, items []model.ModelRoute) map[string][]model.ModelRoute {
	out := make(map[string][]model.ModelRoute, len(ids))
	for _, id := range ids {
		out[id] = []model.ModelRoute{}
	}
	for _, item := range items {
		if _, wanted := out[item.LogicalModelID]; !wanted || !model.IsCanonicalRoute(item) {
			continue
		}
		out[item.LogicalModelID] = append(out[item.LogicalModelID], item)
	}
	for id := range out {
		sortRoutes(out[id])
	}
	return out
}
