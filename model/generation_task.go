package model

import "encoding/json"

type GenerationTaskStatus string
type CreditSource string

const (
	GenerationTaskStatusRunning GenerationTaskStatus = "running"
	GenerationTaskStatusSuccess GenerationTaskStatus = "success"
	GenerationTaskStatusFailed  GenerationTaskStatus = "failed"
)

// PricingSnapshot records the exact pricing inputs and result used when a request is charged.
type PricingSnapshot struct {
	Model           string   `json:"model"`
	Modality        string   `json:"modality"`
	Operation       string   `json:"operation"`
	Unit            string   `json:"unit"`
	ResolutionTier  string   `json:"resolutionTier"`
	Quantity        int      `json:"quantity"`
	BillingMode     string   `json:"billingMode"`
	RuleCredits     int      `json:"ruleCredits"`
	MinCredits      int      `json:"minCredits"`
	ModelRatio      float64  `json:"modelRatio"`
	CompletionRatio float64  `json:"completionRatio"`
	RuleEnabled     bool     `json:"ruleEnabled"`
	UserGroup       string   `json:"userGroup"`
	GroupRatio      float64  `json:"groupRatio"`
	UserSpecRatio   *float64 `json:"userSpecRatio,omitempty"`
	EffectiveRatio  float64  `json:"effectiveRatio"`
	Source          string   `json:"source"`
	Credits         int      `json:"credits"`
}

const (
	CreditSourcePersonal     CreditSource = "personal"
	CreditSourceOrganization CreditSource = "organization"
)

// GenerationTask records backend model requests for user task center and admin operations.
type GenerationTask struct {
	ID              string               `json:"id" gorm:"primaryKey"`
	OrganizationID  string               `json:"organizationId" gorm:"index;uniqueIndex:idx_generation_task_request"`
	UserID          string               `json:"userId" gorm:"index;uniqueIndex:idx_generation_task_request"`
	RequestID       string               `json:"requestId" gorm:"size:191;uniqueIndex:idx_generation_task_request"`
	BatchJobID      string               `json:"batchJobId,omitempty" gorm:"index"`
	BatchItemID     string               `json:"batchItemId,omitempty" gorm:"index"`
	Model           string               `json:"model" gorm:"index"`
	UpstreamModel   string               `json:"upstreamModel"`
	ChannelName     string               `json:"channelName" gorm:"index"`
	Path            string               `json:"path"`
	Modality        string               `json:"modality" gorm:"index"`
	Operation       string               `json:"operation"`
	ResolutionTier  string               `json:"resolutionTier"`
	Quantity        int                  `json:"quantity"`
	Credits         int                  `json:"credits"`
	PricingSnapshot PricingSnapshot      `json:"-" gorm:"serializer:json;type:text"`
	CreditSource    CreditSource         `json:"creditSource" gorm:"size:16;not null;default:personal;index"`
	Status          GenerationTaskStatus `json:"status" gorm:"index"`
	UpstreamTaskID  string               `json:"-" gorm:"index"`
	ResultJSON      string               `json:"-" gorm:"type:longtext"`
	StorageKeys     []string             `json:"storageKeys,omitempty" gorm:"serializer:json;type:text"`
	ErrorMessage    string               `json:"errorMessage" gorm:"type:text"`
	DurationMs      int64                `json:"durationMs"`
	CreatedAt       string               `json:"createdAt" gorm:"index"`
	UpdatedAt       string               `json:"updatedAt"`
}

type GenerationTaskRecovery struct {
	RequestID      string               `json:"requestId"`
	Model          string               `json:"model"`
	Modality       string               `json:"modality"`
	Path           string               `json:"path"`
	Status         GenerationTaskStatus `json:"status"`
	UpstreamTaskID string               `json:"upstreamTaskId,omitempty"`
	Result         json.RawMessage      `json:"result,omitempty"`
	StorageKeys    []string             `json:"storageKeys,omitempty"`
	ErrorMessage   string               `json:"errorMessage,omitempty"`
	CreatedAt      string               `json:"createdAt"`
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
