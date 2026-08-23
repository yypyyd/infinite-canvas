package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

const (
	agentSystemPrompt = "你是画布助手，根据画布 JSON 和最后用户请求行动。修改画布必须调用工具，禁止假装完成；多工具任务先调用 canvas.plan，每次只调用一个。参数：canvas.plan {summary,steps}；image.generate {prompt,count,referenceNodeIds}；video.generate {prompt,duration,imageNodeId}；canvas.add_text {text,placement,sourceNodeIds}；canvas.arrange {nodeIds,mode,gap}；canvas.delete {nodeIds}；canvas.update_text {nodeId,text}。工具成功后根据 TOOL_RESULT 继续，不得重复同名同参数工具；删除和改文本需用户确认。调用工具时只输出一行 `TOOL_CALL {\"name\":\"canvas.arrange\",\"arguments\":{...}}`；无需工具或任务完成时简短回答。"
	agentMaxToolCalls = 8
	agentWaitingToolTimeout = 15 * time.Minute
	agentWaitingToolTimeoutError = "工具结果等待超时"
	agentToolExecutionLease = 90 * time.Second
	agentRunningTimeout = 3 * time.Minute
	agentRunningTimeoutError = "助手运行恢复超时"
)

type CreateAgentSessionRequest struct {
	SessionID string `json:"sessionId"`
	ProjectID string `json:"projectId"`
	Title     string `json:"title"`
	Profile   any    `json:"profile"`
}

type AgentCanvasNode struct {
	ID         string   `json:"id"`
	Type       string   `json:"type"`
	Title      string   `json:"title"`
	X          float64  `json:"x"`
	Y          float64  `json:"y"`
	Width      float64  `json:"width"`
	Height     float64  `json:"height"`
	Content    string   `json:"content,omitempty"`
	Prompt     string   `json:"prompt,omitempty"`
	References []string `json:"references,omitempty"`
	StorageKey string   `json:"storageKey,omitempty"`
}

type AgentCanvasConnection struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type AgentCanvasContext struct {
	SelectedNodeIDs []string                `json:"selectedNodeIds"`
	Nodes           []AgentCanvasNode       `json:"nodes"`
	Connections     []AgentCanvasConnection `json:"connections"`
}

type SubmitAgentMessageRequest struct {
	RunID         string             `json:"runId"`
	Content       string             `json:"content"`
	Model         string             `json:"model"`
	CanvasContext AgentCanvasContext `json:"canvasContext"`
}

type AgentToolImage struct {
	NodeID     string `json:"nodeId"`
	StorageKey string `json:"storageKey"`
}

type AgentToolVideo struct {
	NodeID     string `json:"nodeId"`
	StorageKey string `json:"storageKey"`
}

type AgentToolPosition struct {
	NodeID string  `json:"nodeId"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
}

type AgentToolPlan struct {
	Summary string   `json:"summary"`
	Steps   []string `json:"steps"`
}

type SubmitAgentToolResultRequest struct {
	CallID        string              `json:"callId"`
	ExecutionToken string              `json:"executionToken,omitempty"`
	Status        string              `json:"status"`
	Error     string              `json:"error,omitempty"`
	Images    []AgentToolImage    `json:"images,omitempty"`
	Video     *AgentToolVideo     `json:"video,omitempty"`
	NodeIDs   []string            `json:"nodeIds,omitempty"`
	Positions []AgentToolPosition `json:"positions,omitempty"`
	NodeID    string              `json:"nodeId,omitempty"`
	Text      string              `json:"text,omitempty"`
	Placement string              `json:"placement,omitempty"`
	Plan      *AgentToolPlan      `json:"plan,omitempty"`
}

type ConfirmAgentToolRequest struct {
	CallID   string `json:"callId"`
	Decision string `json:"decision"`
}

type RevertAgentToolRequest struct {
	CallID string `json:"callId"`
}

type AgentToolResultReceipt struct {
	Status string                  `json:"status"`
	Result *SubmitAgentToolResultRequest `json:"result,omitempty"`
}

type ClaimAgentToolRequest struct {
	CallID string `json:"callId"`
	Token  string `json:"token"`
}

type AgentRunSubmission struct {
	Message model.AgentMessage `json:"message"`
	Run     model.AgentRun     `json:"run"`
}

type imageGenerateArguments struct {
	Prompt          string   `json:"prompt"`
	Count           int      `json:"count"`
	ReferenceNodeIDs []string `json:"referenceNodeIds,omitempty"`
}

type videoGenerateArguments struct {
	Prompt      string `json:"prompt"`
	Duration    int    `json:"duration"`
	ImageNodeID string `json:"imageNodeId,omitempty"`
}

type canvasArrangeArguments struct {
	NodeIDs []string `json:"nodeIds"`
	Mode    string   `json:"mode"`
	Gap     int      `json:"gap"`
}

type canvasAddTextArguments struct {
	Text          string   `json:"text"`
	Placement     string   `json:"placement"`
	SourceNodeIDs []string `json:"sourceNodeIds,omitempty"`
}

type canvasDeleteArguments struct {
	NodeIDs []string `json:"nodeIds"`
}

type canvasUpdateTextArguments struct {
	NodeID string `json:"nodeId"`
	Text   string `json:"text"`
}

type canvasPlanArguments struct {
	Summary string   `json:"summary"`
	Steps   []string `json:"steps"`
}

type agentNodeAuthorization struct {
	NodeIDs      map[string]struct{}
	ImageNodeIDs map[string]struct{}
	TextNodeIDs  map[string]struct{}
}

type agentToolCall struct {
	ID        string
	Name      string
	Arguments any
	Raw       string
}

type agentCompletion struct {
	Content  string
	ToolCall *agentToolCall
}

type agentChatMessage struct {
	Role       string              `json:"role"`
	Content    any                 `json:"content,omitempty"`
	ToolCallID string              `json:"tool_call_id,omitempty"`
	ToolCalls  []agentChatToolCall `json:"tool_calls,omitempty"`
}

type agentChatToolCall struct {
	ID       string                `json:"id"`
	Type     string                `json:"type"`
	Function agentChatToolFunction `json:"function"`
}

type agentChatToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type agentRunExecution struct {
	requestID string
	cancel    context.CancelFunc
}

var agentRuns = struct {
	sync.Mutex
	cancels map[string]agentRunExecution
}{cancels: map[string]agentRunExecution{}}

func CreateAgentSession(user model.AuthUser, request CreateAgentSessionRequest) (model.AgentSession, error) {
	if err := RequireOrganizationWrite(user); err != nil { return model.AgentSession{}, err }
	request.SessionID, request.ProjectID, request.Title = strings.TrimSpace(request.SessionID), strings.TrimSpace(request.ProjectID), strings.TrimSpace(request.Title)
	if request.ProjectID == "" { return model.AgentSession{}, safeMessageError{message: "画布项目编号无效"} }
	if request.SessionID != "" && (!strings.HasPrefix(request.SessionID, "agent-session-") || len(request.SessionID) > 191) { return model.AgentSession{}, safeMessageError{message: "会话编号无效"} }
	exists, err := repository.UserProjectExists(user.OrganizationID, user.ID, request.ProjectID)
	if err != nil { return model.AgentSession{}, err }
	if !exists { return model.AgentSession{}, safeMessageError{message: "画布项目不存在"} }
	profile, err := json.Marshal(request.Profile)
	if err != nil { return model.AgentSession{}, safeMessageError{message: "会话配置无效"} }
	if len(profile) > 256<<10 { return model.AgentSession{}, safeMessageError{message: "会话配置过大"} }
	timestamp := now()
	sessionID := request.SessionID
	if sessionID == "" { sessionID = newID("agent-session") }
	session := model.AgentSession{ID: sessionID, OrganizationID: user.OrganizationID, UserID: user.ID, ProjectID: request.ProjectID, Profile: string(profile), Title: request.Title, Status: model.AgentSessionStatusActive, CreatedAt: timestamp, UpdatedAt: timestamp}
	session, err = repository.CreateAgentSession(session)
	if errors.Is(err, repository.ErrAgentToolResultConflict) { return model.AgentSession{}, safeMessageError{message: "会话编号与已提交请求不一致"} }
	return session, err
}

func SubmitAgentMessage(user model.AuthUser, sessionID string, request SubmitAgentMessageRequest) (AgentRunSubmission, error) {
	if err := RequireOrganizationWrite(user); err != nil { return AgentRunSubmission{}, err }
	sessionID, request.RunID, request.Content, request.Model = strings.TrimSpace(sessionID), strings.TrimSpace(request.RunID), strings.TrimSpace(request.Content), strings.TrimSpace(request.Model)
	if sessionID == "" { return AgentRunSubmission{}, safeMessageError{message: "会话编号无效"} }
	if request.RunID != "" && (!strings.HasPrefix(request.RunID, "agent-run-") || len(request.RunID) > 191) { return AgentRunSubmission{}, safeMessageError{message: "运行编号无效"} }
	if request.Content == "" || len(request.Content) > 256<<10 { return AgentRunSubmission{}, safeMessageError{message: "消息不能为空或过长"} }
	if request.Model == "" || len(request.Model) > 191 { return AgentRunSubmission{}, safeMessageError{message: "模型不能为空或过长"} }
	canvasContext, err := normalizeAgentCanvasContext(request.CanvasContext)
	if err != nil { return AgentRunSubmission{}, err }
	contextJSON, err := json.Marshal(canvasContext)
	if err != nil || len(contextJSON) > 256<<10 { return AgentRunSubmission{}, safeMessageError{message: "画布上下文过大"} }
	if _, err := repository.GetAgentSession(user.OrganizationID, user.ID, sessionID); err != nil { return AgentRunSubmission{}, err }
	timestamp, runID := now(), request.RunID
	if runID == "" { runID = newID("agent-run") }
	message := model.AgentMessage{ID: newID("agent-message"), OrganizationID: user.OrganizationID, UserID: user.ID, SessionID: sessionID, Role: model.AgentMessageRoleUser, Content: request.Content, CreatedAt: timestamp}
	run := model.AgentRun{ID: runID, OrganizationID: user.OrganizationID, UserID: user.ID, SessionID: sessionID, MessageID: message.ID, Model: request.Model, Context: string(contextJSON), Status: model.AgentRunStatusRunning, StartedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp}
	step := model.AgentStep{ID: newID("agent-step"), OrganizationID: user.OrganizationID, UserID: user.ID, RunID: runID, Type: model.AgentStepTypeCompletion, Status: model.AgentStepStatusRunning, Input: request.Content, StartedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp}
	event := model.AgentEvent{ID: newID("agent-event"), OrganizationID: user.OrganizationID, UserID: user.ID, RunID: runID, Type: model.AgentEventRunStarted, Payload: "{}", CreatedAt: timestamp}
	message, run, err = repository.CreateAgentRun(message, run, step, event)
	if errors.Is(err, repository.ErrAgentToolResultConflict) { return AgentRunSubmission{}, safeMessageError{message: "运行编号与已提交请求不一致"} }
	if err != nil { return AgentRunSubmission{}, err }
	agentRuns.Lock()
	_, alreadyRunning := agentRuns.cancels[run.ID]
	agentRuns.Unlock()
	if run.Status == model.AgentRunStatusRunning && !alreadyRunning { startAgentRun(run, user.Group) }
	return AgentRunSubmission{Message: message, Run: run}, nil
}

func SubmitAgentToolResult(user model.AuthUser, runID string, request SubmitAgentToolResultRequest) (model.AgentRun, error) {
	if err := RequireOrganizationWrite(user); err != nil { return model.AgentRun{}, err }
	runID, request.CallID, request.ExecutionToken, request.Status, request.Error = strings.TrimSpace(runID), strings.TrimSpace(request.CallID), strings.TrimSpace(request.ExecutionToken), strings.TrimSpace(request.Status), strings.TrimSpace(request.Error)
	request.Placement = strings.TrimSpace(request.Placement)
	if runID == "" || request.CallID == "" || request.ExecutionToken == "" || len(request.ExecutionToken) > 191 { return model.AgentRun{}, safeMessageError{message: "运行、工具调用或执行租约无效"} }
	if err := expireWaitingAgentRun(user.OrganizationID, user.ID, runID); err != nil { return model.AgentRun{}, err }
	if request.Status != "success" && request.Status != "failed" { return model.AgentRun{}, safeMessageError{message: "工具结果状态无效"} }
	if request.Status == "failed" {
		if request.Error == "" { return model.AgentRun{}, safeMessageError{message: "工具失败时必须返回错误"} }
		if len(request.Images) != 0 || request.Video != nil || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" || request.Plan != nil {
			return model.AgentRun{}, safeMessageError{message: "工具失败结果无效"}
		}
	} else if request.Error != "" {
		return model.AgentRun{}, safeMessageError{message: "工具成功时不能返回错误"}
	}

	run, err := repository.GetAgentRun(user.OrganizationID, user.ID, runID)
	if err != nil { return model.AgentRun{}, err }
	toolStep, err := repository.GetAgentToolStep(user.OrganizationID, user.ID, runID, request.CallID)
	if err != nil { return model.AgentRun{}, err }
	authorization, err := agentRunNodeAuthorization(run)
	if err != nil { return model.AgentRun{}, err }
	toolArguments, _, err := decodeAgentToolArguments(toolStep.ToolName, toolStep.Input, authorization)
	if err != nil { return model.AgentRun{}, err }
	if request.Status == "success" {
		if err := validateAgentToolSuccess(toolStep.ToolName, toolArguments, &request); err != nil { return model.AgentRun{}, err }
	}
	executionToken := request.ExecutionToken
	request.ExecutionToken = ""
	output, err := json.Marshal(request)
	if err != nil { return model.AgentRun{}, err }
	if len(output) > 64<<10 { return model.AgentRun{}, safeMessageError{message: "工具结果过大"} }
	timestamp := now()
	completedPayload, _ := json.Marshal(map[string]any{"callId": request.CallID, "name": toolStep.ToolName, "status": request.Status, "output": agentToolResultOutput(toolStep.ToolName, request)})
	failedPayload, _ := json.Marshal(map[string]string{"error": request.Error})
	completionStep := model.AgentStep{ID: newID("agent-step"), OrganizationID: user.OrganizationID, UserID: user.ID, RunID: runID, Type: model.AgentStepTypeCompletion, Status: model.AgentStepStatusRunning, Input: string(output), StartedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp}
	run, _, resume, err := repository.SubmitAgentToolResult(
		user.OrganizationID, user.ID, runID, request.CallID, executionToken, string(output), request.Error, timestamp, request.Status == "success",
		model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolCompleted, Payload: string(completedPayload), CreatedAt: timestamp},
		model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventRunFailed, Payload: string(failedPayload), CreatedAt: timestamp}, completionStep,
	)
	if errors.Is(err, repository.ErrAgentToolResultConflict) { return model.AgentRun{}, safeMessageError{message: "工具结果与已保存回执不一致"} }
	if errors.Is(err, repository.ErrAgentToolExecutionClaimed) { return model.AgentRun{}, safeMessageError{message: "工具执行租约已失效"} }
	if err != nil { return model.AgentRun{}, err }
	if resume { startAgentRun(run, user.Group) }
	return run, nil
}

func ClaimAgentToolExecution(user model.AuthUser, runID string, request ClaimAgentToolRequest) error {
	if err := RequireOrganizationWrite(user); err != nil { return err }
	runID, request.CallID, request.Token = strings.TrimSpace(runID), strings.TrimSpace(request.CallID), strings.TrimSpace(request.Token)
	if runID == "" || request.CallID == "" || request.Token == "" || len(request.Token) > 191 {
		return safeMessageError{message: "工具执行认领参数无效"}
	}
	if err := expireWaitingAgentRun(user.OrganizationID, user.ID, runID); err != nil { return err }
	run, err := repository.GetAgentRun(user.OrganizationID, user.ID, runID)
	if err != nil { return err }
	if run.Status != model.AgentRunStatusWaitingTool { return safeMessageError{message: "当前运行不再等待工具结果"} }
	if _, err := repository.GetAgentToolStep(user.OrganizationID, user.ID, runID, request.CallID); err != nil { return err }
	timestamp := now()
	deadline := time.Now().UTC().Add(-agentToolExecutionLease).Format(timestampLayout)
	if err := repository.ClaimAgentToolExecution(user.OrganizationID, user.ID, runID, request.CallID, request.Token, timestamp, deadline); errors.Is(err, repository.ErrAgentToolExecutionClaimed) {
		return safeMessageError{message: "该工具正在另一个页面执行"}
	} else {
		return err
	}
}

func GetAgentToolResultReceipt(user model.AuthUser, runID, callID string) (AgentToolResultReceipt, error) {
	runID, callID = strings.TrimSpace(runID), strings.TrimSpace(callID)
	if runID == "" || callID == "" { return AgentToolResultReceipt{}, safeMessageError{message: "运行或工具调用编号无效"} }
	if err := expireWaitingAgentRun(user.OrganizationID, user.ID, runID); err != nil { return AgentToolResultReceipt{}, err }
	run, err := repository.GetAgentRun(user.OrganizationID, user.ID, runID)
	if err != nil { return AgentToolResultReceipt{}, err }
	step, err := repository.GetAgentToolStep(user.OrganizationID, user.ID, runID, callID)
	if err != nil { return AgentToolResultReceipt{}, err }
	if step.Confirmation == "rejected" { return AgentToolResultReceipt{Status: "rejected"}, nil }
	if step.Output != "" {
		var result SubmitAgentToolResultRequest
		if err := json.Unmarshal([]byte(step.Output), &result); err == nil && (result.Status == "success" || result.Status == "failed") {
			return AgentToolResultReceipt{Status: "completed", Result: &result}, nil
		}
	}
	if step.Status == model.AgentStepStatusCancelled || run.Status == model.AgentRunStatusCancelled {
		return AgentToolResultReceipt{Status: "cancelled"}, nil
	}
	if step.Status == model.AgentStepStatusFailed || run.Status == model.AgentRunStatusFailed {
		return AgentToolResultReceipt{Status: "failed"}, nil
	}
	return AgentToolResultReceipt{Status: "pending"}, nil
}

func ConfirmAgentTool(user model.AuthUser, runID string, request ConfirmAgentToolRequest) (model.AgentRun, error) {
	if err := RequireOrganizationWrite(user); err != nil { return model.AgentRun{}, err }
	runID, request.CallID, request.Decision = strings.TrimSpace(runID), strings.TrimSpace(request.CallID), strings.TrimSpace(request.Decision)
	if runID == "" || request.CallID == "" { return model.AgentRun{}, safeMessageError{message: "运行或工具调用编号无效"} }
	if request.Decision != "approved" && request.Decision != "rejected" { return model.AgentRun{}, safeMessageError{message: "工具确认决定无效"} }

	run, err := repository.GetAgentRun(user.OrganizationID, user.ID, runID)
	if err != nil { return model.AgentRun{}, err }
	toolStep, err := repository.GetAgentToolStep(user.OrganizationID, user.ID, runID, request.CallID)
	if err != nil { return model.AgentRun{}, err }
	if toolStep.ToolName != "canvas.delete" && toolStep.ToolName != "canvas.update_text" {
		return model.AgentRun{}, safeMessageError{message: "该工具无需确认"}
	}
	authorization, err := agentRunNodeAuthorization(run)
	if err != nil { return model.AgentRun{}, err }
	toolArguments, _, err := decodeAgentToolArguments(toolStep.ToolName, toolStep.Input, authorization)
	if err != nil { return model.AgentRun{}, err }

	rejectedObservation, err := json.Marshal(map[string]string{"status": "rejected", "error": "用户拒绝执行该工具调用"})
	if err != nil { return model.AgentRun{}, err }
	timestamp := now()
	approvedPayload, _ := json.Marshal(map[string]any{"callId": request.CallID, "name": toolStep.ToolName, "arguments": toolArguments})
	rejectedPayload, _ := json.Marshal(map[string]any{"callId": request.CallID, "name": toolStep.ToolName, "status": "rejected", "output": map[string]string{"error": "用户拒绝执行该工具调用"}})
	completionStep := model.AgentStep{ID: newID("agent-step"), OrganizationID: user.OrganizationID, UserID: user.ID, RunID: runID, Type: model.AgentStepTypeCompletion, Status: model.AgentStepStatusRunning, Input: string(rejectedObservation), StartedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp}
	run, _, approved, resume, err := repository.ConfirmAgentTool(
		user.OrganizationID, user.ID, runID, request.CallID, request.Decision, string(rejectedObservation), timestamp,
		model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolCall, Payload: string(approvedPayload), CreatedAt: timestamp},
		model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolCompleted, Payload: string(rejectedPayload), CreatedAt: timestamp}, completionStep,
	)
	if err != nil { return model.AgentRun{}, err }
	if approved {
		time.AfterFunc(agentWaitingToolTimeout, func() { _ = expireWaitingAgentRun(user.OrganizationID, user.ID, runID) })
	}
	if resume {
		startAgentRun(run, user.Group)
	}
	return run, nil
}

func GetAgentRun(user model.AuthUser, runID string) (model.AgentRun, error) {
	runID = strings.TrimSpace(runID)
	if err := expireWaitingAgentRun(user.OrganizationID, user.ID, runID); err != nil { return model.AgentRun{}, err }
	return repository.GetAgentRun(user.OrganizationID, user.ID, runID)
}

func ListAgentEvents(user model.AuthUser, runID string, after int64) ([]model.AgentEvent, error) {
	runID = strings.TrimSpace(runID)
	if err := expireWaitingAgentRun(user.OrganizationID, user.ID, runID); err != nil { return nil, err }
	if _, err := repository.GetAgentRun(user.OrganizationID, user.ID, runID); err != nil { return nil, err }
	return repository.ListAgentEvents(user.OrganizationID, user.ID, runID, after, 100)
}

func CancelAgentRun(user model.AuthUser, runID string) (model.AgentRun, error) {
	runID = strings.TrimSpace(runID)
	run, cancelled, err := repository.CancelAgentRun(user.OrganizationID, user.ID, runID, newID("agent-event"), now())
	if err != nil { return model.AgentRun{}, err }
	if cancelled {
		agentRuns.Lock()
		execution := agentRuns.cancels[runID]
		agentRuns.Unlock()
		if execution.cancel != nil { execution.cancel() }
	}
	return run, nil
}

func RevertAgentTool(user model.AuthUser, runID string, request RevertAgentToolRequest) (model.AgentRun, error) {
	if err := RequireOrganizationWrite(user); err != nil { return model.AgentRun{}, err }
	runID, request.CallID = strings.TrimSpace(runID), strings.TrimSpace(request.CallID)
	if runID == "" || request.CallID == "" { return model.AgentRun{}, safeMessageError{message: "运行或工具调用编号无效"} }
	step, err := repository.GetAgentToolStep(user.OrganizationID, user.ID, runID, request.CallID)
	if err != nil { return model.AgentRun{}, err }
	switch step.ToolName {
	case "image.generate", "video.generate", "canvas.arrange", "canvas.add_text", "canvas.delete", "canvas.update_text":
	default:
		return model.AgentRun{}, safeMessageError{message: "该工具没有可撤销的画布操作"}
	}
	timestamp := now()
	revertedPayload, _ := json.Marshal(map[string]string{"callId": request.CallID, "name": step.ToolName})
	run, err := repository.RevertAgentTool(
		user.OrganizationID, user.ID, runID, request.CallID, timestamp,
		model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolReverted, Payload: string(revertedPayload), CreatedAt: timestamp},
		model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventRunCancelled, Payload: `{"reason":"tool_reverted"}`, CreatedAt: timestamp},
	)
	if errors.Is(err, repository.ErrAgentToolNotRevertible) { return model.AgentRun{}, safeMessageError{message: "该工具调用当前不能撤销"} }
	if err != nil { return model.AgentRun{}, err }
	agentRuns.Lock()
	execution := agentRuns.cancels[runID]
	agentRuns.Unlock()
	if execution.cancel != nil { execution.cancel() }
	return run, nil
}

func startAgentRun(run model.AgentRun, userGroup string) {
	toolSteps, err := repository.ListCompletedAgentToolSteps(run.OrganizationID, run.UserID, run.ID)
	if err != nil { return }
	requestID := fmt.Sprintf("%s-completion-%d", run.ID, len(toolSteps)+1)
	timestamp := time.Now().UTC()
	if err := repository.ClaimAgentRunExecution(run.OrganizationID, run.UserID, run.ID, newID("agent-execution"), timestamp.Format(timestampLayout), timestamp.Add(-agentRunningTimeout).Format(timestampLayout)); err != nil { return }
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	agentRuns.Lock()
	agentRuns.cancels[run.ID] = agentRunExecution{requestID: requestID, cancel: cancel}
	agentRuns.Unlock()
	saved, err := repository.GetAgentRun(run.OrganizationID, run.UserID, run.ID)
	if err != nil || saved.Status != model.AgentRunStatusRunning {
		cancel()
		agentRuns.Lock()
		if execution := agentRuns.cancels[run.ID]; execution.requestID == requestID { delete(agentRuns.cancels, run.ID) }
		agentRuns.Unlock()
		return
	}
	go executeAgentRun(ctx, cancel, run, userGroup, requestID)
}

func executeAgentRun(ctx context.Context, cancel context.CancelFunc, run model.AgentRun, userGroup, requestID string) {
	defer func() {
		cancel()
		agentRuns.Lock()
		if execution := agentRuns.cancels[run.ID]; execution.requestID == requestID { delete(agentRuns.cancels, run.ID) }
		agentRuns.Unlock()
	}()

	completion, err := requestAgentCompletion(ctx, run, userGroup, requestID)
	if err != nil { failAgentRunUnlessCancelled(run, err); return }
	if completion.ToolCall != nil {
		payload, _ := json.Marshal(map[string]any{"callId": completion.ToolCall.ID, "name": completion.ToolCall.Name, "arguments": completion.ToolCall.Arguments})
		timestamp := now()
		step := model.AgentStep{ID: newID("agent-step"), OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeTool, Status: model.AgentStepStatusRunning, ToolCallID: completion.ToolCall.ID, ToolName: completion.ToolCall.Name, Input: completion.ToolCall.Raw, StartedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp}
		confirmationRequired := completion.ToolCall.Name == "canvas.delete" || completion.ToolCall.Name == "canvas.update_text"
		if confirmationRequired {
			event := model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolConfirmationRequired, Payload: string(payload), CreatedAt: timestamp}
			if err := repository.WaitAgentRunForConfirmation(run.OrganizationID, run.UserID, run.ID, completion.Content, step, event, timestamp); err != nil {
				failAgentRunUnlessCancelled(run, err)
			}
		} else {
			events := []model.AgentEvent{}
			if completion.ToolCall.Name == "canvas.plan" {
				events = append(events, model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventPlanCreated, Payload: string(payload), CreatedAt: timestamp})
			}
			if completion.ToolCall.Name == "canvas.arrange" {
				argumentPayload, _ := json.Marshal(map[string]any{"callId": completion.ToolCall.ID, "name": completion.ToolCall.Name, "arguments": completion.ToolCall.Arguments, "meta": map[string]any{"needsClaim": false}})
				events = append(events, model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolCall, Payload: string(argumentPayload), CreatedAt: timestamp})
			} else {
				events = append(events, model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolCall, Payload: string(payload), CreatedAt: timestamp})
			}
			if err := repository.WaitAgentRunForTool(run.OrganizationID, run.UserID, run.ID, completion.Content, step, timestamp, events...); err != nil {
				failAgentRunUnlessCancelled(run, err)
			} else {
				time.AfterFunc(agentWaitingToolTimeout, func() { _ = expireWaitingAgentRun(run.OrganizationID, run.UserID, run.ID) })
			}
		}
		return
	}
	payload, _ := json.Marshal(map[string]string{"content": completion.Content})
	if err := repository.CompleteAgentRun(run.OrganizationID, run.UserID, run.ID, newID("agent-message"), newID("agent-event"), newID("agent-event"), completion.Content, string(payload), now()); err != nil {
		failedPayload, _ := json.Marshal(map[string]string{"error": "助手结果保存失败"})
		_ = repository.FailAgentRun(run.OrganizationID, run.UserID, run.ID, newID("agent-event"), "助手结果保存失败", string(failedPayload), now())
	}
}

func failAgentRunUnlessCancelled(run model.AgentRun, err error) {
	if saved, getErr := repository.GetAgentRun(run.OrganizationID, run.UserID, run.ID); getErr == nil && saved.Status == model.AgentRunStatusCancelled { return }
	message := "助手请求失败，请重试"
	if errors.Is(err, context.DeadlineExceeded) {
		message = "助手请求超时，请重试"
	} else if strings.Contains(strings.ToLower(err.Error()), "status 429") || strings.Contains(strings.ToLower(err.Error()), "status 5") {
		message = "助手服务暂时不可用，请重试"
	} else if strings.Contains(strings.ToLower(err.Error()), "connection reset") || strings.Contains(strings.ToLower(err.Error()), "unexpected eof") {
		message = "助手服务连接中断，请重试"
	}
	payload, _ := json.Marshal(map[string]string{"error": message})
	_ = repository.FailAgentRun(run.OrganizationID, run.UserID, run.ID, newID("agent-event"), message, string(payload), now())
}

func expireWaitingAgentRun(organizationID, userID, runID string) error {
	timestamp := time.Now().UTC()
	waitingPayload, _ := json.Marshal(map[string]string{"error": agentWaitingToolTimeoutError})
	if _, _, err := repository.ExpireWaitingAgentRun(organizationID, userID, runID, newID("agent-event"), agentWaitingToolTimeoutError, string(waitingPayload), timestamp.Format(timestampLayout), timestamp.Add(-agentWaitingToolTimeout).Format(timestampLayout)); err != nil { return err }
	agentRuns.Lock()
	_, active := agentRuns.cancels[runID]
	agentRuns.Unlock()
	if active { return nil }
	runningPayload, _ := json.Marshal(map[string]string{"error": agentRunningTimeoutError})
	_, _, err := repository.ExpireRunningAgentRun(organizationID, userID, runID, newID("agent-event"), agentRunningTimeoutError, string(runningPayload), timestamp.Format(timestampLayout), timestamp.Add(-agentRunningTimeout).Format(timestampLayout))
	return err
}

func requestAgentCompletion(ctx context.Context, run model.AgentRun, userGroup, requestID string) (completion agentCompletion, err error) {
	messages, err := repository.ListRecentAgentMessages(run.OrganizationID, run.UserID, run.SessionID, 30)
	if err != nil { return completion, err }
	toolSteps, err := repository.ListCompletedAgentToolSteps(run.OrganizationID, run.UserID, run.ID)
	if err != nil { return completion, err }
	if routed, handled, routeErr := routeDeterministicAgentCompletion(run, messages, toolSteps); handled || routeErr != nil { return routed, routeErr }
	pricingRequest := PricingRequest{Model: run.Model, Modality: "text", Operation: "completion", Unit: "request", Quantity: 1}
	selection, err := SelectModelChannel(pricingRequest)
	if err != nil { return completion, err }
	credits, err := CalculateRequestCreditsForGroup(pricingRequest, userGroup)
	if err != nil { return completion, err }
	task, err := BeginGenerationTask(GenerationTaskInput{
		UserID: run.UserID, OrganizationID: run.OrganizationID, RequestID: requestID,
		Model: run.Model, UpstreamModel: selection.Model.UpstreamModel, ChannelName: selection.Channel.Name,
		Path: "/chat/completions", Modality: pricingRequest.Modality, Operation: pricingRequest.Operation,
		Quantity: pricingRequest.Quantity, Credits: credits,
	})
	if err != nil { return completion, err }
	defer func() {
		status, message := model.GenerationTaskStatusSuccess, ""
		if err != nil { status, message = model.GenerationTaskStatusFailed, err.Error() }
		if finishErr := FinishGenerationTask(task, status, message); finishErr != nil && err == nil { completion, err = agentCompletion{}, finishErr }
	}()

	requestMessages := make([]agentChatMessage, 0, len(messages)+4)
	requestMessages = append(requestMessages,
		agentChatMessage{Role: "system", Content: agentSystemPrompt},
		agentChatMessage{Role: "system", Content: run.Context},
	)
	if len(toolSteps) > 0 {
		completed := make([]map[string]any, 0, len(toolSteps))
		for _, step := range toolSteps {
			var arguments, result any
			if json.Unmarshal([]byte(step.Input), &arguments) != nil { arguments = step.Input }
			if json.Unmarshal([]byte(step.Output), &result) != nil { result = step.Output }
			completed = append(completed, map[string]any{"name": step.ToolName, "arguments": arguments, "result": result})
		}
		completedPayload, _ := json.Marshal(completed)
		requestMessages = append(requestMessages, agentChatMessage{Role: "system", Content: "本轮已完成的工具及真实 TOOL_RESULT："+string(completedPayload)+"。继续最后一条用户请求；仍需工具时只输出下一条 TOOL_CALL，否则立即简洁总结。"})
	}
	currentIndex := len(messages)-1
	for i := range messages {
		if messages[i].ID == run.MessageID { currentIndex = i; break }
	}
	conversationStart := currentIndex
	for i := currentIndex-1; i >= 1; i -= 2 {
		if messages[i].Role != model.AgentMessageRoleAssistant || messages[i-1].Role != model.AgentMessageRoleUser { break }
		conversationStart = i-1
	}
	for _, message := range messages[conversationStart:currentIndex+1] { requestMessages = append(requestMessages, agentChatMessage{Role: string(message.Role), Content: message.Content}) }
	if len(toolSteps) >= agentMaxToolCalls {
		requestMessages = append(requestMessages, agentChatMessage{Role: "system", Content: "本次运行已达到工具调用上限，请根据已有真实结果直接总结，不要再请求工具。"})
	}
	bodyValue := map[string]any{"model": selection.Model.UpstreamModel, "messages": requestMessages, "stream": false, "max_tokens": 256}
	body, err := json.Marshal(bodyValue)
	if err != nil { return completion, err }
	client := &http.Client{Timeout: 2 * time.Minute}
	var response *http.Response
	for attempt := 0; attempt < 2; attempt++ {
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, BuildModelChannelURL(selection.Channel, "/chat/completions"), bytes.NewReader(body))
		if requestErr != nil { return completion, requestErr }
		request.Header.Set("Authorization", "Bearer "+selection.Channel.APIKey)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Request-ID", requestID)
		request.Header.Set("Idempotency-Key", requestID)
		response, err = client.Do(request)
		if err == nil { break }
		if attempt > 0 || ctx.Err() != nil { return completion, err }
		select {
		case <-ctx.Done(): return completion, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil { return completion, err }
	if response.StatusCode < 200 || response.StatusCode >= 300 { return completion, fmt.Errorf("agent upstream returned status %d", response.StatusCode) }
	var result struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct { Name string `json:"name"`; Arguments string `json:"arguments"` } `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil { return completion, err }
	if len(result.Choices) == 0 { return completion, errors.New("agent upstream returned no choices") }
	message := result.Choices[0].Message
	completion.Content = strings.TrimSpace(message.Content)
	if len(message.ToolCalls) > 1 { return completion, errors.New("agent upstream returned multiple tool calls") }
	if len(message.ToolCalls) == 1 {
		if len(toolSteps) >= agentMaxToolCalls { return completion, errors.New("agent exceeded tool call limit") }
		call := message.ToolCalls[0]
		if call.Type != "function" { return completion, errors.New("agent upstream returned unsupported tool call") }
		if call.Function.Name == "canvas.plan" && len(toolSteps) != 0 { return completion, errors.New("agent requested a duplicate plan") }
		authorization, authorizationErr := agentRunNodeAuthorization(run)
		if authorizationErr != nil { return completion, authorizationErr }
		arguments, raw, decodeErr := decodeAgentToolArguments(call.Function.Name, call.Function.Arguments, authorization)
		if decodeErr != nil { return completion, decodeErr }
		if completedAgentToolCall(toolSteps, call.Function.Name, raw) { completion.Content = "请求的画布操作已完成。"; return completion, nil }
		call.ID = newID("agent-tool-call")
		completion.ToolCall = &agentToolCall{ID: call.ID, Name: call.Function.Name, Arguments: arguments, Raw: raw}
		return completion, nil
	}
	if textName, textArguments, ok := parseAgentTextToolCall(completion.Content); ok {
		if len(toolSteps) >= agentMaxToolCalls { return completion, errors.New("agent exceeded tool call limit") }
		if textName == "canvas.plan" && len(toolSteps) != 0 { return completion, errors.New("agent requested a duplicate plan") }
		authorization, authorizationErr := agentRunNodeAuthorization(run)
		if authorizationErr != nil { return completion, authorizationErr }
		arguments, raw, decodeErr := decodeAgentToolArguments(textName, textArguments, authorization)
		if decodeErr != nil { return completion, decodeErr }
		if completedAgentToolCall(toolSteps, textName, raw) { completion.Content = "请求的画布操作已完成。"; return completion, nil }
		completion.ToolCall = &agentToolCall{ID: newID("agent-tool-call"), Name: textName, Arguments: arguments, Raw: raw}
		return completion, nil
	}
	if textName, textArguments, ok := parseAgentNaturalLanguageToolCall(completion.Content); ok {
		if len(toolSteps) >= agentMaxToolCalls { return completion, errors.New("agent exceeded tool call limit") }
		if textName == "canvas.plan" && len(toolSteps) != 0 { return completion, errors.New("agent requested a duplicate plan") }
		authorization, authorizationErr := agentRunNodeAuthorization(run)
		if authorizationErr != nil { return completion, authorizationErr }
		arguments, raw, decodeErr := decodeAgentToolArguments(textName, textArguments, authorization)
		if decodeErr != nil { return completion, decodeErr }
		if completedAgentToolCall(toolSteps, textName, raw) { completion.Content = "请求的画布操作已完成。"; return completion, nil }
		completion.ToolCall = &agentToolCall{ID: newID("agent-tool-call"), Name: textName, Arguments: arguments, Raw: raw}
		return completion, nil
	}
	if completion.Content == "" { return completion, errors.New("agent upstream returned empty content") }
	return completion, nil
}

func routeDeterministicAgentCompletion(run model.AgentRun, messages []model.AgentMessage, toolSteps []model.AgentStep) (agentCompletion, bool, error) {
	content := ""
	for _, message := range messages {
		if message.ID == run.MessageID && message.Role == model.AgentMessageRoleUser { content = strings.TrimSpace(message.Content); break }
	}
	if content == "" { return agentCompletion{}, false, nil }
	var canvasContext AgentCanvasContext
	if err := json.Unmarshal([]byte(run.Context), &canvasContext); err != nil { return agentCompletion{}, true, errors.New("agent canvas context invalid") }

	if containsAgentPhrase(content, "多少节点", "多少个节点", "几个节点", "节点数量") {
		return agentCompletion{Content: fmt.Sprintf("当前画布有 %d 个节点。", len(canvasContext.Nodes))}, true, nil
	}
	if text, placement, ok := parseExplicitAddTextRequest(content); ok {
		return deterministicAgentToolCompletion(run, toolSteps, "canvas.add_text", canvasAddTextArguments{Text: text, Placement: placement}, "文本已添加到画布。")
	}

	lower := strings.ToLower(content)
	mediaNegated := containsAgentPhrase(lower, "不要生成媒体", "不要生成图片", "不要生图", "别生成图片", "无需生成图片", "不要生成视频", "别生成视频", "无需生成视频")
	videoIntent := !mediaNegated && containsAgentPhrase(lower, "生成视频", "生成短视频", "制作视频", "制作短视频")
	imageIntent := !mediaNegated && !videoIntent && containsAgentPhrase(lower, "生成图片", "生成一张图", "生成一张图片", "生图", "商品主图", "电商主图", "场景图", "海报")
	if !imageIntent && !videoIntent { return agentCompletion{}, false, nil }

	authorization, err := agentRunNodeAuthorization(run)
	if err != nil { return agentCompletion{}, true, err }
	planned := containsAgentPhrase(content, "策划", "规划", "计划")
	if planned {
		kind := "图片"
		if videoIntent { kind = "视频" }
		arguments, raw, err := normalizeDeterministicAgentToolArguments("canvas.plan", canvasPlanArguments{
			Summary: "策划并生成" + kind,
			Steps: []string{"明确" + kind + "视觉目标", "生成" + kind + "并添加到画布"},
		}, authorization)
		if err != nil { return agentCompletion{}, true, err }
		if !completedAgentToolCall(toolSteps, "canvas.plan", raw) {
			return agentCompletion{ToolCall: &agentToolCall{ID: newID("agent-tool-call"), Name: "canvas.plan", Arguments: arguments, Raw: raw}}, true, nil
		}
	}

	selectedImages := selectedAgentImageNodeIDs(canvasContext)
	if videoIntent {
		imageNodeID := ""
		if len(selectedImages) > 0 { imageNodeID = selectedImages[0] }
		return deterministicAgentToolCompletionWithAuthorization(toolSteps, "video.generate", videoGenerateArguments{Prompt: content, Duration: 6, ImageNodeID: imageNodeID}, "视频已生成并添加到画布。", authorization)
	}
	if len(selectedImages) > 6 { selectedImages = selectedImages[:6] }
	return deterministicAgentToolCompletionWithAuthorization(toolSteps, "image.generate", imageGenerateArguments{Prompt: content, Count: 1, ReferenceNodeIDs: selectedImages}, "图片已生成并添加到画布。", authorization)
}

func deterministicAgentToolCompletion(run model.AgentRun, steps []model.AgentStep, name string, input any, completedText string) (agentCompletion, bool, error) {
	authorization, err := agentRunNodeAuthorization(run)
	if err != nil { return agentCompletion{}, true, err }
	return deterministicAgentToolCompletionWithAuthorization(steps, name, input, completedText, authorization)
}

func deterministicAgentToolCompletionWithAuthorization(steps []model.AgentStep, name string, input any, completedText string, authorization agentNodeAuthorization) (agentCompletion, bool, error) {
	arguments, raw, err := normalizeDeterministicAgentToolArguments(name, input, authorization)
	if err != nil { return agentCompletion{}, true, err }
	if completedAgentToolCall(steps, name, raw) { return agentCompletion{Content: completedText}, true, nil }
	return agentCompletion{ToolCall: &agentToolCall{ID: newID("agent-tool-call"), Name: name, Arguments: arguments, Raw: raw}}, true, nil
}

func normalizeDeterministicAgentToolArguments(name string, input any, authorization agentNodeAuthorization) (any, string, error) {
	raw, err := json.Marshal(input)
	if err != nil { return nil, "", err }
	return decodeAgentToolArguments(name, string(raw), authorization)
}

func parseExplicitAddTextRequest(content string) (string, string, bool) {
	if !strings.Contains(strings.ToLower(content), "canvas.add_text") { return "", "", false }
	match := regexp.MustCompile(`(?i)(?:参数\s*)?text\s*(?:为|是|=|:|：)\s*[“"']([^”"']+)[”"']`).FindStringSubmatch(content)
	if len(match) < 2 {
		match = regexp.MustCompile(`(?i)(?:参数\s*)?text\s*(?:为|是|=|:|：)\s*([^,，;；\n]+)`).FindStringSubmatch(content)
	}
	if len(match) < 2 || strings.TrimSpace(match[1]) == "" { return "", "", false }
	placement := "center"
	if placementMatch := regexp.MustCompile(`(?i)placement\s*(?:为|是|=|:|：)\s*(center|right_of_selection)`).FindStringSubmatch(content); len(placementMatch) > 1 {
		placement = strings.ToLower(placementMatch[1])
	}
	return strings.TrimSpace(match[1]), placement, true
}

func selectedAgentImageNodeIDs(canvasContext AgentCanvasContext) []string {
	images := make(map[string]struct{})
	for _, node := range canvasContext.Nodes {
		if node.Type == "image" { images[node.ID] = struct{}{} }
	}
	result := make([]string, 0, len(canvasContext.SelectedNodeIDs))
	for _, id := range canvasContext.SelectedNodeIDs {
		if _, valid := images[id]; valid { result = append(result, id) }
	}
	return result
}

func containsAgentPhrase(content string, phrases ...string) bool {
	for _, phrase := range phrases {
		if strings.Contains(content, phrase) { return true }
	}
	return false
}

func completedAgentToolCall(steps []model.AgentStep, name, input string) bool {
	for _, step := range steps {
		if step.ToolName == name && step.Input == input { return true }
	}
	return false
}

func normalizeAgentCanvasContext(value AgentCanvasContext) (AgentCanvasContext, error) {
	if len(value.SelectedNodeIDs) > 20 { return AgentCanvasContext{}, safeMessageError{message: "选中节点过多"} }
	if len(value.Nodes) > 200 { return AgentCanvasContext{}, safeMessageError{message: "画布节点过多"} }
	selected := make(map[string]struct{}, len(value.SelectedNodeIDs))
	selectedIDs := make([]string, 0, len(value.SelectedNodeIDs))
	for _, id := range value.SelectedNodeIDs {
		id = strings.TrimSpace(id)
		if id == "" { return AgentCanvasContext{}, safeMessageError{message: "选中节点编号无效"} }
		if _, exists := selected[id]; exists { return AgentCanvasContext{}, safeMessageError{message: "选中节点编号重复"} }
		selected[id] = struct{}{}
		selectedIDs = append(selectedIDs, id)
	}
	nodes := make(map[string]AgentCanvasNode, len(value.Nodes))
	nodeIDs := make([]string, 0, len(value.Nodes))
	for _, node := range value.Nodes {
		node.ID = strings.TrimSpace(node.ID)
		if node.ID == "" { return AgentCanvasContext{}, safeMessageError{message: "画布节点编号无效"} }
		if _, exists := nodes[node.ID]; exists { return AgentCanvasContext{}, safeMessageError{message: "画布节点编号重复"} }
		node.Type, node.Title = strings.TrimSpace(node.Type), strings.TrimSpace(node.Title)
		if node.Title == "" { return AgentCanvasContext{}, safeMessageError{message: "画布节点标题无效"} }
		node.Content, node.Prompt, node.StorageKey = strings.TrimSpace(node.Content), strings.TrimSpace(node.Prompt), strings.TrimSpace(node.StorageKey)
		if utf8.RuneCountInString(node.Content) > 4000 || utf8.RuneCountInString(node.Prompt) > 2000 || utf8.RuneCountInString(node.StorageKey) > 512 {
			return AgentCanvasContext{}, safeMessageError{message: "画布节点内容过大"}
		}
		if !finite(node.X) || !finite(node.Y) || !finite(node.Width) || !finite(node.Height) || math.Abs(node.X) > 1e7 || math.Abs(node.Y) > 1e7 || node.Width <= 0 || node.Height <= 0 || node.Width > 100000 || node.Height > 100000 {
			return AgentCanvasContext{}, safeMessageError{message: "画布节点尺寸或坐标无效"}
		}
		seenReferences := make(map[string]struct{}, len(node.References))
		for i, reference := range node.References {
			reference = strings.TrimSpace(reference)
			if reference == "" { return AgentCanvasContext{}, safeMessageError{message: "画布节点来源编号无效"} }
			if _, exists := seenReferences[reference]; exists { return AgentCanvasContext{}, safeMessageError{message: "画布节点来源编号重复"} }
			seenReferences[reference] = struct{}{}
			node.References[i] = reference
		}
		nodes[node.ID] = node
		nodeIDs = append(nodeIDs, node.ID)
	}
	connections := make([]AgentCanvasConnection, 0, len(value.Connections))
	seenConnections := make(map[string]struct{}, len(value.Connections))
	for _, connection := range value.Connections {
		connection.From, connection.To = strings.TrimSpace(connection.From), strings.TrimSpace(connection.To)
		if connection.From == "" || connection.To == "" { continue }
		if _, exists := nodes[connection.From]; !exists { continue }
		if _, exists := nodes[connection.To]; !exists { continue }
		key := connection.From + ">" + connection.To
		if _, exists := seenConnections[key]; exists { continue }
		seenConnections[key] = struct{}{}
		connections = append(connections, connection)
	}
	validSelectedIDs := make([]string, 0, len(selectedIDs))
	for _, id := range selectedIDs {
		if _, exists := nodes[id]; exists { validSelectedIDs = append(validSelectedIDs, id) }
	}
	result := AgentCanvasContext{SelectedNodeIDs: validSelectedIDs, Nodes: make([]AgentCanvasNode, 0, len(nodes)), Connections: connections}
	for _, id := range nodeIDs {
		node, exists := nodes[id]
		if !exists { continue }
		result.Nodes = append(result.Nodes, node)
	}
	return result, nil
}

func agentRunNodeAuthorization(run model.AgentRun) (agentNodeAuthorization, error) {
	authorization := agentNodeAuthorization{NodeIDs: map[string]struct{}{}, ImageNodeIDs: map[string]struct{}{}, TextNodeIDs: map[string]struct{}{}}
	var canvasContext AgentCanvasContext
	if err := json.Unmarshal([]byte(run.Context), &canvasContext); err != nil { return authorization, errors.New("agent canvas context invalid") }
	for _, node := range canvasContext.Nodes {
		authorization.NodeIDs[node.ID] = struct{}{}
		if node.Type == "image" { authorization.ImageNodeIDs[node.ID] = struct{}{} }
		if node.Type == "text" { authorization.TextNodeIDs[node.ID] = struct{}{} }
	}
	steps, err := repository.ListCompletedAgentToolSteps(run.OrganizationID, run.UserID, run.ID)
	if err != nil { return authorization, err }
	for _, step := range steps {
		var result SubmitAgentToolResultRequest
		if json.Unmarshal([]byte(step.Output), &result) != nil || result.Status != "success" { continue }
		for _, image := range result.Images {
			authorization.NodeIDs[image.NodeID], authorization.ImageNodeIDs[image.NodeID] = struct{}{}, struct{}{}
		}
		if result.Video != nil { authorization.NodeIDs[result.Video.NodeID] = struct{}{} }
		if result.NodeID != "" {
			authorization.NodeIDs[result.NodeID] = struct{}{}
			if step.ToolName == "canvas.add_text" { authorization.TextNodeIDs[result.NodeID] = struct{}{} }
		}
		for _, id := range result.NodeIDs { authorization.NodeIDs[id] = struct{}{} }
	}
	return authorization, nil
}

func parseAgentTextToolCall(content string) (string, string, bool) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < len("TOOL_CALL ") || !strings.EqualFold(line[:len("TOOL_CALL ")], "TOOL_CALL ") { continue }
		payload := strings.TrimSpace(line[len("TOOL_CALL "):])
		var call struct {
			Name      string          `json:"name"`
			Action    string          `json:"action"`
			Tool      string          `json:"tool"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal([]byte(payload), &call) != nil { continue }
		if call.Name == "" { call.Name = call.Action }
		if call.Name == "" { call.Name = call.Tool }
		if call.Name == "" { continue }
		if len(call.Arguments) == 0 {
			var flat map[string]json.RawMessage
			if json.Unmarshal([]byte(payload), &flat) != nil { continue }
			delete(flat, "name")
			delete(flat, "action")
			delete(flat, "tool")
			call.Arguments, _ = json.Marshal(flat)
		}
		if len(call.Arguments) == 0 { continue }
		return call.Name, string(call.Arguments), true
	}
	return "", "", false
}

func parseAgentNaturalLanguageToolCall(content string) (string, string, bool) {
	callPattern := regexp.MustCompile(`(?i)\b(?:tool\s+)?call\s+((?:canvas|image|video)\.[a-z_]+)\s+with\s+(.*)`)
	quotedPattern := regexp.MustCompile(`"([^"]*)"|'([^']*)'`)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		match := callPattern.FindStringSubmatch(line)
		if len(match) < 3 { continue }
		name := strings.ToLower(strings.TrimSpace(match[1]))
		rest := match[2]
		switch name {
		case "canvas.plan":
			summaryMatch := regexp.MustCompile(`(?i)summary\s+is\s+(.+?)(?:,\s*steps\s+is\s+|$)`).FindStringSubmatch(rest)
			stepsMatch := regexp.MustCompile(`(?i)steps\s+is\s+(\[[^\]]*\])`).FindStringSubmatch(rest)
			if len(summaryMatch) < 2 || len(stepsMatch) < 2 { continue }
			var steps []string
			if json.Unmarshal([]byte(stepsMatch[1]), &steps) != nil { continue }
			raw, _ := json.Marshal(map[string]any{"summary": strings.Trim(strings.TrimSpace(summaryMatch[1]), `"'`), "steps": steps})
			return name, string(raw), true
		case "image.generate":
			promptMatch := regexp.MustCompile(`(?i)prompt\s+is\s+(.+)`).FindStringSubmatch(rest)
			if len(promptMatch) < 2 { continue }
			raw, _ := json.Marshal(map[string]any{"prompt": strings.Trim(strings.TrimSpace(promptMatch[1]), `"'`)})
			return name, string(raw), true
		case "video.generate":
			promptMatch := regexp.MustCompile(`(?i)prompt\s+is\s+(.+)`).FindStringSubmatch(rest)
			if len(promptMatch) < 2 { continue }
			raw, _ := json.Marshal(map[string]any{"prompt": strings.Trim(strings.TrimSpace(promptMatch[1]), `"'`)})
			return name, string(raw), true
		case "canvas.arrange":
			nodesMatch := regexp.MustCompile(`nodes\s+is\s+(\[[^\]]*\])`).FindStringSubmatch(rest)
			if len(nodesMatch) < 2 { continue }
			var nodeIDs []string
			if err := json.Unmarshal([]byte(nodesMatch[1]), &nodeIDs); err != nil { continue }
			modeMatch := regexp.MustCompile(`direction\s+is\s+(horizontal|vertical|grid)`).FindStringSubmatch(rest)
			if len(modeMatch) < 2 { continue }
			gap := 40
			if gapMatch := regexp.MustCompile(`spacing\s+is\s+(\d+)`).FindStringSubmatch(rest); len(gapMatch) > 1 {
				if value, err := strconv.Atoi(gapMatch[1]); err == nil { gap = value }
			}
			raw, _ := json.Marshal(map[string]any{"nodeIds": nodeIDs, "mode": modeMatch[1], "gap": gap})
			return name, string(raw), true
		case "canvas.delete":
			nodesMatch := regexp.MustCompile(`nodes\s+is\s+(\[[^\]]*\])`).FindStringSubmatch(rest)
			if len(nodesMatch) < 2 { continue }
			var nodeIDs []string
			if err := json.Unmarshal([]byte(nodesMatch[1]), &nodeIDs); err != nil { continue }
			raw, _ := json.Marshal(map[string]any{"nodeIds": nodeIDs})
			return name, string(raw), true
		case "canvas.add_text":
			textMatch := regexp.MustCompile(`text\s+is\s+(.+)`).FindStringSubmatch(rest)
			if len(textMatch) < 2 { continue }
			text := strings.TrimSpace(textMatch[1])
			if quoted := quotedPattern.FindStringSubmatch(text); len(quoted) > 1 { text = quoted[1] }
			placement := "center"
			if placementMatch := regexp.MustCompile(`placement\s+is\s+(center|right_of_selection)`).FindStringSubmatch(rest); len(placementMatch) > 1 {
				placement = placementMatch[1]
			}
			raw, _ := json.Marshal(map[string]any{"text": text, "placement": placement})
			return name, string(raw), true
		case "canvas.update_text":
			nodeIDMatch := regexp.MustCompile(`nodeId\s+is\s+([^\s]+)`).FindStringSubmatch(rest)
			textMatch := regexp.MustCompile(`text\s+is\s+(.+)`).FindStringSubmatch(rest)
			if len(nodeIDMatch) < 2 || len(textMatch) < 2 { continue }
			nodeID := strings.Trim(nodeIDMatch[1], `"'`)
			text := strings.TrimSpace(textMatch[1])
			if quoted := quotedPattern.FindStringSubmatch(text); len(quoted) > 1 { text = quoted[1] }
			raw, _ := json.Marshal(map[string]any{"nodeId": nodeID, "text": text})
			return name, string(raw), true
		}
	}
	return "", "", false
}

func decodeAgentToolArguments(name, value string, authorization agentNodeAuthorization) (any, string, error) {
	var arguments any
	switch name {
	case "canvas.plan":
		var input canvasPlanArguments
		if err := decodeAgentJSONValue(value, &input); err != nil { return nil, "", errors.New("canvas.plan arguments invalid") }
		input.Summary = strings.TrimSpace(input.Summary)
		if input.Summary == "" || utf8.RuneCountInString(input.Summary) > 120 || len(input.Steps) < 2 || len(input.Steps) > 7 { return nil, "", errors.New("canvas.plan arguments invalid") }
		for i := range input.Steps {
			input.Steps[i] = strings.TrimSpace(input.Steps[i])
			if input.Steps[i] == "" || utf8.RuneCountInString(input.Steps[i]) > 80 { return nil, "", errors.New("canvas.plan steps invalid") }
		}
		arguments = input
	case "image.generate":
		var input struct {
			Prompt          string   `json:"prompt"`
			Count           *int     `json:"count"`
			ReferenceNodeIDs []string `json:"referenceNodeIds"`
		}
		if err := decodeAgentJSONValue(value, &input); err != nil { return nil, "", errors.New("image.generate arguments invalid") }
		input.Prompt = strings.TrimSpace(input.Prompt)
		if input.Prompt == "" || utf8.RuneCountInString(input.Prompt) > 8000 { return nil, "", errors.New("image.generate prompt invalid") }
		count := 1
		if input.Count != nil { count = *input.Count }
		if count < 1 || count > 4 { return nil, "", errors.New("image.generate count invalid") }
		seenReferences := make(map[string]struct{}, len(input.ReferenceNodeIDs))
		for i, id := range input.ReferenceNodeIDs {
			id = strings.TrimSpace(id)
			if id == "" { return nil, "", errors.New("image.generate referenceNodeIds invalid") }
			if _, exists := seenReferences[id]; exists { return nil, "", errors.New("image.generate referenceNodeIds invalid") }
			if _, valid := authorization.ImageNodeIDs[id]; !valid { return nil, "", errors.New("image.generate referenceNodeIds unauthorized") }
			seenReferences[id] = struct{}{}
			input.ReferenceNodeIDs[i] = id
		}
		arguments = imageGenerateArguments{Prompt: input.Prompt, Count: count, ReferenceNodeIDs: input.ReferenceNodeIDs}
	case "video.generate":
		var input struct { Prompt string `json:"prompt"`; Duration *int `json:"duration"`; ImageNodeID string `json:"imageNodeId"` }
		if err := decodeAgentJSONValue(value, &input); err != nil { return nil, "", errors.New("video.generate arguments invalid") }
		input.Prompt, input.ImageNodeID = strings.TrimSpace(input.Prompt), strings.TrimSpace(input.ImageNodeID)
		if input.Prompt == "" || utf8.RuneCountInString(input.Prompt) > 8000 { return nil, "", errors.New("video.generate prompt invalid") }
		duration := 6
		if input.Duration != nil { duration = *input.Duration }
		if duration < 1 || duration > 20 { return nil, "", errors.New("video.generate duration invalid") }
		if input.ImageNodeID != "" {
			if _, valid := authorization.ImageNodeIDs[input.ImageNodeID]; !valid { return nil, "", errors.New("video.generate imageNodeId unauthorized") }
		}
		arguments = videoGenerateArguments{Prompt: input.Prompt, Duration: duration, ImageNodeID: input.ImageNodeID}
	case "canvas.arrange":
		var input struct { NodeIDs []string `json:"nodeIds"`; Mode string `json:"mode"`; Gap *int `json:"gap"` }
		if err := decodeAgentJSONValue(value, &input); err != nil { return nil, "", errors.New("canvas.arrange arguments invalid") }
		if len(input.NodeIDs) < 2 || len(input.NodeIDs) > 20 { return nil, "", errors.New("canvas.arrange nodeIds invalid") }
		seen := make(map[string]struct{}, len(input.NodeIDs))
		for i, id := range input.NodeIDs {
			id = strings.TrimSpace(id)
			if id == "" { return nil, "", errors.New("canvas.arrange nodeIds invalid") }
			if _, exists := seen[id]; exists { return nil, "", errors.New("canvas.arrange nodeIds invalid") }
			if _, exists := authorization.NodeIDs[id]; !exists { return nil, "", errors.New("canvas.arrange nodeIds unauthorized") }
			seen[id], input.NodeIDs[i] = struct{}{}, id
		}
		input.Mode = strings.TrimSpace(input.Mode)
		if input.Mode != "horizontal" && input.Mode != "vertical" && input.Mode != "grid" { return nil, "", errors.New("canvas.arrange mode invalid") }
		gap := 40
		if input.Gap != nil { gap = *input.Gap }
		if gap < 16 || gap > 400 { return nil, "", errors.New("canvas.arrange gap invalid") }
		arguments = canvasArrangeArguments{NodeIDs: input.NodeIDs, Mode: input.Mode, Gap: gap}
	case "canvas.add_text":
		var input struct { Text string `json:"text"`; Placement string `json:"placement"`; SourceNodeIDs []string `json:"sourceNodeIds"` }
		if err := decodeAgentJSONValue(value, &input); err != nil { return nil, "", errors.New("canvas.add_text arguments invalid") }
		input.Text, input.Placement = strings.TrimSpace(input.Text), strings.TrimSpace(input.Placement)
		if input.Text == "" || utf8.RuneCountInString(input.Text) > 4000 { return nil, "", errors.New("canvas.add_text text invalid") }
		if input.Placement == "" { input.Placement = "center" }
		if input.Placement != "center" && input.Placement != "right_of_selection" { return nil, "", errors.New("canvas.add_text placement invalid") }
		if len(input.SourceNodeIDs) > 20 { return nil, "", errors.New("canvas.add_text sourceNodeIds invalid") }
		seen := make(map[string]struct{}, len(input.SourceNodeIDs))
		for i, id := range input.SourceNodeIDs {
			id = strings.TrimSpace(id)
			if id == "" { return nil, "", errors.New("canvas.add_text sourceNodeIds invalid") }
			if _, exists := seen[id]; exists { return nil, "", errors.New("canvas.add_text sourceNodeIds invalid") }
			if _, exists := authorization.NodeIDs[id]; !exists { return nil, "", errors.New("canvas.add_text sourceNodeIds unauthorized") }
			seen[id], input.SourceNodeIDs[i] = struct{}{}, id
		}
		arguments = canvasAddTextArguments{Text: input.Text, Placement: input.Placement, SourceNodeIDs: input.SourceNodeIDs}
	case "canvas.delete":
		var input canvasDeleteArguments
		if err := decodeAgentJSONValue(value, &input); err != nil || len(input.NodeIDs) < 1 || len(input.NodeIDs) > 20 { return nil, "", errors.New("canvas.delete arguments invalid") }
		seen := make(map[string]struct{}, len(input.NodeIDs))
		for i, id := range input.NodeIDs {
			id = strings.TrimSpace(id)
			if id == "" { return nil, "", errors.New("canvas.delete nodeIds invalid") }
			if _, exists := seen[id]; exists { return nil, "", errors.New("canvas.delete nodeIds invalid") }
			if _, exists := authorization.NodeIDs[id]; !exists { return nil, "", errors.New("canvas.delete nodeIds unauthorized") }
			seen[id], input.NodeIDs[i] = struct{}{}, id
		}
		arguments = input
	case "canvas.update_text":
		var input canvasUpdateTextArguments
		if err := decodeAgentJSONValue(value, &input); err != nil { return nil, "", errors.New("canvas.update_text arguments invalid") }
		input.NodeID, input.Text = strings.TrimSpace(input.NodeID), strings.TrimSpace(input.Text)
		if input.NodeID == "" || utf8.RuneCountInString(input.Text) > 4000 { return nil, "", errors.New("canvas.update_text arguments invalid") }
		if _, valid := authorization.TextNodeIDs[input.NodeID]; !valid { return nil, "", errors.New("canvas.update_text nodeId unauthorized") }
		arguments = input
	default:
		return nil, "", errors.New("agent upstream returned unsupported tool call")
	}
	raw, err := json.Marshal(arguments)
	if err != nil { return nil, "", err }
	return arguments, string(raw), nil
}

func decodeAgentJSONValue(value string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil { return err }
	if err := decoder.Decode(&struct{}{}); err != io.EOF { return errors.New("trailing JSON value") }
	return nil
}

func validateAgentToolSuccess(name string, arguments any, request *SubmitAgentToolResultRequest) error {
	switch name {
	case "canvas.plan":
		input := arguments.(canvasPlanArguments)
		if request.Plan == nil || request.Video != nil || len(request.Images) != 0 || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" {
			return safeMessageError{message: "执行计划工具结果无效"}
		}
		request.Plan.Summary = strings.TrimSpace(request.Plan.Summary)
		for i := range request.Plan.Steps { request.Plan.Steps[i] = strings.TrimSpace(request.Plan.Steps[i]) }
		if request.Plan.Summary != input.Summary || !sameStringSlice(request.Plan.Steps, input.Steps) { return safeMessageError{message: "执行计划工具结果无效"} }
	case "image.generate":
		input := arguments.(imageGenerateArguments)
		if request.Plan != nil || request.Video != nil || len(request.Images) < 1 || len(request.Images) > input.Count || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" {
			return safeMessageError{message: "图片生成工具结果无效"}
		}
		for i := range request.Images {
			request.Images[i].NodeID, request.Images[i].StorageKey = strings.TrimSpace(request.Images[i].NodeID), strings.TrimSpace(request.Images[i].StorageKey)
			if request.Images[i].NodeID == "" || request.Images[i].StorageKey == "" { return safeMessageError{message: "工具结果图片无效"} }
		}
	case "video.generate":
		if request.Plan != nil || request.Video == nil || len(request.Images) != 0 || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" {
			return safeMessageError{message: "视频生成工具结果无效"}
		}
		request.Video.NodeID, request.Video.StorageKey = strings.TrimSpace(request.Video.NodeID), strings.TrimSpace(request.Video.StorageKey)
		if request.Video.NodeID == "" || request.Video.StorageKey == "" { return safeMessageError{message: "工具结果视频无效"} }
	case "canvas.arrange":
		input := arguments.(canvasArrangeArguments)
		if request.Plan != nil || request.Video != nil || len(request.Images) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" || !sameTrimmedStringSet(request.NodeIDs, input.NodeIDs) || len(request.Positions) != len(input.NodeIDs) {
			return safeMessageError{message: "画布排列工具结果无效"}
		}
		expected, seen := stringSet(input.NodeIDs), make(map[string]struct{}, len(request.Positions))
		for i := range request.Positions {
			position := &request.Positions[i]
			position.NodeID = strings.TrimSpace(position.NodeID)
			if _, exists := expected[position.NodeID]; !exists || !finite(position.X) || !finite(position.Y) { return safeMessageError{message: "画布排列位置无效"} }
			if _, exists := seen[position.NodeID]; exists { return safeMessageError{message: "画布排列位置重复"} }
			seen[position.NodeID] = struct{}{}
		}
	case "canvas.add_text":
		input := arguments.(canvasAddTextArguments)
		request.NodeID, request.Placement = strings.TrimSpace(request.NodeID), strings.TrimSpace(request.Placement)
		if request.Plan != nil || request.Video != nil || len(request.Images) != 0 || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.NodeID == "" || request.Text != "" || request.Placement != input.Placement {
			return safeMessageError{message: "文本节点工具结果无效"}
		}
	case "canvas.delete":
		input := arguments.(canvasDeleteArguments)
		if request.Plan != nil || request.Video != nil || len(request.Images) != 0 || len(request.Positions) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" || !sameTrimmedStringSet(request.NodeIDs, input.NodeIDs) {
			return safeMessageError{message: "删除节点工具结果无效"}
		}
	case "canvas.update_text":
		input := arguments.(canvasUpdateTextArguments)
		request.NodeID, request.Text = strings.TrimSpace(request.NodeID), strings.TrimSpace(request.Text)
		if request.Plan != nil || request.Video != nil || len(request.Images) != 0 || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.Placement != "" || request.NodeID != input.NodeID || request.Text != input.Text {
			return safeMessageError{message: "更新文本工具结果无效"}
		}
	default:
		return safeMessageError{message: "工具名称无效"}
	}
	return nil
}

func agentToolResultOutput(name string, request SubmitAgentToolResultRequest) any {
	if request.Status == "failed" { return struct { Error string `json:"error"` }{request.Error} }
	switch name {
	case "canvas.plan":
		return struct { Plan *AgentToolPlan `json:"plan"` }{request.Plan}
	case "image.generate":
		return struct { Images []AgentToolImage `json:"images"` }{request.Images}
	case "video.generate":
		return struct { Video *AgentToolVideo `json:"video"` }{request.Video}
	case "canvas.arrange":
		return struct { NodeIDs []string `json:"nodeIds"`; Positions []AgentToolPosition `json:"positions"` }{request.NodeIDs, request.Positions}
	case "canvas.delete":
		return struct { NodeIDs []string `json:"nodeIds"` }{request.NodeIDs}
	case "canvas.update_text":
		return struct { NodeID string `json:"nodeId"`; Text string `json:"text"` }{request.NodeID, request.Text}
	default:
		return struct { NodeID string `json:"nodeId"`; Placement string `json:"placement"` }{request.NodeID, request.Placement}
	}
}

func agentToolSchemas() []any {
	return []any{
		map[string]any{"type": "function", "function": map[string]any{"name": "canvas.plan", "description": "当任务需要两个及以上工具时，先向用户展示本轮简短执行计划；单步任务不要调用", "parameters": map[string]any{"type": "object", "properties": map[string]any{"summary": map[string]any{"type": "string", "maxLength": 120}, "steps": map[string]any{"type": "array", "minItems": 2, "maxItems": 7, "items": map[string]any{"type": "string", "maxLength": 80}}}, "required": []string{"summary", "steps"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "image.generate", "description": "根据提示词生成一至四张图片，可通过 referenceNodeIds 引用画布中的图片节点作为参考", "parameters": map[string]any{"type": "object", "properties": map[string]any{"prompt": map[string]any{"type": "string", "maxLength": 8000}, "count": map[string]any{"type": "integer", "minimum": 1, "maximum": 4, "default": 1}, "referenceNodeIds": map[string]any{"type": "array", "maxItems": 6, "uniqueItems": true, "items": map[string]any{"type": "string"}}}, "required": []string{"prompt"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "video.generate", "description": "生成一个视频，可使用画布中相关图片作为参考", "parameters": map[string]any{"type": "object", "properties": map[string]any{"prompt": map[string]any{"type": "string", "maxLength": 8000}, "duration": map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "default": 6}, "imageNodeId": map[string]any{"type": "string"}}, "required": []string{"prompt"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "canvas.arrange", "description": "重排画布中相关节点", "parameters": map[string]any{"type": "object", "properties": map[string]any{"nodeIds": map[string]any{"type": "array", "minItems": 2, "maxItems": 20, "uniqueItems": true, "items": map[string]any{"type": "string"}}, "mode": map[string]any{"type": "string", "enum": []string{"horizontal", "vertical", "grid"}}, "gap": map[string]any{"type": "integer", "minimum": 16, "maximum": 400, "default": 40}}, "required": []string{"nodeIds", "mode"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "canvas.add_text", "description": "在画布中插入文本节点，可通过 sourceNodeIds 关联画布相关来源节点", "parameters": map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string", "maxLength": 4000}, "placement": map[string]any{"type": "string", "enum": []string{"center", "right_of_selection"}, "default": "center"}, "sourceNodeIds": map[string]any{"type": "array", "maxItems": 20, "uniqueItems": true, "items": map[string]any{"type": "string"}}}, "required": []string{"text"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "canvas.delete", "description": "删除画布中相关且当前仍选中的节点，执行前需要用户确认", "parameters": map[string]any{"type": "object", "properties": map[string]any{"nodeIds": map[string]any{"type": "array", "minItems": 1, "maxItems": 20, "uniqueItems": true, "items": map[string]any{"type": "string"}}}, "required": []string{"nodeIds"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "canvas.update_text", "description": "修改画布中相关且当前仍选中的文本节点，执行前需要用户确认", "parameters": map[string]any{"type": "object", "properties": map[string]any{"nodeId": map[string]any{"type": "string"}, "text": map[string]any{"type": "string", "maxLength": 4000}}, "required": []string{"nodeId", "text"}, "additionalProperties": false}}},
	}
}

func sameTrimmedStringSet(values, expected []string) bool {
	if len(values) != len(expected) { return false }
	expectedSet, seen := stringSet(expected), make(map[string]struct{}, len(values))
	for i, value := range values {
		value = strings.TrimSpace(value)
		if _, exists := expectedSet[value]; !exists { return false }
		if _, exists := seen[value]; exists { return false }
		seen[value], values[i] = struct{}{}, value
	}
	return true
}

func sameStringSlice(values, expected []string) bool {
	if len(values) != len(expected) { return false }
	for i := range values { if values[i] != expected[i] { return false } }
	return true
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values { result[value] = struct{}{} }
	return result
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
