package model

type OperationsQueueHealth struct {
	BatchQueued                     int64 `json:"batchQueued"`
	BatchRunning                    int64 `json:"batchRunning"`
	BatchExpiredLeases              int64 `json:"batchExpiredLeases"`
	EmailPending                    int64 `json:"emailPending"`
	EmailFailed                     int64 `json:"emailFailed"`
	EmailExpiredLeases              int64 `json:"emailExpiredLeases"`
	ObjectDeletionPending           int64 `json:"objectDeletionPending"`
	ObjectDeletionFailed            int64 `json:"objectDeletionFailed"`
	ObjectDeletionExpiredLeases     int64 `json:"objectDeletionExpiredLeases"`
}

type OperationsGenerationMetrics struct {
	WindowHours       int     `json:"windowHours"`
	Total             int64   `json:"total"`
	Running           int64   `json:"running"`
	Success           int64   `json:"success"`
	Failed            int64   `json:"failed"`
	SuccessRate       float64 `json:"successRate"`
	AverageDurationMs int64   `json:"averageDurationMs"`
	P95DurationMs     int64   `json:"p95DurationMs"`
}

type OperationsAlert struct {
	Key       string `json:"key"`
	Value     int64  `json:"value"`
	Threshold int64  `json:"threshold"`
}

type DataConsistencyIssue struct {
	ID             string `json:"id"`
	Category       string `json:"category"`
	Code           string `json:"code"`
	Severity       string `json:"severity"`
	OrganizationID string `json:"organizationId"`
	ResourceType   string `json:"resourceType"`
	ResourceID     string `json:"resourceId"`
	Message        string `json:"message"`
	RepairAction   string `json:"repairAction,omitempty"`
	Target         string `json:"-"`
}

type DataConsistencyReport struct {
	CheckedAt     string                 `json:"checkedAt"`
	StorageStatus string                 `json:"storageStatus"`
	TotalIssues   int                    `json:"totalIssues"`
	Repairable    int                    `json:"repairable"`
	Truncated     bool                   `json:"truncated"`
	Summary       map[string]int         `json:"summary"`
	Issues        []DataConsistencyIssue `json:"issues"`
}

type RepairDataConsistencyInput struct {
	IssueID string `json:"issueId"`
}
