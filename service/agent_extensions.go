package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

const (
	agentDefaultMaxToolCalls   = 8
	agentDefaultMaxMediaCalls  = 3
	agentDefaultMaxDurationSec = 900
	agentDefaultMaxCredits     = 100
)

type SubmitAgentFeedbackRequest struct {
	Signal string `json:"signal"`
	Note   string `json:"note,omitempty"`
}

type AgentFeedbackResult struct {
	Feedback         model.AgentFeedback `json:"feedback"`
	AdjustedMemories int64               `json:"adjustedMemories"`
}

type AgentMetrics struct {
	Hours              int            `json:"hours"`
	Runs               int            `json:"runs"`
	CompletedRuns      int            `json:"completedRuns"`
	FailedRuns         int            `json:"failedRuns"`
	CancelledRuns      int            `json:"cancelledRuns"`
	BudgetStoppedRuns  int            `json:"budgetStoppedRuns"`
	ToolCalls          int            `json:"toolCalls"`
	FailedToolCalls    int            `json:"failedToolCalls"`
	MediaCalls         int            `json:"mediaCalls"`
	StreamReconnects   int            `json:"streamReconnects"`
	ToolLeaseTakeovers int            `json:"toolLeaseTakeovers"`
	Feedback           map[string]int `json:"feedback"`
	AverageDurationMS  int64          `json:"averageDurationMs"`
	AverageToolCalls   float64        `json:"averageToolCalls"`
	ToolFailureRate    float64        `json:"toolFailureRate"`
	CompletionRate     float64        `json:"completionRate"`
	Credits            int64          `json:"credits"`
	Alerts             []string       `json:"alerts"`
}

func normalizeAgentRunBudget(value AgentRunBudget) (AgentRunBudget, error) {
	if value.MaxToolCalls == 0 {
		value.MaxToolCalls = agentDefaultMaxToolCalls
	}
	if value.MaxMediaCalls == 0 {
		value.MaxMediaCalls = agentDefaultMaxMediaCalls
	}
	if value.MaxDurationSec == 0 {
		value.MaxDurationSec = agentDefaultMaxDurationSec
	}
	if value.MaxCredits == 0 {
		value.MaxCredits = agentDefaultMaxCredits
	}
	if value.MaxToolCalls < 1 || value.MaxToolCalls > 12 || value.MaxMediaCalls < 1 || value.MaxMediaCalls > 6 || value.MaxDurationSec < 60 || value.MaxDurationSec > 1800 || value.MaxCredits < 1 || value.MaxCredits > 10000 {
		return AgentRunBudget{}, safeMessageError{message: "Agent 运行预算无效"}
	}
	return value, nil
}

func effectiveAgentRunBudget(run model.AgentRun) AgentRunBudget {
	budget, _ := normalizeAgentRunBudget(AgentRunBudget{MaxToolCalls: run.MaxToolCalls, MaxMediaCalls: run.MaxMediaCalls, MaxDurationSec: run.MaxDurationSec, MaxCredits: run.MaxCredits})
	return budget
}

func normalizeAgentRunSnapshot(raw json.RawMessage, context AgentCanvasContext) (json.RawMessage, string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		fallback, err := json.Marshal(map[string]any{"nodes": context.Nodes, "connections": context.Connections})
		if err != nil {
			return nil, "", err
		}
		raw = fallback
	}
	if len(raw) > 384<<10 {
		return nil, "", safeMessageError{message: "画布撤销快照过大"}
	}
	var snapshot struct {
		Nodes       []json.RawMessage `json:"nodes"`
		Connections []json.RawMessage `json:"connections"`
	}
	if json.Unmarshal(raw, &snapshot) != nil || snapshot.Nodes == nil || snapshot.Connections == nil || len(snapshot.Nodes) > 200 || len(snapshot.Connections) > 1000 {
		return nil, "", safeMessageError{message: "画布撤销快照无效"}
	}
	canonical, err := json.Marshal(snapshot)
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(digest[:]), nil
}

func agentRunBudgetReason(run model.AgentRun, steps []model.AgentStep, proposedTool string) string {
	budget := effectiveAgentRunBudget(run)
	toolCalls, mediaCalls := 0, 0
	for _, step := range steps {
		if step.Type != model.AgentStepTypeTool {
			continue
		}
		toolCalls++
		if definition, ok := agentToolDefinitionFor(step.ToolName); ok && definition.MediaCost > 0 {
			mediaCalls += definition.MediaCost
		}
	}
	if proposedTool != "" {
		toolCalls++
		if definition, ok := agentToolDefinitionFor(proposedTool); ok {
			mediaCalls += definition.MediaCost
		}
	}
	if toolCalls > budget.MaxToolCalls || proposedTool == "" && toolCalls >= budget.MaxToolCalls {
		return "tool_calls"
	}
	if mediaCalls > budget.MaxMediaCalls {
		return "media_calls"
	}
	if run.ID != "" && run.OrganizationID != "" && run.UserID != "" {
		credits, _ := repository.GetAgentRunCredits(run.OrganizationID, run.UserID, run.ID)
		if credits >= budget.MaxCredits {
			return "credits"
		}
	}
	startedAt, err := time.Parse(timestampLayout, run.StartedAt)
	if err == nil && time.Since(startedAt) >= time.Duration(budget.MaxDurationSec)*time.Second {
		return "duration"
	}
	return ""
}

func agentBudgetMessage(reason string) string {
	switch reason {
	case "tool_calls":
		return "本轮已达到工具调用预算，已停止继续操作并保留现有结果。"
	case "media_calls":
		return "本轮已达到媒体生成预算，已停止继续生成并保留现有结果。"
	case "duration":
		return "本轮已达到运行时间预算，已停止继续操作并保留现有结果。"
	case "credits":
		return "本轮已达到算力预算，已停止继续操作并保留现有结果。"
	default:
		return "本轮运行预算已用尽，已保留现有结果。"
	}
}

func agentRunUsage(run model.AgentRun, steps []model.AgentStep) AgentRunUsage {
	usage := AgentRunUsage{DurationSec: int(agentDurationMS(run.StartedAt, run.CompletedAt) / 1000), StreamReconnects: run.StreamReconnects, ToolLeaseTakeovers: run.ToolLeaseTakeovers}
	if run.ID != "" && run.OrganizationID != "" && run.UserID != "" {
		usage.Credits, _ = repository.GetAgentRunCredits(run.OrganizationID, run.UserID, run.ID)
	}
	for _, step := range steps {
		if step.Type != model.AgentStepTypeTool {
			continue
		}
		usage.ToolCalls++
		if definition, ok := agentToolDefinitionFor(step.ToolName); ok {
			usage.MediaCalls += definition.MediaCost
		}
	}
	return usage
}

func mustListAgentToolSteps(run model.AgentRun) []model.AgentStep {
	steps, _ := repository.ListCompletedAgentToolSteps(run.OrganizationID, run.UserID, run.ID)
	return steps
}

func SubmitAgentFeedback(user model.AuthUser, runID string, request SubmitAgentFeedbackRequest) (AgentFeedbackResult, error) {
	if err := RequireOrganizationWrite(user); err != nil {
		return AgentFeedbackResult{}, err
	}
	runID, request.Signal, request.Note = strings.TrimSpace(runID), strings.TrimSpace(request.Signal), strings.TrimSpace(request.Note)
	signal := model.AgentFeedbackSignal(request.Signal)
	if runID == "" || !validAgentFeedbackSignal(signal) || utf8.RuneCountInString(request.Note) > 500 {
		return AgentFeedbackResult{}, safeMessageError{message: "Agent 反馈无效"}
	}
	run, err := repository.GetAgentRun(user.OrganizationID, user.ID, runID)
	if err != nil {
		return AgentFeedbackResult{}, err
	}
	if !run.Terminal() {
		return AgentFeedbackResult{}, safeMessageError{message: "运行结束后才能提交反馈"}
	}
	timestamp := now()
	feedback, adjusted, err := repository.SaveAgentFeedback(model.AgentFeedback{ID: newID("agent-feedback"), OrganizationID: user.OrganizationID, UserID: user.ID, RunID: runID, Signal: signal, Note: request.Note, CreatedAt: timestamp, UpdatedAt: timestamp})
	if err != nil {
		return AgentFeedbackResult{}, err
	}
	return AgentFeedbackResult{Feedback: feedback, AdjustedMemories: adjusted}, nil
}

func validAgentFeedbackSignal(signal model.AgentFeedbackSignal) bool {
	switch signal {
	case model.AgentFeedbackSignalAccepted, model.AgentFeedbackSignalHelpful, model.AgentFeedbackSignalUnhelpful, model.AgentFeedbackSignalDeleted, model.AgentFeedbackSignalCorrected:
		return true
	default:
		return false
	}
}

func RecordAgentStreamReconnect(user model.AuthUser, runID string) error {
	if strings.TrimSpace(runID) == "" {
		return safeMessageError{message: "运行编号无效"}
	}
	return repository.RecordAgentStreamReconnect(user.OrganizationID, user.ID, runID)
}

func GetAgentMetrics(user model.AuthUser, hours int) (AgentMetrics, error) {
	if hours == 0 {
		hours = 24
	}
	if hours < 1 || hours > 720 {
		return AgentMetrics{}, safeMessageError{message: "统计时间范围无效"}
	}
	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Format(timestampLayout)
	data, err := repository.GetAgentMetricsData(user.OrganizationID, user.ID, since)
	if err != nil {
		return AgentMetrics{}, err
	}
	result := AgentMetrics{Hours: hours, Runs: len(data.Runs), Feedback: map[string]int{}, Credits: data.Credits, Alerts: []string{}}
	var durationTotal int64
	for _, run := range data.Runs {
		switch run.Status {
		case model.AgentRunStatusCompleted:
			result.CompletedRuns++
		case model.AgentRunStatusFailed:
			result.FailedRuns++
		case model.AgentRunStatusCancelled:
			result.CancelledRuns++
		}
		if run.BudgetReason != "" {
			result.BudgetStoppedRuns++
		}
		result.StreamReconnects += run.StreamReconnects
		result.ToolLeaseTakeovers += run.ToolLeaseTakeovers
		durationTotal += agentDurationMS(run.StartedAt, run.CompletedAt)
	}
	for _, step := range data.Steps {
		if step.Type != model.AgentStepTypeTool {
			continue
		}
		result.ToolCalls++
		if step.Status == model.AgentStepStatusFailed {
			result.FailedToolCalls++
		}
		if definition, ok := agentToolDefinitionFor(step.ToolName); ok {
			result.MediaCalls += definition.MediaCost
		}
	}
	for _, feedback := range data.Feedback {
		result.Feedback[string(feedback.Signal)]++
	}
	if result.Runs > 0 {
		result.AverageDurationMS = durationTotal / int64(result.Runs)
		result.AverageToolCalls = float64(result.ToolCalls) / float64(result.Runs)
		result.CompletionRate = float64(result.CompletedRuns) / float64(result.Runs)
	}
	if result.ToolCalls > 0 {
		result.ToolFailureRate = float64(result.FailedToolCalls) / float64(result.ToolCalls)
	}
	if result.Runs >= 5 && result.CompletionRate < 0.8 {
		result.Alerts = append(result.Alerts, "Agent 运行完成率低于 80%")
	}
	if result.ToolCalls >= 10 && result.ToolFailureRate > 0.15 {
		result.Alerts = append(result.Alerts, "Agent 工具失败率高于 15%")
	}
	if result.Runs >= 5 && result.StreamReconnects > result.Runs {
		result.Alerts = append(result.Alerts, "Agent 平均每轮事件流重连超过 1 次")
	}
	if result.ToolLeaseTakeovers > 0 {
		result.Alerts = append(result.Alerts, "Agent 出现工具执行租约接管，请检查页面失联或执行耗时")
	}
	return result, nil
}
