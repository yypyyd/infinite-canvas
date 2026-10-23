package model

// Public model health for the user-facing model plaza.
// Status values mirror sub2api monitor semantics without exposing channels.
const (
	ModelStatusOperational = "operational"
	ModelStatusDegraded    = "degraded"
	ModelStatusUnavailable = "unavailable"
	ModelStatusUnknown     = "unknown"
)

// ModelStatusBucket is one time window in the mini timeline.
type ModelStatusBucket struct {
	Status    string `json:"status"`
	At        string `json:"at"`
	LatencyMs *int64 `json:"latencyMs,omitempty"`
}

// ModelStatusItem is the aggregate health of one logical public model.
type ModelStatusItem struct {
	ID        string              `json:"id"`
	Status    string              `json:"status"`
	Label     string              `json:"label"`
	LatencyMs *int64              `json:"latencyMs,omitempty"`
	Timeline  []ModelStatusBucket `json:"timeline,omitempty"`
}

// ModelStatusResponse is returned by GET /api/model-status.
type ModelStatusResponse struct {
	UpdatedAt string            `json:"updatedAt"`
	Models    []ModelStatusItem `json:"models"`
}