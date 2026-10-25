package service

import "context"

const (
	maxReferencePixels = 100_000_000
)

// facePanelTransformEnabled was the Adobe Seedance face-panel grid switch.
// Adobe Seedance left the closed catalog with the GPT Image 2 cut, so the
// transform never applies. Kept as a wire-compatible no-op for callers.
const facePanelTransformEnabled = false

// shouldApplyReferenceGrid is retained as the wire-compatible name for the
// former reference_grid / face-panel flag. Always false after Adobe video
// engines left the closed catalog.
func (s *V1Service) shouldApplyReferenceGrid(ctx context.Context, modelID string, _ bool) bool {
	_ = ctx
	_ = modelID
	_ = s
	return facePanelTransformEnabled
}
