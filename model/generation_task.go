package model

type GenerationTaskStatus string
type CreditSource string

const (
	GenerationTaskStatusRunning GenerationTaskStatus = "running"
	GenerationTaskStatusSuccess GenerationTaskStatus = "success"
	GenerationTaskStatusFailed  GenerationTaskStatus = "failed"
)

const (
	CreditSourcePersonal     CreditSource = "personal"
	CreditSourceOrganization CreditSource = "organization"
)

// GenerationTask records backend model requests for user task center and admin operations.
type GenerationTask struct {
	ID             string               `json:"id" gorm:"primaryKey"`
	OrganizationID string               `json:"organizationId" gorm:"index;uniqueIndex:idx_generation_task_request"`
	UserID         string               `json:"userId" gorm:"index;uniqueIndex:idx_generation_task_request"`
	RequestID      string               `json:"requestId" gorm:"size:191;uniqueIndex:idx_generation_task_request"`
	Model          string               `json:"model" gorm:"index"`
	UpstreamModel  string               `json:"upstreamModel"`
	ChannelName    string               `json:"channelName" gorm:"index"`
	Path           string               `json:"path"`
	Modality       string               `json:"modality" gorm:"index"`
	Operation      string               `json:"operation"`
	ResolutionTier string               `json:"resolutionTier"`
	Quantity       int                  `json:"quantity"`
	Credits        int                  `json:"credits"`
	CreditSource   CreditSource         `json:"creditSource" gorm:"size:16;not null;default:personal;index"`
	Status         GenerationTaskStatus `json:"status" gorm:"index"`
	ErrorMessage   string               `json:"errorMessage" gorm:"type:text"`
	DurationMs     int64                `json:"durationMs"`
	CreatedAt      string               `json:"createdAt" gorm:"index"`
	UpdatedAt      string               `json:"updatedAt"`
}

type GenerationTaskList struct {
	Items []GenerationTask `json:"items"`
	Total int              `json:"total"`
}

type DashboardMetric struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value int64  `json:"value"`
}

type DashboardNameValue struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

type AdminDashboard struct {
	Metrics        []DashboardMetric    `json:"metrics"`
	RecentTasks    []GenerationTask     `json:"recentTasks"`
	TopModels      []DashboardNameValue `json:"topModels"`
	ChannelErrors  []DashboardNameValue `json:"channelErrors"`
	RecentFailures []GenerationTask     `json:"recentFailures"`
}
