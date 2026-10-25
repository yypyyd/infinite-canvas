package model

// VideoRenderJobStatus is the Release-2 render lifecycle. Stages form a linear DAG:
// queued → materializing → rendering → archiving → pending_review → completed
// with fail/cancel terminals. Workers must not skip stages or invent a "success"
// from the freeze-version API.
type VideoRenderJobStatus string

const (
	VideoRenderStatusQueued        VideoRenderJobStatus = "queued"
	VideoRenderStatusMaterializing VideoRenderJobStatus = "materializing"
	VideoRenderStatusRendering     VideoRenderJobStatus = "rendering"
	VideoRenderStatusArchiving     VideoRenderJobStatus = "archiving"
	VideoRenderStatusPendingReview VideoRenderJobStatus = "pending_review"
	VideoRenderStatusCompleted     VideoRenderJobStatus = "completed"
	VideoRenderStatusFailed        VideoRenderJobStatus = "failed"
	VideoRenderStatusCancelled     VideoRenderJobStatus = "cancelled"
)

type VideoRenderReviewStatus string

const (
	VideoRenderReviewPending  VideoRenderReviewStatus = "pending"
	VideoRenderReviewApproved VideoRenderReviewStatus = "approved"
	VideoRenderReviewRejected VideoRenderReviewStatus = "rejected"
)

// VideoRenderJob is one render attempt for a frozen project version.
type VideoRenderJob struct {
	ID               string                  `json:"id" gorm:"primaryKey"`
	OrganizationID   string                  `json:"organizationId" gorm:"not null;index;uniqueIndex:idx_video_render_request,priority:1"`
	RequestID        string                  `json:"requestId" gorm:"size:64;uniqueIndex:idx_video_render_request,priority:2"`
	RequestHash      string                  `json:"-" gorm:"size:64"`
	ProjectID        string                  `json:"projectId" gorm:"index"`
	ProjectVersionID string                  `json:"projectVersionId" gorm:"index"`
	VersionNumber    int                     `json:"versionNumber"`
	Name             string                  `json:"name"`
	Status           VideoRenderJobStatus    `json:"status" gorm:"index;size:32"`
	Phase            string                  `json:"phase" gorm:"size:32"` // current DAG node
	TimelineJSON     string                  `json:"-" gorm:"type:text"`
	OutputSpecJSON   string                  `json:"-" gorm:"type:text"`
	DurationMs       int                     `json:"durationMs"`
	EstimatedCredits int                     `json:"estimatedCredits"`
	ReservedCredits  int                     `json:"reservedCredits"`
	SettledCredits   int                     `json:"settledCredits"`
	ReleasedCredits  int                     `json:"releasedCredits"`
	ActualCredits    int                     `json:"actualCredits"`
	CreditSource     CreditSource            `json:"creditSource" gorm:"size:16;not null;default:personal;index"`
	ResultStorageKey string                  `json:"resultStorageKey" gorm:"size:191"`
	ResultSize       int64                   `json:"resultSize"`
	ErrorCategory    string                  `json:"errorCategory,omitempty" gorm:"size:48"`
	ErrorMessage     string                  `json:"errorMessage,omitempty" gorm:"type:text"`
	ReviewStatus     VideoRenderReviewStatus `json:"reviewStatus" gorm:"size:16;index"`
	Attempt          int                     `json:"attempt"`
	LeaseToken       string                  `json:"-" gorm:"size:64;index"`
	LeaseOwner       string                  `json:"-" gorm:"size:64"`
	LeaseExpiresAt   string                  `json:"-" gorm:"index"`
	NextAttemptAt    string                  `json:"-" gorm:"index"`
	StartedAt        string                  `json:"startedAt,omitempty"`
	FinishedAt       string                  `json:"finishedAt,omitempty"`
	CreatedBy        string                  `json:"createdBy" gorm:"index"`
	CreatedAt        string                  `json:"createdAt"`
	UpdatedAt        string                  `json:"updatedAt"`
}

type VideoRenderJobList struct {
	Items []VideoRenderJob `json:"items"`
	Total int              `json:"total"`
}

type CreateVideoRenderJobInput struct {
	RequestID       string `json:"requestId"`
	ExpectedVersion int64  `json:"expectedVersion"`
	VersionNumber   int    `json:"versionNumber"`
}

type ReviewVideoRenderJobInput struct {
	Status VideoRenderReviewStatus `json:"status"`
	Note   string                  `json:"note"`
}
