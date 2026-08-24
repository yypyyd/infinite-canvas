package model

type AgentSessionStatus string
type AgentMessageRole string
type AgentRunStatus string
type AgentStepType string
type AgentStepStatus string
type AgentEventType string
type AgentMemoryKind string
type AgentMemoryStatus string

const (
	AgentSessionStatusActive   AgentSessionStatus = "active"
	AgentSessionStatusArchived AgentSessionStatus = "archived"

	AgentMessageRoleUser      AgentMessageRole = "user"
	AgentMessageRoleAssistant AgentMessageRole = "assistant"

	AgentRunStatusRunning             AgentRunStatus = "running"
	AgentRunStatusWaitingConfirmation AgentRunStatus = "waiting_confirmation"
	AgentRunStatusWaitingTool         AgentRunStatus = "waiting_tool"
	AgentRunStatusCompleted           AgentRunStatus = "completed"
	AgentRunStatusFailed      AgentRunStatus = "failed"
	AgentRunStatusCancelled   AgentRunStatus = "cancelled"

	AgentStepTypeCompletion AgentStepType = "completion"
	AgentStepTypeTool       AgentStepType = "tool"

	AgentStepStatusRunning   AgentStepStatus = "running"
	AgentStepStatusCompleted AgentStepStatus = "completed"
	AgentStepStatusFailed    AgentStepStatus = "failed"
	AgentStepStatusCancelled AgentStepStatus = "cancelled"

	AgentEventRunStarted           AgentEventType = "run.started"
	AgentEventPlanCreated          AgentEventType = "plan.created"
	AgentEventMessageDelta         AgentEventType = "message.delta"
	AgentEventToolConfirmationRequired AgentEventType = "tool.confirmation_required"
	AgentEventToolCall             AgentEventType = "tool.call"
	AgentEventToolCompleted AgentEventType = "tool.completed"
	AgentEventToolReverted  AgentEventType = "tool.reverted"
	AgentEventRunCompleted  AgentEventType = "run.completed"
	AgentEventRunFailed     AgentEventType = "run.failed"
	AgentEventRunCancelled  AgentEventType = "run.cancelled"

	AgentMemoryKindPreference AgentMemoryKind = "preference"
	AgentMemoryKindFact       AgentMemoryKind = "fact"
	AgentMemoryKindConstraint AgentMemoryKind = "constraint"
	AgentMemoryKindExperience AgentMemoryKind = "experience"

	AgentMemoryStatusActive   AgentMemoryStatus = "active"
	AgentMemoryStatusForgotten AgentMemoryStatus = "forgotten"
)

type AgentSession struct {
	ID             string             `json:"id" gorm:"primaryKey"`
	OrganizationID string             `json:"organizationId" gorm:"index:idx_agent_session_owner,priority:1"`
	UserID         string             `json:"userId" gorm:"index:idx_agent_session_owner,priority:2"`
	ProjectID      string             `json:"projectId" gorm:"index"`
	Profile        string             `json:"profile" gorm:"type:text"`
	Title          string             `json:"title"`
	Status         AgentSessionStatus `json:"status" gorm:"index"`
	CreatedAt      string             `json:"createdAt" gorm:"index"`
	UpdatedAt      string             `json:"updatedAt"`
}

type AgentMessage struct {
	ID             string           `json:"id" gorm:"primaryKey"`
	OrganizationID string           `json:"organizationId" gorm:"index:idx_agent_message_owner,priority:1"`
	UserID         string           `json:"userId" gorm:"index:idx_agent_message_owner,priority:2"`
	SessionID      string           `json:"sessionId" gorm:"uniqueIndex:idx_agent_message_sequence,priority:1;index"`
	Role           AgentMessageRole `json:"role" gorm:"index"`
	Content        string           `json:"content" gorm:"type:text"`
	Sequence       int64            `json:"sequence" gorm:"uniqueIndex:idx_agent_message_sequence,priority:2"`
	CreatedAt      string           `json:"createdAt" gorm:"index"`
}

type AgentRun struct {
	ID             string         `json:"id" gorm:"primaryKey"`
	OrganizationID string         `json:"organizationId" gorm:"index:idx_agent_run_owner,priority:1"`
	UserID         string         `json:"userId" gorm:"index:idx_agent_run_owner,priority:2"`
	SessionID      string         `json:"sessionId" gorm:"index"`
	MessageID      string         `json:"-" gorm:"index"`
	Model          string         `json:"model" gorm:"index"`
	Context        string         `json:"-" gorm:"type:text"`
	Status         AgentRunStatus `json:"status" gorm:"index"`
	Error          string         `json:"error,omitempty" gorm:"type:text"`
	StartedAt      string         `json:"startedAt" gorm:"index"`
	CompletedAt    string         `json:"completedAt,omitempty"`
	CreatedAt      string         `json:"createdAt" gorm:"index"`
	UpdatedAt      string         `json:"updatedAt"`
}

type AgentStep struct {
	ID             string          `json:"id" gorm:"primaryKey"`
	OrganizationID string          `json:"organizationId" gorm:"index:idx_agent_step_owner,priority:1"`
	UserID         string          `json:"userId" gorm:"index:idx_agent_step_owner,priority:2"`
	RunID          string          `json:"runId" gorm:"index;index:idx_agent_step_tool_call,priority:1"`
	Type           AgentStepType   `json:"type" gorm:"index"`
	Status         AgentStepStatus `json:"status" gorm:"index"`
	ToolCallID     string          `json:"toolCallId" gorm:"index:idx_agent_step_tool_call,priority:2"`
	ToolName       string          `json:"toolName" gorm:"index"`
	Confirmation   string          `json:"confirmation,omitempty" gorm:"index"`
	ExecutionToken string          `json:"-" gorm:"index"`
	ExecutionAt    string          `json:"-" gorm:"index"`
	Input          string          `json:"-" gorm:"type:text"`
	Output         string          `json:"-" gorm:"type:text"`
	Error          string          `json:"error,omitempty" gorm:"type:text"`
	StartedAt      string          `json:"startedAt"`
	CompletedAt    string          `json:"completedAt,omitempty"`
	CreatedAt      string          `json:"createdAt"`
	UpdatedAt      string          `json:"updatedAt"`
}

type AgentEvent struct {
	ID             string         `json:"id" gorm:"primaryKey"`
	OrganizationID string         `json:"organizationId" gorm:"index:idx_agent_event_owner,priority:1"`
	UserID         string         `json:"userId" gorm:"index:idx_agent_event_owner,priority:2"`
	RunID          string         `json:"runId" gorm:"uniqueIndex:idx_agent_event_sequence,priority:1;index"`
	Sequence       int64          `json:"sequence" gorm:"uniqueIndex:idx_agent_event_sequence,priority:2"`
	Type           AgentEventType `json:"type" gorm:"index"`
	Payload        string         `json:"-" gorm:"type:text"`
	CreatedAt      string         `json:"createdAt" gorm:"index"`
}

// AgentMemory is a user-controlled, server-side fact that can be reused by later Runs.
// Content is intentionally plain text and never contains model credentials or canvas media.
type AgentMemory struct {
	ID             string            `json:"id" gorm:"primaryKey"`
	OrganizationID string            `json:"organizationId" gorm:"index:idx_agent_memory_scope,priority:1;uniqueIndex:idx_agent_memory_identity,priority:1"`
	UserID         string            `json:"userId" gorm:"index:idx_agent_memory_scope,priority:2;uniqueIndex:idx_agent_memory_identity,priority:2"`
	ProjectID      string            `json:"projectId" gorm:"index:idx_agent_memory_scope,priority:3;uniqueIndex:idx_agent_memory_identity,priority:3"`
	Kind           AgentMemoryKind   `json:"kind" gorm:"index"`
	Key            string            `json:"key" gorm:"index:idx_agent_memory_key,priority:1;uniqueIndex:idx_agent_memory_identity,priority:4"`
	Content        string            `json:"content" gorm:"type:text"`
	SourceRunID    string            `json:"sourceRunId,omitempty" gorm:"index"`
	SourceMessageID string           `json:"sourceMessageId,omitempty" gorm:"index"`
	Confidence     float64           `json:"confidence"`
	Status         AgentMemoryStatus `json:"status" gorm:"index"`
	ExpiresAt      string            `json:"expiresAt,omitempty" gorm:"index"`
	ForgottenAt    string            `json:"forgottenAt,omitempty"`
	CreatedAt      string            `json:"createdAt" gorm:"index"`
	UpdatedAt      string            `json:"updatedAt"`
}

func (run AgentRun) Terminal() bool {
	return run.Status == AgentRunStatusCompleted || run.Status == AgentRunStatusFailed || run.Status == AgentRunStatusCancelled
}
