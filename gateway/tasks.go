package gateway

import (
	"context"
	"errors"

	"github.com/yypyyd/infinite-canvas/gateway/internal/service"
)

// ErrTaskNotFound means the pool has no record of the request.
var ErrTaskNotFound = service.ErrImageTaskNotFound

func (m *Module) principal() *service.APIPrincipal {
	return &service.APIPrincipal{Credential: m.app.Builtin}
}

// ImageTask returns the pool's view of an image request made by 幻图 with the
// given Idempotency-Key: status queued/in_progress/completed/failed, data for
// a completed task and, for failures, whether rerouting is safe.
func (m *Module) ImageTask(ctx context.Context, requestID string) (map[string]any, error) {
	return m.app.V1.ImageTask(ctx, m.principal(), requestID)
}

// AbandonImageTask stops a pending image request so no result can arrive
// after 幻图 settles it. A finished task is returned as is.
func (m *Module) AbandonImageTask(ctx context.Context, requestID string) (map[string]any, error) {
	return m.app.V1.AbandonImageRequest(ctx, m.principal(), requestID)
}

// AbandonVideoJob is the video counterpart of AbandonImageTask.
func (m *Module) AbandonVideoJob(ctx context.Context, jobID string) (map[string]any, error) {
	return m.app.V1.AbandonVideoJob(ctx, m.principal(), jobID)
}

// IsTaskNotFound reports a missing pool task.
func IsTaskNotFound(err error) bool {
	return errors.Is(err, service.ErrImageTaskNotFound) || errors.Is(err, service.ErrVideoJobNotFound)
}
