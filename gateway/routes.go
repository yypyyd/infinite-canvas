package gateway

import (
	"context"
	"errors"

	"github.com/yypyyd/infinite-canvas/gateway/internal/service"
)

// RouteDetail is one provider route of a pool model with its effective
// schedule and account health (see service.RouteDetail).
type RouteDetail = service.RouteDetail

// RouteUpdate patches one route; nil fields stay unchanged and Clear* resets
// an override to the provider default.
type RouteUpdate = service.RouteUpdate

var errAdminUnavailable = errors.New("account pool admin service unavailable")

// RouteDetails lists every pool route with priority, weight and account counts.
func (m *Module) RouteDetails(ctx context.Context) ([]RouteDetail, error) {
	if m == nil || m.app == nil || m.app.Admin == nil {
		return nil, errAdminUnavailable
	}
	return m.app.Admin.RouteDetails(ctx)
}

// UpdateRoute patches one pool route and drops the cached model list so the
// main site sees the change at once.
func (m *Module) UpdateRoute(ctx context.Context, logicalModelID, routeID string, update RouteUpdate) error {
	if m == nil || m.app == nil || m.app.Admin == nil {
		return errAdminUnavailable
	}
	if _, err := m.app.Admin.UpdateLogicalRoute(ctx, logicalModelID, routeID, update); err != nil {
		return err
	}
	m.InvalidateModels()
	return nil
}
