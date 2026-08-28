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
	agentSystemPrompt            = "你是画布 Agent，不是只会回答问题的聊天机器人。根据画布 JSON、长期记忆和最后用户请求，先确定目标与可验证的完成条件，再自主拆解、执行工具、观察真实结果并继续规划，直到完成或明确阻塞。修改画布必须调用工具，禁止假装完成；多工具任务先调用 canvas.plan，每次只调用一个，计划的每个 steps 项必须按顺序对应后续一个真实工具及其成功条件，不要把纯思考过程写成步骤；计划中的步骤失败且仍能调整时，再次调用 canvas.plan 给出剩余步骤的新计划，服务端会保留旧计划与跳过原因。用户要求修改、替换或调整已有图片节点时优先使用 image.edit 而不是重新生成。参数：canvas.plan {summary,steps}；image.generate {prompt,count,referenceNodeIds}；image.edit {nodeId,prompt,count}；image.inspect {nodeIds,criteria}；video.generate {prompt,duration,imageNodeId}；video.inspect {nodeId,criteria}；canvas.add_text {text,placement,sourceNodeIds}；canvas.arrange {nodeIds,mode,gap}；canvas.delete {nodeIds}；canvas.update_text {nodeId,text}；agent.ask_user {question,options}；agent.remember {kind,key,content,scope,confidence,expiresInDays}；agent.forget {key,scope}。生成或编辑图片、生成视频后必须调用对应的 image.inspect 或 video.inspect 对照用户目标验收真实内容，再决定完成或调整；只有验收结果为 needs_revision 才可根据 revisedPrompt 重新生成，自主模式下每种媒体最多调整一次。只有用户明确表达以后长期遵循的偏好、项目事实或约束时才调用 agent.remember；不要把一次性请求或敏感信息写入记忆。删除、改文本、记住或遗忘都需用户确认。只有缺少可执行的主体或目标、要求互相冲突、或必须操作的目标无法唯一确定时才调用 agent.ask_user；能从画布、记忆或安全默认值推断的信息不要追问。每个真实 TOOL_RESULT 后先检查完成条件、剩余差距和可观察错误，再决定总结、调整或继续；不得重复同名同参数工具，也不得声称验证了工具结果中没有提供的信息。调用工具时只输出一行 `TOOL_CALL {\"name\":\"canvas.arrange\",\"arguments\":{...}}`；无需工具或任务完成时简短回答，并说明采用过的关键假设。"
	agentAutonomyCautious        = "cautious"
	agentAutonomyStandard        = "standard"
	agentAutonomyAutonomous      = "autonomous"
	agentWaitingToolTimeout      = 15 * time.Minute
	agentWaitingToolTimeoutError = "工具结果等待超时"
	agentToolExecutionLease      = 90 * time.Second
	agentRunningTimeout          = 3 * time.Minute
	agentRunningTimeoutError     = "助手运行恢复超时"
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
	Autonomy        string                  `json:"autonomy"`
	SelectedNodeIDs []string                `json:"selectedNodeIds"`
	FocusNodeIDs    []string                `json:"focusNodeIds,omitempty"`
	Nodes           []AgentCanvasNode       `json:"nodes"`
	Connections     []AgentCanvasConnection `json:"connections"`
}

type SubmitAgentMessageRequest struct {
	RunID          string             `json:"runId"`
	Content        string             `json:"content"`
	Model          string             `json:"model"`
	CanvasContext  AgentCanvasContext `json:"canvasContext"`
	CanvasSnapshot json.RawMessage    `json:"canvasSnapshot"`
	Budget         AgentRunBudget     `json:"budget"`
}

type AgentRunBudget struct {
	MaxToolCalls   int `json:"maxToolCalls"`
	MaxMediaCalls  int `json:"maxMediaCalls"`
	MaxDurationSec int `json:"maxDurationSec"`
	MaxCredits     int `json:"maxCredits"`
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

type AgentToolInspection struct {
	Status        string   `json:"status"`
	Summary       string   `json:"summary"`
	Issues        []string `json:"issues"`
	RevisedPrompt string   `json:"revisedPrompt,omitempty"`
}

type SubmitAgentToolResultRequest struct {
	CallID         string               `json:"callId"`
	ExecutionToken string               `json:"executionToken,omitempty"`
	Status         string               `json:"status"`
	Error          string               `json:"error,omitempty"`
	Images         []AgentToolImage     `json:"images,omitempty"`
	Video          *AgentToolVideo      `json:"video,omitempty"`
	NodeIDs        []string             `json:"nodeIds,omitempty"`
	Positions      []AgentToolPosition  `json:"positions,omitempty"`
	NodeID         string               `json:"nodeId,omitempty"`
	Text           string               `json:"text,omitempty"`
	Placement      string               `json:"placement,omitempty"`
	Plan           *AgentToolPlan       `json:"plan,omitempty"`
	Inspection     *AgentToolInspection `json:"inspection,omitempty"`
	Memory         *AgentToolMemory     `json:"memory,omitempty"`
}

type ConfirmAgentToolRequest struct {
	CallID   string `json:"callId"`
	Decision string `json:"decision"`
	Answer   string `json:"answer,omitempty"`
}

type RevertAgentToolRequest struct {
	CallID string `json:"callId"`
}

type AgentRunDiagnostic struct {
	ID           string                `json:"id"`
	Status       model.AgentRunStatus  `json:"status"`
	Model        string                `json:"model"`
	Error        string                `json:"error,omitempty"`
	StartedAt    string                `json:"startedAt"`
	CompletedAt  string                `json:"completedAt,omitempty"`
	DurationMS   int64                 `json:"durationMs"`
	CanRevert    bool                  `json:"canRevert"`
	Budget       AgentRunBudget        `json:"budget"`
	Usage        AgentRunUsage         `json:"usage"`
	BudgetReason string                `json:"budgetReason,omitempty"`
	Plan         []model.AgentPlanStep `json:"plan"`
	Steps        []AgentStepDiagnostic `json:"steps"`
}

type AgentRunUsage struct {
	ToolCalls          int `json:"toolCalls"`
	MediaCalls         int `json:"mediaCalls"`
	DurationSec        int `json:"durationSec"`
	StreamReconnects   int `json:"streamReconnects"`
	Credits            int `json:"credits"`
	ToolLeaseTakeovers int `json:"toolLeaseTakeovers"`
}

type AgentStepDiagnostic struct {
	CallID       string                `json:"callId,omitempty"`
	Type         model.AgentStepType   `json:"type"`
	ToolName     string                `json:"toolName,omitempty"`
	Status       model.AgentStepStatus `json:"status"`
	Confirmation string                `json:"confirmation,omitempty"`
	Error        string                `json:"error,omitempty"`
	StartedAt    string                `json:"startedAt"`
	CompletedAt  string                `json:"completedAt,omitempty"`
	DurationMS   int64                 `json:"durationMs"`
	Retryable    bool                  `json:"retryable"`
	Revertible   bool                  `json:"revertible"`
	Reverted     bool                  `json:"reverted"`
}

type AgentStepRetryResult struct {
	Run          model.AgentRun `json:"run"`
	CallID       string         `json:"callId"`
	SourceCallID string         `json:"sourceCallId"`
}

type AgentRunRevertResult struct {
	Run              model.AgentRun  `json:"run"`
	CallIDs          []string        `json:"callIds"`
	Snapshot         json.RawMessage `json:"snapshot"`
	SnapshotChecksum string          `json:"snapshotChecksum"`
}

type AgentToolResultReceipt struct {
	Status string                        `json:"status"`
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
	Prompt           string   `json:"prompt"`
	Count            int      `json:"count"`
	ReferenceNodeIDs []string `json:"referenceNodeIds,omitempty"`
}

type imageEditArguments struct {
	NodeID string `json:"nodeId"`
	Prompt string `json:"prompt"`
	Count  int    `json:"count"`
}

type videoGenerateArguments struct {
	Prompt      string `json:"prompt"`
	Duration    int    `json:"duration"`
	ImageNodeID string `json:"imageNodeId,omitempty"`
}

type videoInspectArguments struct {
	NodeID   string `json:"nodeId"`
	Criteria string `json:"criteria"`
}

type imageInspectArguments struct {
	NodeIDs  []string `json:"nodeIds"`
	Criteria string   `json:"criteria"`
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

type askUserArguments struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

type agentNodeAuthorization struct {
	NodeIDs      map[string]struct{}
	ImageNodeIDs map[string]struct{}
	VideoNodeIDs map[string]struct{}
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
	if err := RequireOrganizationWrite(user); err != nil {
		return model.AgentSession{}, err
	}
	request.SessionID, request.ProjectID, request.Title = strings.TrimSpace(request.SessionID), strings.TrimSpace(request.ProjectID), strings.TrimSpace(request.Title)
	if request.ProjectID == "" {
		return model.AgentSession{}, safeMessageError{message: "画布项目编号无效"}
	}
	if request.SessionID != "" && (!strings.HasPrefix(request.SessionID, "agent-session-") || len(request.SessionID) > 191) {
		return model.AgentSession{}, safeMessageError{message: "会话编号无效"}
	}
	exists, err := repository.UserProjectExists(user.OrganizationID, user.ID, request.ProjectID)
	if err != nil {
		return model.AgentSession{}, err
	}
	if !exists {
		return model.AgentSession{}, safeMessageError{message: "画布项目不存在"}
	}
	profile, err := json.Marshal(request.Profile)
	if err != nil {
		return model.AgentSession{}, safeMessageError{message: "会话配置无效"}
	}
	if len(profile) > 256<<10 {
		return model.AgentSession{}, safeMessageError{message: "会话配置过大"}
	}
	timestamp := now()
	sessionID := request.SessionID
	if sessionID == "" {
		sessionID = newID("agent-session")
	}
	session := model.AgentSession{ID: sessionID, OrganizationID: user.OrganizationID, UserID: user.ID, ProjectID: request.ProjectID, Profile: string(profile), Title: request.Title, Status: model.AgentSessionStatusActive, CreatedAt: timestamp, UpdatedAt: timestamp}
	session, err = repository.CreateAgentSession(session)
	if errors.Is(err, repository.ErrAgentToolResultConflict) {
		return model.AgentSession{}, safeMessageError{message: "会话编号与已提交请求不一致"}
	}
	return session, err
}

func SubmitAgentMessage(user model.AuthUser, sessionID string, request SubmitAgentMessageRequest) (AgentRunSubmission, error) {
	if err := RequireOrganizationWrite(user); err != nil {
		return AgentRunSubmission{}, err
	}
	sessionID, request.RunID, request.Content, request.Model = strings.TrimSpace(sessionID), strings.TrimSpace(request.RunID), strings.TrimSpace(request.Content), strings.TrimSpace(request.Model)
	if sessionID == "" {
		return AgentRunSubmission{}, safeMessageError{message: "会话编号无效"}
	}
	if request.RunID != "" && (!strings.HasPrefix(request.RunID, "agent-run-") || len(request.RunID) > 191) {
		return AgentRunSubmission{}, safeMessageError{message: "运行编号无效"}
	}
	if request.Content == "" || len(request.Content) > 256<<10 {
		return AgentRunSubmission{}, safeMessageError{message: "消息不能为空或过长"}
	}
	if request.Model == "" || len(request.Model) > 191 {
		return AgentRunSubmission{}, safeMessageError{message: "模型不能为空或过长"}
	}
	canvasContext, err := normalizeAgentCanvasContext(request.CanvasContext)
	if err != nil {
		return AgentRunSubmission{}, err
	}
	contextJSON, err := json.Marshal(canvasContext)
	if err != nil || len(contextJSON) > 256<<10 {
		return AgentRunSubmission{}, safeMessageError{message: "画布上下文过大"}
	}
	budget, err := normalizeAgentRunBudget(request.Budget)
	if err != nil {
		return AgentRunSubmission{}, err
	}
	snapshotPayload, snapshotChecksum, err := normalizeAgentRunSnapshot(request.CanvasSnapshot, canvasContext)
	if err != nil {
		return AgentRunSubmission{}, err
	}
	session, err := repository.GetAgentSession(user.OrganizationID, user.ID, sessionID)
	if err != nil {
		return AgentRunSubmission{}, err
	}
	timestamp, runID := now(), request.RunID
	if runID == "" {
		runID = newID("agent-run")
	}
	message := model.AgentMessage{ID: newID("agent-message"), OrganizationID: user.OrganizationID, UserID: user.ID, SessionID: sessionID, Role: model.AgentMessageRoleUser, Content: request.Content, CreatedAt: timestamp}
	run := model.AgentRun{ID: runID, OrganizationID: user.OrganizationID, UserID: user.ID, SessionID: sessionID, MessageID: message.ID, Model: request.Model, Context: string(contextJSON), MaxToolCalls: budget.MaxToolCalls, MaxMediaCalls: budget.MaxMediaCalls, MaxDurationSec: budget.MaxDurationSec, MaxCredits: budget.MaxCredits, Status: model.AgentRunStatusRunning, StartedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp}
	step := model.AgentStep{ID: newID("agent-step"), OrganizationID: user.OrganizationID, UserID: user.ID, RunID: runID, Type: model.AgentStepTypeCompletion, Status: model.AgentStepStatusRunning, Input: request.Content, StartedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp}
	event := model.AgentEvent{ID: newID("agent-event"), OrganizationID: user.OrganizationID, UserID: user.ID, RunID: runID, Type: model.AgentEventRunStarted, Payload: "{}", CreatedAt: timestamp}
	snapshot := model.AgentRunSnapshot{ID: newID("agent-run-snapshot"), OrganizationID: user.OrganizationID, UserID: user.ID, RunID: runID, ProjectID: session.ProjectID, Payload: string(snapshotPayload), Checksum: snapshotChecksum, CreatedAt: timestamp}
	message, run, err = repository.CreateAgentRun(message, run, step, event, snapshot)
	if errors.Is(err, repository.ErrAgentToolResultConflict) {
		return AgentRunSubmission{}, safeMessageError{message: "运行编号与已提交请求不一致"}
	}
	if err != nil {
		return AgentRunSubmission{}, err
	}
	agentRuns.Lock()
	_, alreadyRunning := agentRuns.cancels[run.ID]
	agentRuns.Unlock()
	if run.Status == model.AgentRunStatusRunning && !alreadyRunning {
		startAgentRun(run, user.Group)
	}
	return AgentRunSubmission{Message: message, Run: run}, nil
}

func SubmitAgentToolResult(user model.AuthUser, runID string, request SubmitAgentToolResultRequest) (model.AgentRun, error) {
	if err := RequireOrganizationWrite(user); err != nil {
		return model.AgentRun{}, err
	}
	runID, request.CallID, request.ExecutionToken, request.Status, request.Error = strings.TrimSpace(runID), strings.TrimSpace(request.CallID), strings.TrimSpace(request.ExecutionToken), strings.TrimSpace(request.Status), strings.TrimSpace(request.Error)
	request.Placement = strings.TrimSpace(request.Placement)
	if runID == "" || request.CallID == "" || request.ExecutionToken == "" || len(request.ExecutionToken) > 191 {
		return model.AgentRun{}, safeMessageError{message: "运行、工具调用或执行租约无效"}
	}
	if err := expireWaitingAgentRun(user.OrganizationID, user.ID, runID); err != nil {
		return model.AgentRun{}, err
	}
	if request.Status != "success" && request.Status != "failed" {
		return model.AgentRun{}, safeMessageError{message: "工具结果状态无效"}
	}
	if request.Status == "failed" {
		if request.Error == "" {
			return model.AgentRun{}, safeMessageError{message: "工具失败时必须返回错误"}
		}
		if len(request.Images) != 0 || request.Video != nil || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" || request.Plan != nil || request.Inspection != nil || request.Memory != nil {
			return model.AgentRun{}, safeMessageError{message: "工具失败结果无效"}
		}
	} else if request.Error != "" {
		return model.AgentRun{}, safeMessageError{message: "工具成功时不能返回错误"}
	}

	run, err := repository.GetAgentRun(user.OrganizationID, user.ID, runID)
	if err != nil {
		return model.AgentRun{}, err
	}
	toolStep, err := repository.GetAgentToolStep(user.OrganizationID, user.ID, runID, request.CallID)
	if err != nil {
		return model.AgentRun{}, err
	}
	authorization, err := agentRunNodeAuthorization(run)
	if err != nil {
		return model.AgentRun{}, err
	}
	toolArguments, _, err := decodeAgentToolArguments(toolStep.ToolName, toolStep.Input, authorization)
	if err != nil {
		return model.AgentRun{}, err
	}
	if request.Status == "success" {
		if err := validateAgentToolSuccess(toolStep.ToolName, toolArguments, &request); err != nil {
			return model.AgentRun{}, err
		}
		if toolStep.ToolName == "agent.remember" || toolStep.ToolName == "agent.forget" {
			if err := persistAgentMemoryToolResult(run, toolStep.ToolName, toolArguments, request.Memory); err != nil {
				return model.AgentRun{}, err
			}
		}
	}
	executionToken := request.ExecutionToken
	request.ExecutionToken = ""
	output, err := json.Marshal(request)
	if err != nil {
		return model.AgentRun{}, err
	}
	if len(output) > 64<<10 {
		return model.AgentRun{}, safeMessageError{message: "工具结果过大"}
	}
	timestamp := now()
	completedPayload, _ := json.Marshal(map[string]any{"callId": request.CallID, "name": toolStep.ToolName, "status": request.Status, "output": agentToolResultOutput(toolStep.ToolName, request)})
	failedPayload, _ := json.Marshal(map[string]string{"error": request.Error})
	completionStep := model.AgentStep{ID: newID("agent-step"), OrganizationID: user.OrganizationID, UserID: user.ID, RunID: runID, Type: model.AgentStepTypeCompletion, Status: model.AgentStepStatusRunning, Input: string(output), StartedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp}
	continueAfterFailure := false
	if request.Status == "failed" && agentRunAutonomy(run) == agentAutonomyAutonomous && retryableAgentTool(toolStep.ToolName) {
		steps, listErr := repository.ListCompletedAgentToolSteps(run.OrganizationID, run.UserID, run.ID)
		if listErr != nil {
			return model.AgentRun{}, listErr
		}
		continueAfterFailure = failedAgentToolCallCount(steps) == 0
	}
	run, _, resume, err := repository.SubmitAgentToolResult(
		user.OrganizationID, user.ID, runID, request.CallID, executionToken, string(output), request.Error, timestamp, request.Status == "success",
		continueAfterFailure,
		model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolCompleted, Payload: string(completedPayload), CreatedAt: timestamp},
		model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventRunFailed, Payload: string(failedPayload), CreatedAt: timestamp}, completionStep,
	)
	if errors.Is(err, repository.ErrAgentToolResultConflict) {
		return model.AgentRun{}, safeMessageError{message: "工具结果与已保存回执不一致"}
	}
	if errors.Is(err, repository.ErrAgentToolExecutionClaimed) {
		return model.AgentRun{}, safeMessageError{message: "工具执行租约已失效"}
	}
	if err != nil {
		return model.AgentRun{}, err
	}
	if toolStep.ToolName == "canvas.plan" && request.Plan != nil {
		if _, err := repository.ReplaceAgentPlan(run.OrganizationID, run.UserID, run.ID, request.CallID, request.Plan.Steps, timestamp); err != nil {
			return model.AgentRun{}, err
		}
	}
	if toolStep.ToolName != "canvas.plan" {
		reason := request.Error
		if request.Status == "success" {
			reason = ""
		}
		if err := repository.FinishAgentPlanStep(user.OrganizationID, user.ID, runID, request.CallID, request.Status == "success", reason, timestamp); err != nil {
			return model.AgentRun{}, err
		}
	}
	if resume {
		startAgentRun(run, user.Group)
	}
	return run, nil
}

func ClaimAgentToolExecution(user model.AuthUser, runID string, request ClaimAgentToolRequest) error {
	if err := RequireOrganizationWrite(user); err != nil {
		return err
	}
	runID, request.CallID, request.Token = strings.TrimSpace(runID), strings.TrimSpace(request.CallID), strings.TrimSpace(request.Token)
	if runID == "" || request.CallID == "" || request.Token == "" || len(request.Token) > 191 {
		return safeMessageError{message: "工具执行认领参数无效"}
	}
	if err := expireWaitingAgentRun(user.OrganizationID, user.ID, runID); err != nil {
		return err
	}
	run, err := repository.GetAgentRun(user.OrganizationID, user.ID, runID)
	if err != nil {
		return err
	}
	if run.Status != model.AgentRunStatusWaitingTool {
		return safeMessageError{message: "当前运行不再等待工具结果"}
	}
	if _, err := repository.GetAgentToolStep(user.OrganizationID, user.ID, runID, request.CallID); err != nil {
		return err
	}
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
	if runID == "" || callID == "" {
		return AgentToolResultReceipt{}, safeMessageError{message: "运行或工具调用编号无效"}
	}
	if err := expireWaitingAgentRun(user.OrganizationID, user.ID, runID); err != nil {
		return AgentToolResultReceipt{}, err
	}
	run, err := repository.GetAgentRun(user.OrganizationID, user.ID, runID)
	if err != nil {
		return AgentToolResultReceipt{}, err
	}
	step, err := repository.GetAgentToolStep(user.OrganizationID, user.ID, runID, callID)
	if err != nil {
		return AgentToolResultReceipt{}, err
	}
	if step.Confirmation == "rejected" {
		return AgentToolResultReceipt{Status: "rejected"}, nil
	}
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
	if err := RequireOrganizationWrite(user); err != nil {
		return model.AgentRun{}, err
	}
	runID, request.CallID, request.Decision, request.Answer = strings.TrimSpace(runID), strings.TrimSpace(request.CallID), strings.TrimSpace(request.Decision), strings.TrimSpace(request.Answer)
	if runID == "" || request.CallID == "" {
		return model.AgentRun{}, safeMessageError{message: "运行或工具调用编号无效"}
	}
	if request.Decision != "approved" && request.Decision != "rejected" {
		return model.AgentRun{}, safeMessageError{message: "工具确认决定无效"}
	}

	run, err := repository.GetAgentRun(user.OrganizationID, user.ID, runID)
	if err != nil {
		return model.AgentRun{}, err
	}
	toolStep, err := repository.GetAgentToolStep(user.OrganizationID, user.ID, runID, request.CallID)
	if err != nil {
		return model.AgentRun{}, err
	}
	if toolStep.ToolName == "agent.ask_user" {
		if request.Decision == "approved" && request.Answer == "" {
			return model.AgentRun{}, safeMessageError{message: "回答不能为空"}
		}
		if utf8.RuneCountInString(request.Answer) > 2000 {
			return model.AgentRun{}, safeMessageError{message: "回答过长"}
		}
		answer := request.Answer
		if answer == "" {
			answer = "用户跳过了回答"
		}
		output, err := json.Marshal(map[string]string{"status": "success", "answer": answer})
		if err != nil {
			return model.AgentRun{}, err
		}
		timestamp := now()
		completedPayload, _ := json.Marshal(map[string]any{"callId": request.CallID, "name": toolStep.ToolName, "status": request.Decision, "output": map[string]string{"answer": answer}})
		completionStep := model.AgentStep{ID: newID("agent-step"), OrganizationID: user.OrganizationID, UserID: user.ID, RunID: runID, Type: model.AgentStepTypeCompletion, Status: model.AgentStepStatusRunning, Input: string(output), StartedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp}
		run, resume, err := repository.AnswerAgentAskUser(
			user.OrganizationID, user.ID, runID, request.CallID, request.Decision, string(output), timestamp,
			model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolCompleted, Payload: string(completedPayload), CreatedAt: timestamp}, completionStep,
		)
		if errors.Is(err, repository.ErrAgentToolResultConflict) {
			return model.AgentRun{}, safeMessageError{message: "回答与已保存结果不一致"}
		}
		if err != nil {
			return model.AgentRun{}, err
		}
		_ = repository.FinishAgentPlanStep(user.OrganizationID, user.ID, runID, request.CallID, request.Decision == "approved", answer, timestamp)
		if resume {
			startAgentRun(run, user.Group)
		}
		return run, nil
	}
	if !agentToolRequiresConfirmation(toolStep.ToolName) {
		return model.AgentRun{}, safeMessageError{message: "该工具无需确认"}
	}
	authorization, err := agentRunNodeAuthorization(run)
	if err != nil {
		return model.AgentRun{}, err
	}
	toolArguments, _, err := decodeAgentToolArguments(toolStep.ToolName, toolStep.Input, authorization)
	if err != nil {
		return model.AgentRun{}, err
	}
	rejectedObservation, err := json.Marshal(map[string]string{"status": "rejected", "error": "用户拒绝执行该工具调用"})
	if err != nil {
		return model.AgentRun{}, err
	}
	timestamp := now()
	approvedPayload, _ := json.Marshal(map[string]any{"callId": request.CallID, "name": toolStep.ToolName, "arguments": toolArguments})
	rejectedPayload, _ := json.Marshal(map[string]any{"callId": request.CallID, "name": toolStep.ToolName, "status": "rejected", "output": map[string]string{"error": "用户拒绝执行该工具调用"}})
	completionStep := model.AgentStep{ID: newID("agent-step"), OrganizationID: user.OrganizationID, UserID: user.ID, RunID: runID, Type: model.AgentStepTypeCompletion, Status: model.AgentStepStatusRunning, Input: string(rejectedObservation), StartedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp}
	run, _, approved, resume, err := repository.ConfirmAgentTool(
		user.OrganizationID, user.ID, runID, request.CallID, request.Decision, string(rejectedObservation), timestamp,
		model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolCall, Payload: string(approvedPayload), CreatedAt: timestamp},
		model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolCompleted, Payload: string(rejectedPayload), CreatedAt: timestamp}, completionStep,
	)
	if err != nil {
		return model.AgentRun{}, err
	}
	if request.Decision == "rejected" {
		_ = repository.FinishAgentPlanStep(user.OrganizationID, user.ID, runID, request.CallID, false, "用户拒绝执行", timestamp)
	}
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
	if err := expireWaitingAgentRun(user.OrganizationID, user.ID, runID); err != nil {
		return model.AgentRun{}, err
	}
	return repository.GetAgentRun(user.OrganizationID, user.ID, runID)
}

func PollAgentRunEvents(user model.AuthUser, runID string, after int64) ([]model.AgentEvent, model.AgentRun, error) {
	runID = strings.TrimSpace(runID)
	events, err := repository.ListAgentEvents(user.OrganizationID, user.ID, runID, after, 100)
	if err != nil {
		return nil, model.AgentRun{}, err
	}
	run, err := repository.GetAgentRun(user.OrganizationID, user.ID, runID)
	return events, run, err
}

func GetAgentRunDiagnostics(user model.AuthUser, runID string) (AgentRunDiagnostic, error) {
	run, err := GetAgentRun(user, runID)
	if err != nil {
		return AgentRunDiagnostic{}, err
	}
	steps, err := repository.ListAgentSteps(user.OrganizationID, user.ID, run.ID)
	if err != nil {
		return AgentRunDiagnostic{}, err
	}
	events, err := repository.ListAgentEvents(user.OrganizationID, user.ID, run.ID, 0, 100)
	if err != nil {
		return AgentRunDiagnostic{}, err
	}
	reverted := revertedAgentToolCallIDs(events)
	plan, err := repository.ListAgentPlanSteps(user.OrganizationID, user.ID, run.ID)
	if err != nil {
		return AgentRunDiagnostic{}, err
	}
	if plan == nil {
		plan = []model.AgentPlanStep{}
	}
	result := AgentRunDiagnostic{
		ID: run.ID, Status: run.Status, Model: run.Model, Error: run.Error,
		StartedAt: run.StartedAt, CompletedAt: run.CompletedAt,
		DurationMS: agentDurationMS(run.StartedAt, run.CompletedAt),
		Budget:     effectiveAgentRunBudget(run),
		Usage:      agentRunUsage(run, steps), BudgetReason: run.BudgetReason, Plan: plan,
		Steps: make([]AgentStepDiagnostic, 0, len(steps)),
	}
	for _, step := range steps {
		_, wasReverted := reverted[step.ToolCallID]
		revertible := step.Type == model.AgentStepTypeTool && agentStepSucceeded(step) && revertibleAgentTool(step.ToolName) && !wasReverted
		diagnostic := AgentStepDiagnostic{
			CallID: step.ToolCallID, Type: step.Type, ToolName: step.ToolName, Status: step.Status,
			Confirmation: step.Confirmation, Error: step.Error, StartedAt: step.StartedAt,
			CompletedAt: step.CompletedAt, DurationMS: agentDurationMS(step.StartedAt, step.CompletedAt),
			Retryable:  run.Status == model.AgentRunStatusFailed && step.Status == model.AgentStepStatusFailed && step.Confirmation == "" && retryableAgentTool(step.ToolName),
			Revertible: revertible, Reverted: wasReverted,
		}
		result.CanRevert = result.CanRevert || revertible
		result.Steps = append(result.Steps, diagnostic)
	}
	return result, nil
}

func RetryAgentStep(user model.AuthUser, runID, sourceCallID string) (AgentStepRetryResult, error) {
	if err := RequireOrganizationWrite(user); err != nil {
		return AgentStepRetryResult{}, err
	}
	runID, sourceCallID = strings.TrimSpace(runID), strings.TrimSpace(sourceCallID)
	if runID == "" || sourceCallID == "" {
		return AgentStepRetryResult{}, safeMessageError{message: "运行或工具调用编号无效"}
	}
	run, err := repository.GetAgentRun(user.OrganizationID, user.ID, runID)
	if err != nil {
		return AgentStepRetryResult{}, err
	}
	step, err := repository.GetAgentToolStep(user.OrganizationID, user.ID, runID, sourceCallID)
	if err != nil {
		return AgentStepRetryResult{}, err
	}
	if run.Status != model.AgentRunStatusFailed || step.Status != model.AgentStepStatusFailed || step.Confirmation != "" || !retryableAgentTool(step.ToolName) {
		return AgentStepRetryResult{}, safeMessageError{message: "该失败步骤当前不能重试"}
	}
	authorization, err := agentRunNodeAuthorization(run)
	if err != nil {
		return AgentStepRetryResult{}, err
	}
	arguments, raw, err := decodeAgentToolArguments(step.ToolName, step.Input, authorization)
	if err != nil {
		return AgentStepRetryResult{}, err
	}
	callID, timestamp := newID("agent-tool-call"), now()
	payload, _ := json.Marshal(map[string]any{"callId": callID, "name": step.ToolName, "arguments": arguments, "meta": map[string]any{"retryOf": sourceCallID}})
	retryStep := model.AgentStep{ID: newID("agent-step"), OrganizationID: user.OrganizationID, UserID: user.ID, RunID: runID, Type: model.AgentStepTypeTool, Status: model.AgentStepStatusRunning, ToolCallID: callID, ToolName: step.ToolName, Input: raw, StartedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp}
	run, err = repository.RetryAgentStep(user.OrganizationID, user.ID, runID, sourceCallID, timestamp, retryStep, model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolCall, Payload: string(payload), CreatedAt: timestamp})
	if errors.Is(err, repository.ErrAgentStepNotRetryable) {
		return AgentStepRetryResult{}, safeMessageError{message: "该失败步骤已被处理或当前不能重试"}
	}
	if err != nil {
		return AgentStepRetryResult{}, err
	}
	time.AfterFunc(agentWaitingToolTimeout, func() { _ = expireWaitingAgentRun(user.OrganizationID, user.ID, runID) })
	return AgentStepRetryResult{Run: run, CallID: callID, SourceCallID: sourceCallID}, nil
}

func CancelAgentRun(user model.AuthUser, runID string) (model.AgentRun, error) {
	runID = strings.TrimSpace(runID)
	run, cancelled, err := repository.CancelAgentRun(user.OrganizationID, user.ID, runID, newID("agent-event"), now())
	if err != nil {
		return model.AgentRun{}, err
	}
	if cancelled {
		_ = repository.FinalizeAgentPlan(user.OrganizationID, user.ID, runID, "运行已取消", now())
		agentRuns.Lock()
		execution := agentRuns.cancels[runID]
		agentRuns.Unlock()
		if execution.cancel != nil {
			execution.cancel()
		}
	}
	return run, nil
}

func RevertAgentTool(user model.AuthUser, runID string, request RevertAgentToolRequest) (model.AgentRun, error) {
	if err := RequireOrganizationWrite(user); err != nil {
		return model.AgentRun{}, err
	}
	runID, request.CallID = strings.TrimSpace(runID), strings.TrimSpace(request.CallID)
	if runID == "" || request.CallID == "" {
		return model.AgentRun{}, safeMessageError{message: "运行或工具调用编号无效"}
	}
	step, err := repository.GetAgentToolStep(user.OrganizationID, user.ID, runID, request.CallID)
	if err != nil {
		return model.AgentRun{}, err
	}
	if !revertibleAgentTool(step.ToolName) {
		return model.AgentRun{}, safeMessageError{message: "该工具没有可撤销的画布操作"}
	}
	timestamp := now()
	revertedPayload, _ := json.Marshal(map[string]string{"callId": request.CallID, "name": step.ToolName})
	run, err := repository.RevertAgentTool(
		user.OrganizationID, user.ID, runID, request.CallID, timestamp,
		model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolReverted, Payload: string(revertedPayload), CreatedAt: timestamp},
		model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventRunCancelled, Payload: `{"reason":"tool_reverted"}`, CreatedAt: timestamp},
	)
	if errors.Is(err, repository.ErrAgentToolNotRevertible) {
		return model.AgentRun{}, safeMessageError{message: "该工具调用当前不能撤销"}
	}
	if err != nil {
		return model.AgentRun{}, err
	}
	agentRuns.Lock()
	execution := agentRuns.cancels[runID]
	agentRuns.Unlock()
	if execution.cancel != nil {
		execution.cancel()
	}
	return run, nil
}

func RevertAgentRun(user model.AuthUser, runID string) (AgentRunRevertResult, error) {
	if err := RequireOrganizationWrite(user); err != nil {
		return AgentRunRevertResult{}, err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return AgentRunRevertResult{}, safeMessageError{message: "运行编号无效"}
	}
	if _, err := repository.GetAgentRun(user.OrganizationID, user.ID, runID); err != nil {
		return AgentRunRevertResult{}, err
	}
	snapshot, err := repository.GetAgentRunSnapshot(user.OrganizationID, user.ID, runID)
	if err != nil {
		return AgentRunRevertResult{}, safeMessageError{message: "本轮没有可恢复的服务端画布快照"}
	}
	steps, err := repository.ListAgentSteps(user.OrganizationID, user.ID, runID)
	if err != nil {
		return AgentRunRevertResult{}, err
	}
	callIDs := make([]string, 0)
	events := make([]model.AgentEvent, 0)
	timestamp := now()
	for _, step := range steps {
		if !agentStepSucceeded(step) || !revertibleAgentTool(step.ToolName) {
			continue
		}
		callIDs = append(callIDs, step.ToolCallID)
		payload, _ := json.Marshal(map[string]string{"callId": step.ToolCallID, "name": step.ToolName})
		events = append(events, model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolReverted, Payload: string(payload), CreatedAt: timestamp})
	}
	if len(callIDs) == 0 {
		return AgentRunRevertResult{}, safeMessageError{message: "本轮没有可撤销的画布操作"}
	}
	run, err := repository.RevertAgentRun(user.OrganizationID, user.ID, runID, callIDs, timestamp, events, model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventRunCancelled, Payload: `{"reason":"tool_reverted"}`, CreatedAt: timestamp})
	if errors.Is(err, repository.ErrAgentToolNotRevertible) {
		return AgentRunRevertResult{}, safeMessageError{message: "本轮画布操作当前不能撤销"}
	}
	if err != nil {
		return AgentRunRevertResult{}, err
	}
	_ = repository.MarkAgentRunSnapshotRestored(user.OrganizationID, user.ID, runID, snapshot.Checksum, timestamp)
	agentRuns.Lock()
	execution := agentRuns.cancels[runID]
	agentRuns.Unlock()
	if execution.cancel != nil {
		execution.cancel()
	}
	return AgentRunRevertResult{Run: run, CallIDs: callIDs, Snapshot: json.RawMessage(snapshot.Payload), SnapshotChecksum: snapshot.Checksum}, nil
}

func startAgentRun(run model.AgentRun, userGroup string) {
	toolSteps, err := repository.ListCompletedAgentToolSteps(run.OrganizationID, run.UserID, run.ID)
	if err != nil {
		return
	}
	requestID := fmt.Sprintf("%s-completion-%d", run.ID, len(toolSteps)+1)
	timestamp := time.Now().UTC()
	if err := repository.ClaimAgentRunExecution(run.OrganizationID, run.UserID, run.ID, newID("agent-execution"), timestamp.Format(timestampLayout), timestamp.Add(-agentRunningTimeout).Format(timestampLayout)); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	agentRuns.Lock()
	agentRuns.cancels[run.ID] = agentRunExecution{requestID: requestID, cancel: cancel}
	agentRuns.Unlock()
	saved, err := repository.GetAgentRun(run.OrganizationID, run.UserID, run.ID)
	if err != nil || saved.Status != model.AgentRunStatusRunning {
		cancel()
		agentRuns.Lock()
		if execution := agentRuns.cancels[run.ID]; execution.requestID == requestID {
			delete(agentRuns.cancels, run.ID)
		}
		agentRuns.Unlock()
		return
	}
	go executeAgentRun(ctx, cancel, run, userGroup, requestID)
}

func executeAgentRun(ctx context.Context, cancel context.CancelFunc, run model.AgentRun, userGroup, requestID string) {
	defer func() {
		cancel()
		agentRuns.Lock()
		if execution := agentRuns.cancels[run.ID]; execution.requestID == requestID {
			delete(agentRuns.cancels, run.ID)
		}
		agentRuns.Unlock()
	}()

	completion, err := requestAgentCompletion(ctx, run, userGroup, requestID)
	if err != nil {
		failAgentRunUnlessCancelled(run, err)
		return
	}
	if completion.ToolCall != nil {
		if reason := agentRunBudgetReason(run, mustListAgentToolSteps(run), completion.ToolCall.Name); reason != "" {
			_ = repository.MarkAgentRunBudgetReason(run.OrganizationID, run.UserID, run.ID, reason, now())
			completion.ToolCall = nil
			completion.Content = agentBudgetMessage(reason)
		}
	}
	if completion.ToolCall != nil {
		payload, _ := json.Marshal(map[string]any{"callId": completion.ToolCall.ID, "name": completion.ToolCall.Name, "arguments": completion.ToolCall.Arguments})
		timestamp := now()
		step := model.AgentStep{ID: newID("agent-step"), OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeTool, Status: model.AgentStepStatusRunning, ToolCallID: completion.ToolCall.ID, ToolName: completion.ToolCall.Name, Input: completion.ToolCall.Raw, StartedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp}
		confirmationRequired := agentToolRequiresConfirmation(completion.ToolCall.Name)
		if confirmationRequired {
			event := model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolConfirmationRequired, Payload: string(payload), CreatedAt: timestamp}
			if err := repository.WaitAgentRunForConfirmation(run.OrganizationID, run.UserID, run.ID, completion.Content, step, event, timestamp); err != nil {
				failAgentRunUnlessCancelled(run, err)
			} else if completion.ToolCall.Name != "canvas.plan" {
				_ = repository.StartNextAgentPlanStep(run.OrganizationID, run.UserID, run.ID, completion.ToolCall.ID, completion.ToolCall.Name, timestamp)
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
				if completion.ToolCall.Name != "canvas.plan" {
					_ = repository.StartNextAgentPlanStep(run.OrganizationID, run.UserID, run.ID, completion.ToolCall.ID, completion.ToolCall.Name, timestamp)
				}
				time.AfterFunc(agentWaitingToolTimeout, func() { _ = expireWaitingAgentRun(run.OrganizationID, run.UserID, run.ID) })
			}
		}
		return
	}
	payload, _ := json.Marshal(map[string]string{"content": completion.Content})
	_ = repository.FinalizeAgentPlan(run.OrganizationID, run.UserID, run.ID, "运行已结束", now())
	if err := repository.CompleteAgentRun(run.OrganizationID, run.UserID, run.ID, newID("agent-message"), newID("agent-event"), newID("agent-event"), completion.Content, string(payload), now()); err != nil {
		failedPayload, _ := json.Marshal(map[string]string{"error": "助手结果保存失败"})
		_ = repository.FailAgentRun(run.OrganizationID, run.UserID, run.ID, newID("agent-event"), "助手结果保存失败", string(failedPayload), now())
	}
}

func failAgentRunUnlessCancelled(run model.AgentRun, err error) {
	if saved, getErr := repository.GetAgentRun(run.OrganizationID, run.UserID, run.ID); getErr == nil && saved.Status == model.AgentRunStatusCancelled {
		return
	}
	message := "助手请求失败，请重试"
	if errors.Is(err, context.DeadlineExceeded) {
		message = "助手请求超时，请重试"
	} else if strings.Contains(strings.ToLower(err.Error()), "status 429") || strings.Contains(strings.ToLower(err.Error()), "status 5") {
		message = "助手服务暂时不可用，请重试"
	} else if strings.Contains(strings.ToLower(err.Error()), "connection reset") || strings.Contains(strings.ToLower(err.Error()), "unexpected eof") {
		message = "助手服务连接中断，请重试"
	}
	payload, _ := json.Marshal(map[string]string{"error": message})
	timestamp := now()
	_ = repository.FailAgentRun(run.OrganizationID, run.UserID, run.ID, newID("agent-event"), message, string(payload), timestamp)
	_ = repository.FinalizeAgentPlan(run.OrganizationID, run.UserID, run.ID, message, timestamp)
}

func expireWaitingAgentRun(organizationID, userID, runID string) error {
	timestamp := time.Now().UTC()
	waitingPayload, _ := json.Marshal(map[string]string{"error": agentWaitingToolTimeoutError})
	if _, _, err := repository.ExpireWaitingAgentRun(organizationID, userID, runID, newID("agent-event"), agentWaitingToolTimeoutError, string(waitingPayload), timestamp.Format(timestampLayout), timestamp.Add(-agentWaitingToolTimeout).Format(timestampLayout)); err != nil {
		return err
	}
	agentRuns.Lock()
	_, active := agentRuns.cancels[runID]
	agentRuns.Unlock()
	if active {
		return nil
	}
	runningPayload, _ := json.Marshal(map[string]string{"error": agentRunningTimeoutError})
	_, _, err := repository.ExpireRunningAgentRun(organizationID, userID, runID, newID("agent-event"), agentRunningTimeoutError, string(runningPayload), timestamp.Format(timestampLayout), timestamp.Add(-agentRunningTimeout).Format(timestampLayout))
	return err
}

func requestAgentCompletion(ctx context.Context, run model.AgentRun, userGroup, requestID string) (completion agentCompletion, err error) {
	messages, err := repository.ListRecentAgentMessages(run.OrganizationID, run.UserID, run.SessionID, 30)
	if err != nil {
		return completion, err
	}
	maxToolCalls := effectiveAgentRunBudget(run).MaxToolCalls
	toolSteps, err := repository.ListCompletedAgentToolSteps(run.OrganizationID, run.UserID, run.ID)
	if err != nil {
		return completion, err
	}
	if reason := agentRunBudgetReason(run, toolSteps, ""); reason != "" {
		_ = repository.MarkAgentRunBudgetReason(run.OrganizationID, run.UserID, run.ID, reason, now())
		return agentCompletion{Content: agentBudgetMessage(reason)}, nil
	}
	session, err := repository.GetAgentSession(run.OrganizationID, run.UserID, run.SessionID)
	if err != nil {
		return completion, err
	}
	memories, err := repository.ListActiveAgentMemories(run.OrganizationID, run.UserID, session.ProjectID, 20)
	if err != nil {
		return completion, err
	}
	if routed, handled, routeErr := routePendingAgentImageInspection(run, messages, toolSteps); handled || routeErr != nil {
		return routed, routeErr
	}
	if routed, handled, routeErr := routePendingAgentVideoInspection(run, messages, toolSteps); handled || routeErr != nil {
		return routed, routeErr
	}
	if routed, handled, routeErr := routeDeterministicAgentCompletion(run, messages, toolSteps); handled || routeErr != nil {
		return routed, routeErr
	}
	pricingRequest := PricingRequest{Model: run.Model, Modality: "text", Operation: "completion", Unit: "request", Quantity: 1}
	selection, err := SelectModelChannel(pricingRequest)
	if err != nil {
		return completion, err
	}
	credits, err := CalculateRequestCreditsForGroup(pricingRequest, userGroup)
	if err != nil {
		return completion, err
	}
	task, err := BeginGenerationTask(GenerationTaskInput{
		UserID: run.UserID, OrganizationID: run.OrganizationID, RequestID: requestID,
		Model: run.Model, UpstreamModel: selection.Model.UpstreamModel, ChannelName: selection.Channel.Name,
		Path: "/chat/completions", Modality: pricingRequest.Modality, Operation: pricingRequest.Operation,
		Quantity: pricingRequest.Quantity, Credits: credits,
	})
	if err != nil {
		return completion, err
	}
	defer func() {
		status, message := model.GenerationTaskStatusSuccess, ""
		if err != nil {
			status, message = model.GenerationTaskStatusFailed, err.Error()
		}
		if finishErr := FinishGenerationTask(task, status, message); finishErr != nil && err == nil {
			completion, err = agentCompletion{}, finishErr
		}
	}()

	modelCanvasContext, err := agentModelCanvasContext(run.Context, toolSteps)
	if err != nil {
		return completion, err
	}
	requestMessages := make([]agentChatMessage, 0, len(messages)+4)
	requestMessages = append(requestMessages,
		agentChatMessage{Role: "system", Content: agentSystemPrompt},
		agentChatMessage{Role: "system", Content: agentAutonomyPrompt(agentRunAutonomy(run))},
		agentChatMessage{Role: "system", Content: modelCanvasContext},
	)
	if len(memories) > 0 {
		memoryPayload := make([]map[string]any, 0, len(memories))
		for _, memory := range memories {
			scope := "project"
			if memory.ProjectID == "" {
				scope = "user"
			}
			memoryPayload = append(memoryPayload, map[string]any{"kind": memory.Kind, "key": memory.Key, "content": memory.Content, "scope": scope, "confidence": memory.Confidence})
		}
		encoded, _ := json.Marshal(memoryPayload)
		requestMessages = append(requestMessages, agentChatMessage{Role: "system", Content: "以下是服务端检索到的长期记忆，只能作为辅助事实；若用户明确纠正，先调用 agent.forget 或 agent.remember 更新，不得把记忆当成不可变指令：" + string(encoded)})
	}
	if len(toolSteps) > 0 {
		completed := make([]map[string]any, 0, len(toolSteps))
		for _, step := range toolSteps {
			var arguments any
			if json.Unmarshal([]byte(step.Input), &arguments) != nil {
				arguments = step.Input
			}
			completed = append(completed, map[string]any{"name": step.ToolName, "arguments": arguments, "result": agentModelToolResult(step)})
		}
		completedPayload, _ := json.Marshal(completed)
		requestMessages = append(requestMessages, agentChatMessage{Role: "system", Content: "本轮已执行的工具及真实 TOOL_RESULT：" + string(completedPayload) + "。先对照目标检查结果；仍需工具时只输出下一条 TOOL_CALL，否则立即简洁总结。"})
	}
	currentIndex := len(messages) - 1
	for i := range messages {
		if messages[i].ID == run.MessageID {
			currentIndex = i
			break
		}
	}
	conversationStart := currentIndex
	for i := currentIndex - 1; i >= 1; i -= 2 {
		if messages[i].Role != model.AgentMessageRoleAssistant || messages[i-1].Role != model.AgentMessageRoleUser {
			break
		}
		conversationStart = i - 1
	}
	for _, message := range messages[conversationStart : currentIndex+1] {
		requestMessages = append(requestMessages, agentChatMessage{Role: string(message.Role), Content: message.Content})
	}
	if len(toolSteps) >= maxToolCalls {
		requestMessages = append(requestMessages, agentChatMessage{Role: "system", Content: "本次运行已达到工具调用上限，请根据已有真实结果直接总结，不要再请求工具。"})
	}
	bodyValue := map[string]any{"model": selection.Model.UpstreamModel, "messages": requestMessages, "tools": agentToolSchemas(), "tool_choice": "auto", "stream": false, "max_tokens": 256}
	body, err := json.Marshal(bodyValue)
	if err != nil {
		return completion, err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	var response *http.Response
	for attempt := 0; attempt < 2; attempt++ {
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, BuildModelChannelURL(selection.Channel, "/chat/completions"), bytes.NewReader(body))
		if requestErr != nil {
			return completion, requestErr
		}
		request.Header.Set("Authorization", "Bearer "+selection.Channel.APIKey)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Request-ID", requestID)
		request.Header.Set("Idempotency-Key", requestID)
		response, err = client.Do(request)
		if err == nil {
			break
		}
		if attempt > 0 || ctx.Err() != nil {
			return completion, err
		}
		select {
		case <-ctx.Done():
			return completion, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return completion, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return completion, fmt.Errorf("agent upstream returned status %d", response.StatusCode)
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return completion, err
	}
	if len(result.Choices) == 0 {
		return completion, errors.New("agent upstream returned no choices")
	}
	message := result.Choices[0].Message
	completion.Content = strings.TrimSpace(message.Content)
	if len(message.ToolCalls) > 1 {
		return completion, errors.New("agent upstream returned multiple tool calls")
	}
	if len(message.ToolCalls) == 1 {
		if len(toolSteps) >= maxToolCalls {
			return completion, errors.New("agent exceeded tool call limit")
		}
		call := message.ToolCalls[0]
		if call.Type != "function" {
			return completion, errors.New("agent upstream returned unsupported tool call")
		}
		toolName := canonicalAgentToolName(call.Function.Name)
		if toolName == "canvas.plan" && len(toolSteps) != 0 && failedAgentToolCallCount(toolSteps) == 0 {
			return completion, errors.New("agent requested a duplicate plan")
		}
		authorization, authorizationErr := agentRunNodeAuthorization(run)
		if authorizationErr != nil {
			return completion, authorizationErr
		}
		arguments, raw, decodeErr := decodeAgentToolArguments(toolName, call.Function.Arguments, authorization)
		if decodeErr != nil {
			return completion, decodeErr
		}
		if agentMediaRevisionLimitReached(run, toolSteps, toolName) {
			completion.Content = agentMediaRevisionLimitMessage(toolName)
			return completion, nil
		}
		if completedAgentToolCall(toolSteps, toolName, raw) {
			completion.Content = "请求的画布操作已完成。"
			return completion, nil
		}
		if failedAgentToolCall(toolSteps, toolName, raw) {
			completion.Content = "相同参数的操作已经失败，未重复执行。"
			return completion, nil
		}
		call.ID = newID("agent-tool-call")
		completion.ToolCall = &agentToolCall{ID: call.ID, Name: toolName, Arguments: arguments, Raw: raw}
		return completion, nil
	}
	if textName, textArguments, ok := parseAgentTextToolCall(completion.Content); ok {
		if len(toolSteps) >= maxToolCalls {
			return completion, errors.New("agent exceeded tool call limit")
		}
		textName = canonicalAgentToolName(textName)
		if textName == "canvas.plan" && len(toolSteps) != 0 && failedAgentToolCallCount(toolSteps) == 0 {
			return completion, errors.New("agent requested a duplicate plan")
		}
		authorization, authorizationErr := agentRunNodeAuthorization(run)
		if authorizationErr != nil {
			return completion, authorizationErr
		}
		arguments, raw, decodeErr := decodeAgentToolArguments(textName, textArguments, authorization)
		if decodeErr != nil {
			return completion, decodeErr
		}
		if agentMediaRevisionLimitReached(run, toolSteps, textName) {
			completion.Content = agentMediaRevisionLimitMessage(textName)
			return completion, nil
		}
		if completedAgentToolCall(toolSteps, textName, raw) {
			completion.Content = "请求的画布操作已完成。"
			return completion, nil
		}
		if failedAgentToolCall(toolSteps, textName, raw) {
			completion.Content = "相同参数的操作已经失败，未重复执行。"
			return completion, nil
		}
		completion.ToolCall = &agentToolCall{ID: newID("agent-tool-call"), Name: textName, Arguments: arguments, Raw: raw}
		return completion, nil
	}
	if textName, textArguments, ok := parseAgentNaturalLanguageToolCall(completion.Content); ok {
		if len(toolSteps) >= maxToolCalls {
			return completion, errors.New("agent exceeded tool call limit")
		}
		textName = canonicalAgentToolName(textName)
		if textName == "canvas.plan" && len(toolSteps) != 0 && failedAgentToolCallCount(toolSteps) == 0 {
			return completion, errors.New("agent requested a duplicate plan")
		}
		authorization, authorizationErr := agentRunNodeAuthorization(run)
		if authorizationErr != nil {
			return completion, authorizationErr
		}
		arguments, raw, decodeErr := decodeAgentToolArguments(textName, textArguments, authorization)
		if decodeErr != nil {
			return completion, decodeErr
		}
		if agentMediaRevisionLimitReached(run, toolSteps, textName) {
			completion.Content = agentMediaRevisionLimitMessage(textName)
			return completion, nil
		}
		if completedAgentToolCall(toolSteps, textName, raw) {
			completion.Content = "请求的画布操作已完成。"
			return completion, nil
		}
		if failedAgentToolCall(toolSteps, textName, raw) {
			completion.Content = "相同参数的操作已经失败，未重复执行。"
			return completion, nil
		}
		completion.ToolCall = &agentToolCall{ID: newID("agent-tool-call"), Name: textName, Arguments: arguments, Raw: raw}
		return completion, nil
	}
	if completion.Content == "" {
		return completion, errors.New("agent upstream returned empty content")
	}
	return completion, nil
}

func routeDeterministicAgentCompletion(run model.AgentRun, messages []model.AgentMessage, toolSteps []model.AgentStep) (agentCompletion, bool, error) {
	content := ""
	for _, message := range messages {
		if message.ID == run.MessageID && message.Role == model.AgentMessageRoleUser {
			content = strings.TrimSpace(message.Content)
			break
		}
	}
	if content == "" {
		return agentCompletion{}, false, nil
	}
	var canvasContext AgentCanvasContext
	if err := json.Unmarshal([]byte(run.Context), &canvasContext); err != nil {
		return agentCompletion{}, true, errors.New("agent canvas context invalid")
	}
	if canvasContext.Autonomy == agentAutonomyAutonomous && failedAgentToolCallCount(toolSteps) > 0 {
		return agentCompletion{}, false, nil
	}

	if containsAgentPhrase(content, "多少节点", "多少个节点", "几个节点", "节点数量") {
		return agentCompletion{Content: fmt.Sprintf("当前画布有 %d 个节点。", len(canvasContext.Nodes))}, true, nil
	}
	if text, placement, ok := parseExplicitAddTextRequest(content); ok {
		return deterministicAgentToolCompletion(run, toolSteps, "canvas.add_text", canvasAddTextArguments{Text: text, Placement: placement}, "文本已添加到画布。")
	}

	lower := strings.ToLower(content)
	mediaNegated := containsAgentPhrase(lower, "不要生成媒体", "不要生成图片", "不要生图", "别生成图片", "无需生成图片", "不要生成视频", "别生成视频", "无需生成视频")
	imageIntent, videoIntent := agentMediaIntent(lower, mediaNegated)
	if !imageIntent && !videoIntent {
		return agentCompletion{}, false, nil
	}
	if !simpleAgentMediaCommand(content) {
		return agentCompletion{}, false, nil
	}
	if canvasContext.Autonomy == agentAutonomyCautious {
		return agentCompletion{}, false, nil
	}
	for _, step := range toolSteps {
		if step.ToolName == "agent.ask_user" {
			return agentCompletion{}, false, nil
		}
	}
	if agentMediaRequestNeedsClarification(content) {
		return agentCompletion{}, false, nil
	}

	authorization, err := agentRunNodeAuthorization(run)
	if err != nil {
		return agentCompletion{}, true, err
	}
	planned := containsAgentPhrase(content, "策划", "规划", "计划")
	if planned {
		kind := "图片"
		if videoIntent {
			kind = "视频"
		}
		arguments, raw, err := normalizeDeterministicAgentToolArguments("canvas.plan", canvasPlanArguments{
			Summary: "策划并生成" + kind,
			Steps:   []string{"生成" + kind + "并添加到画布", "对照目标验收真实内容"},
		}, authorization)
		if err != nil {
			return agentCompletion{}, true, err
		}
		if !completedAgentToolCall(toolSteps, "canvas.plan", raw) {
			return agentCompletion{ToolCall: &agentToolCall{ID: newID("agent-tool-call"), Name: "canvas.plan", Arguments: arguments, Raw: raw}}, true, nil
		}
	}

	selectedImages := selectedAgentImageNodeIDs(canvasContext)
	if videoIntent {
		imageNodeID := ""
		if len(selectedImages) > 0 {
			imageNodeID = selectedImages[0]
		}
		generated := successfulAgentToolSteps(toolSteps, "video.generate")
		inspected := successfulAgentToolSteps(toolSteps, "video.inspect")
		if len(inspected) > 0 {
			if len(generated) == 0 {
				return agentCompletion{}, true, errors.New("agent video generation result missing")
			}
			inspection, err := agentStepInspection(inspected[len(inspected)-1])
			if err != nil {
				return agentCompletion{}, true, err
			}
			if inspection.Status == "needs_revision" && canvasContext.Autonomy == agentAutonomyAutonomous && len(generated) < 2 && inspection.RevisedPrompt != "" {
				previous, err := agentStepVideoArguments(generated[len(generated)-1])
				if err != nil {
					return agentCompletion{}, true, err
				}
				return deterministicAgentToolCompletionWithAuthorization(toolSteps, "video.generate", videoGenerateArguments{Prompt: inspection.RevisedPrompt, Duration: previous.Duration, ImageNodeID: previous.ImageNodeID}, "视频已根据视觉验收结果调整并重新生成。", authorization)
			}
			if inspection.Status == "passed" {
				return agentCompletion{Content: "视频已生成，视觉验收通过：" + inspection.Summary}, true, nil
			}
			if inspection.Status == "unavailable" {
				return agentCompletion{Content: "视频已生成；视觉验收暂不可用：" + inspection.Summary}, true, nil
			}
			return agentCompletion{Content: "视频已生成，但视觉验收仍发现问题：" + inspection.Summary}, true, nil
		}
		return deterministicAgentToolCompletionWithAuthorization(toolSteps, "video.generate", videoGenerateArguments{Prompt: agentVideoExecutionPrompt(content), Duration: 6, ImageNodeID: imageNodeID}, "视频已生成并添加到画布。", authorization)
	}
	if len(selectedImages) > 6 {
		selectedImages = selectedImages[:6]
	}
	generated := successfulAgentToolSteps(toolSteps, "image.generate")
	inspected := successfulAgentToolSteps(toolSteps, "image.inspect")
	if len(inspected) > 0 {
		inspection, err := agentStepInspection(inspected[len(inspected)-1])
		if err != nil {
			return agentCompletion{}, true, err
		}
		if inspection.Status == "needs_revision" && canvasContext.Autonomy == agentAutonomyAutonomous && len(generated) < 2 && inspection.RevisedPrompt != "" {
			return deterministicAgentToolCompletionWithAuthorization(toolSteps, "image.generate", imageGenerateArguments{Prompt: inspection.RevisedPrompt, Count: 1, ReferenceNodeIDs: selectedImages}, "图片已根据视觉验收结果调整并重新生成。", authorization)
		}
		if inspection.Status == "passed" {
			return agentCompletion{Content: "图片已生成，视觉验收通过：" + inspection.Summary}, true, nil
		}
		if inspection.Status == "unavailable" {
			return agentCompletion{Content: "图片已生成；视觉验收暂不可用：" + inspection.Summary}, true, nil
		}
		return agentCompletion{Content: "图片已生成，但视觉验收仍发现问题：" + inspection.Summary}, true, nil
	}
	return deterministicAgentToolCompletionWithAuthorization(toolSteps, "image.generate", imageGenerateArguments{Prompt: agentImageExecutionPrompt(content), Count: 1, ReferenceNodeIDs: selectedImages}, "图片已生成并添加到画布。", authorization)
}

func deterministicAgentToolCompletion(run model.AgentRun, steps []model.AgentStep, name string, input any, completedText string) (agentCompletion, bool, error) {
	authorization, err := agentRunNodeAuthorization(run)
	if err != nil {
		return agentCompletion{}, true, err
	}
	return deterministicAgentToolCompletionWithAuthorization(steps, name, input, completedText, authorization)
}

func deterministicAgentToolCompletionWithAuthorization(steps []model.AgentStep, name string, input any, completedText string, authorization agentNodeAuthorization) (agentCompletion, bool, error) {
	arguments, raw, err := normalizeDeterministicAgentToolArguments(name, input, authorization)
	if err != nil {
		return agentCompletion{}, true, err
	}
	if completedAgentToolCall(steps, name, raw) {
		return agentCompletion{Content: completedText}, true, nil
	}
	return agentCompletion{ToolCall: &agentToolCall{ID: newID("agent-tool-call"), Name: name, Arguments: arguments, Raw: raw}}, true, nil
}

func normalizeDeterministicAgentToolArguments(name string, input any, authorization agentNodeAuthorization) (any, string, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, "", err
	}
	return decodeAgentToolArguments(name, string(raw), authorization)
}

func parseExplicitAddTextRequest(content string) (string, string, bool) {
	if !strings.Contains(strings.ToLower(content), "canvas.add_text") {
		return "", "", false
	}
	match := regexp.MustCompile(`(?i)(?:参数\s*)?text\s*(?:为|是|=|:|：)\s*[“"']([^”"']+)[”"']`).FindStringSubmatch(content)
	if len(match) < 2 {
		match = regexp.MustCompile(`(?i)(?:参数\s*)?text\s*(?:为|是|=|:|：)\s*([^,，;；\n]+)`).FindStringSubmatch(content)
	}
	if len(match) < 2 || strings.TrimSpace(match[1]) == "" {
		return "", "", false
	}
	placement := "center"
	if placementMatch := regexp.MustCompile(`(?i)placement\s*(?:为|是|=|:|：)\s*(center|right_of_selection)`).FindStringSubmatch(content); len(placementMatch) > 1 {
		placement = strings.ToLower(placementMatch[1])
	}
	return strings.TrimSpace(match[1]), placement, true
}

func selectedAgentImageNodeIDs(canvasContext AgentCanvasContext) []string {
	images := make(map[string]struct{})
	for _, node := range canvasContext.Nodes {
		if node.Type == "image" {
			images[node.ID] = struct{}{}
		}
	}
	result := make([]string, 0, len(canvasContext.SelectedNodeIDs))
	for _, id := range canvasContext.SelectedNodeIDs {
		if _, valid := images[id]; valid {
			result = append(result, id)
		}
	}
	return result
}

func containsAgentPhrase(content string, phrases ...string) bool {
	for _, phrase := range phrases {
		if strings.Contains(content, phrase) {
			return true
		}
	}
	return false
}

type agentRememberArguments struct {
	Kind          string  `json:"kind"`
	Key           string  `json:"key"`
	Content       string  `json:"content"`
	Scope         string  `json:"scope"`
	Confidence    float64 `json:"confidence"`
	ExpiresInDays int     `json:"expiresInDays"`
}

type agentForgetArguments struct {
	Key   string `json:"key"`
	Scope string `json:"scope"`
}

type AgentToolMemory struct {
	ID     string `json:"id,omitempty"`
	Key    string `json:"key"`
	Status string `json:"status"`
	Scope  string `json:"scope"`
}

func agentMediaRequestNeedsClarification(content string) bool {
	detail := strings.NewReplacer(
		"我想要", "", "想要", "", "帮我", "", "请", "", "给我", "", "策划", "", "规划", "", "计划", "", "生成", "", "制作", "", "做", "", "生图", "",
		"商品主图", "", "电商主图", "", "场景图", "", "短视频", "", "视频", "", "海报", "", "图片", "", "图", "",
		"一张", "", "一个", "", "一下", "", "并", "",
	).Replace(strings.ToLower(strings.TrimSpace(content)))
	return strings.Trim(detail, " \t\r\n,，.。!！?？:：;；") == ""
}

func agentMediaIntent(content string, negated bool) (bool, bool) {
	mediaAction := containsAgentPhrase(content, "生成", "制作", "做一张", "画一张")
	video := !negated && (containsAgentPhrase(content, "生成视频", "生成短视频", "制作视频", "制作短视频", "我要视频", "我要的是视频", "要的是视频", "做成视频") || (mediaAction && containsAgentPhrase(content, "视频", "短片")))
	image := !negated && !video && (containsAgentPhrase(content, "生成图片", "生成一张图", "生成一张图片", "生图", "商品主图", "电商主图", "场景图", "海报") || (mediaAction && containsAgentPhrase(content, "图片", "图像", "照片")))
	return image, video
}

func simpleAgentMediaCommand(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" || strings.ContainsAny(content, "；;\n") {
		return false
	}
	if strings.HasPrefix(content, "先") || containsAgentPhrase(content, "，先", ",先", "然后", "接着", "随后", "再把", "并且", "同时") {
		return false
	}
	return !containsAgentPhrase(content, "分析画布", "分析图片", "比较图片", "对比图片", "排列节点", "整理画布", "添加文本", "添加文案", "删除节点", "修改文字", "更新文字", "记住这个", "忘记这个")
}

func agentImageExecutionPrompt(content string) string {
	return "创作目标：" + strings.TrimSpace(content) + "\n执行规格：保持用户明确指定的主体数量、风格和构图；未指定时采用单一核心主体、完整构图、自然协调的光线与背景和清晰细节；不要添加用户未要求的文字、标志或水印。"
}

func agentVideoExecutionPrompt(content string) string {
	return "创作目标：" + strings.TrimSpace(content) + "\n执行规格：保持用户明确指定的主体、风格、镜头和节奏；未指定时采用主体清晰、动作连贯、镜头稳定、光线与背景协调的短镜头；不要添加用户未要求的文字、标志、水印或突兀转场。"
}

func routePendingAgentImageInspection(run model.AgentRun, messages []model.AgentMessage, toolSteps []model.AgentStep) (agentCompletion, bool, error) {
	nodeIDs, pending, err := pendingAgentImageInspection(toolSteps)
	if err != nil || !pending {
		return agentCompletion{}, pending, err
	}
	content := ""
	for _, message := range messages {
		if message.ID == run.MessageID && message.Role == model.AgentMessageRoleUser {
			content = strings.TrimSpace(message.Content)
			break
		}
	}
	if content == "" {
		return agentCompletion{}, true, errors.New("agent image inspection goal missing")
	}
	authorization, err := agentRunNodeAuthorization(run)
	if err != nil {
		return agentCompletion{}, true, err
	}
	return deterministicAgentToolCompletionWithAuthorization(toolSteps, "image.inspect", imageInspectArguments{NodeIDs: nodeIDs, Criteria: agentImageInspectionCriteria(content)}, "图片已生成并完成视觉验收。", authorization)
}

func pendingAgentImageInspection(steps []model.AgentStep) ([]string, bool, error) {
	inspected := make(map[string]struct{})
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		if step.Status != model.AgentStepStatusCompleted {
			continue
		}
		switch step.ToolName {
		case "image.inspect":
			var input imageInspectArguments
			if json.Unmarshal([]byte(step.Input), &input) != nil {
				return nil, false, errors.New("agent image inspection input invalid")
			}
			for _, id := range input.NodeIDs {
				inspected[id] = struct{}{}
			}
		case "image.generate", "image.edit":
			var result SubmitAgentToolResultRequest
			if json.Unmarshal([]byte(step.Output), &result) != nil || result.Status != "success" || len(result.Images) == 0 {
				return nil, false, errors.New("agent image result invalid")
			}
			pending := make([]string, 0, len(result.Images))
			for _, image := range result.Images {
				if _, exists := inspected[image.NodeID]; !exists {
					pending = append(pending, image.NodeID)
				}
			}
			if len(pending) > 0 {
				return pending, true, nil
			}
		}
	}
	return nil, false, nil
}

func agentImageInspectionCriteria(content string) string {
	return agentImageExecutionPrompt(content) + "\n验收主体是否正确、关键要求是否满足、构图与细节是否存在明显缺陷。"
}

func routePendingAgentVideoInspection(run model.AgentRun, messages []model.AgentMessage, toolSteps []model.AgentStep) (agentCompletion, bool, error) {
	nodeID, pending, err := pendingAgentVideoInspection(toolSteps)
	if err != nil || !pending {
		return agentCompletion{}, pending, err
	}
	content := ""
	for _, message := range messages {
		if message.ID == run.MessageID && message.Role == model.AgentMessageRoleUser {
			content = strings.TrimSpace(message.Content)
			break
		}
	}
	if content == "" {
		return agentCompletion{}, true, errors.New("agent video inspection goal missing")
	}
	authorization, err := agentRunNodeAuthorization(run)
	if err != nil {
		return agentCompletion{}, true, err
	}
	return deterministicAgentToolCompletionWithAuthorization(toolSteps, "video.inspect", videoInspectArguments{NodeID: nodeID, Criteria: agentVideoInspectionCriteria(content)}, "视频已生成并完成视觉验收。", authorization)
}

func pendingAgentVideoInspection(steps []model.AgentStep) (string, bool, error) {
	inspected := make(map[string]struct{})
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		if step.Status != model.AgentStepStatusCompleted {
			continue
		}
		switch step.ToolName {
		case "video.inspect":
			var input videoInspectArguments
			if json.Unmarshal([]byte(step.Input), &input) != nil {
				return "", false, errors.New("agent video inspection input invalid")
			}
			inspected[input.NodeID] = struct{}{}
		case "video.generate":
			var result SubmitAgentToolResultRequest
			if json.Unmarshal([]byte(step.Output), &result) != nil || result.Status != "success" || result.Video == nil {
				return "", false, errors.New("agent video result invalid")
			}
			if _, exists := inspected[result.Video.NodeID]; !exists {
				return result.Video.NodeID, true, nil
			}
		}
	}
	return "", false, nil
}

func agentVideoInspectionCriteria(content string) string {
	return agentVideoExecutionPrompt(content) + "\n请按时间顺序验收关键帧中的主体一致性、动作和镜头连续性、构图、文字与明显画面缺陷；不要臆测关键帧无法证明的声音或完整运动细节。"
}

func agentMediaRevisionLimitReached(run model.AgentRun, steps []model.AgentStep, proposedTool string) bool {
	inspectTool := ""
	switch proposedTool {
	case "image.generate", "image.edit":
		inspectTool = "image.inspect"
	case "video.generate":
		inspectTool = "video.inspect"
	default:
		return false
	}
	pendingRevision, adjustmentUsed := false, false
	for _, step := range steps {
		if step.Status != model.AgentStepStatusCompleted {
			continue
		}
		if step.ToolName == inspectTool {
			inspection, err := agentStepInspection(step)
			if err == nil {
				pendingRevision = inspection.Status == "needs_revision"
			}
		} else if step.ToolName == proposedTool || (inspectTool == "image.inspect" && (step.ToolName == "image.generate" || step.ToolName == "image.edit")) {
			if pendingRevision {
				adjustmentUsed, pendingRevision = true, false
			}
		}
	}
	if !pendingRevision {
		return false
	}
	return agentRunAutonomy(run) != agentAutonomyAutonomous || adjustmentUsed
}

func agentMediaRevisionLimitMessage(tool string) string {
	if tool == "video.generate" {
		return "视频视觉验收仍有问题，当前自主策略不再继续生成。"
	}
	return "图片视觉验收仍有问题，当前自主策略不再继续生成。"
}

func successfulAgentToolSteps(steps []model.AgentStep, name string) []model.AgentStep {
	result := make([]model.AgentStep, 0)
	for _, step := range steps {
		if step.Status == model.AgentStepStatusCompleted && step.ToolName == name {
			result = append(result, step)
		}
	}
	return result
}

func agentStepInspection(step model.AgentStep) (AgentToolInspection, error) {
	var result SubmitAgentToolResultRequest
	if json.Unmarshal([]byte(step.Output), &result) != nil || result.Status != "success" || result.Inspection == nil {
		return AgentToolInspection{}, errors.New("agent media inspection invalid")
	}
	return *result.Inspection, nil
}

func agentStepVideoArguments(step model.AgentStep) (videoGenerateArguments, error) {
	var result videoGenerateArguments
	if json.Unmarshal([]byte(step.Input), &result) != nil || result.Prompt == "" || result.Duration < 1 {
		return videoGenerateArguments{}, errors.New("agent video arguments invalid")
	}
	return result, nil
}

func agentRunAutonomy(run model.AgentRun) string {
	var canvasContext AgentCanvasContext
	if json.Unmarshal([]byte(run.Context), &canvasContext) != nil {
		return agentAutonomyStandard
	}
	autonomy, valid := normalizeAgentAutonomy(canvasContext.Autonomy)
	if !valid {
		return agentAutonomyStandard
	}
	return autonomy
}

func normalizeAgentAutonomy(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", agentAutonomyStandard:
		return agentAutonomyStandard, true
	case agentAutonomyCautious:
		return agentAutonomyCautious, true
	case agentAutonomyAutonomous:
		return agentAutonomyAutonomous, true
	default:
		return "", false
	}
}

func agentAutonomyPrompt(autonomy string) string {
	switch autonomy {
	case agentAutonomyCautious:
		return "本轮自主等级为谨慎：仍要优先从上下文推断；只有会显著改变结果且无法可靠推断的信息才使用 agent.ask_user。"
	case agentAutonomyAutonomous:
		return "本轮自主等级为自主：风格、构图、数量和时长等可安全默认的信息直接补全并执行；每个 TOOL_RESULT 后主动评估可观察的完成条件，目标未完成就继续下一步。非破坏性工具失败时可以修改参数重试一次，禁止原参数重放；重试仍失败则说明阻塞。删除、覆盖文本、记忆写入和遗忘仍不得绕过确认。"
	default:
		return "本轮自主等级为标准：风格、构图、数量和时长等可安全默认的信息直接补全并执行，不要为这些信息提问；默认单张图片、6 秒视频，并在计划或总结中简短说明关键假设。只有没有可执行主体或目标、约束冲突、或必须目标无法唯一确定时才询问。"
	}
}

func retryableAgentTool(name string) bool {
	definition, ok := agentToolDefinitionFor(name)
	return ok && definition.Retryable
}

func revertibleAgentTool(name string) bool {
	definition, ok := agentToolDefinitionFor(name)
	return ok && definition.Revertible
}

func agentStepSucceeded(step model.AgentStep) bool {
	if step.Status != model.AgentStepStatusCompleted || step.Confirmation == "rejected" || step.Output == "" {
		return false
	}
	var result SubmitAgentToolResultRequest
	return json.Unmarshal([]byte(step.Output), &result) == nil && result.Status == "success"
}

func agentModelToolResult(step model.AgentStep) any {
	var request SubmitAgentToolResultRequest
	if json.Unmarshal([]byte(step.Output), &request) != nil {
		return map[string]string{"status": string(step.Status)}
	}
	if request.Status == "failed" {
		return map[string]string{"status": request.Status, "error": request.Error}
	}
	switch step.ToolName {
	case "image.generate", "image.edit":
		nodeIDs := make([]string, 0, len(request.Images))
		for _, image := range request.Images {
			nodeIDs = append(nodeIDs, image.NodeID)
		}
		return map[string]any{"status": request.Status, "nodeIds": nodeIDs}
	case "video.generate":
		nodeID := ""
		if request.Video != nil {
			nodeID = request.Video.NodeID
		}
		return map[string]any{"status": request.Status, "nodeId": nodeID}
	default:
		return map[string]any{"status": request.Status, "output": agentToolResultOutput(step.ToolName, request)}
	}
}

func revertedAgentToolCallIDs(events []model.AgentEvent) map[string]struct{} {
	result := make(map[string]struct{})
	for _, event := range events {
		if event.Type != model.AgentEventToolReverted {
			continue
		}
		var payload struct {
			CallID string `json:"callId"`
		}
		if json.Unmarshal([]byte(event.Payload), &payload) == nil && payload.CallID != "" {
			result[payload.CallID] = struct{}{}
		}
	}
	return result
}

func agentDurationMS(start, end string) int64 {
	startedAt, err := time.Parse(timestampLayout, start)
	if err != nil {
		return 0
	}
	finishedAt := time.Now().UTC()
	if end != "" {
		if parsed, parseErr := time.Parse(timestampLayout, end); parseErr == nil {
			finishedAt = parsed
		}
	}
	if finishedAt.Before(startedAt) {
		return 0
	}
	return finishedAt.Sub(startedAt).Milliseconds()
}

func failedAgentToolCallCount(steps []model.AgentStep) int {
	count := 0
	for _, step := range steps {
		if step.Status == model.AgentStepStatusFailed {
			count++
		}
	}
	return count
}

func completedAgentToolCall(steps []model.AgentStep, name, input string) bool {
	for _, step := range steps {
		if step.Status == model.AgentStepStatusCompleted && step.ToolName == name && step.Input == input {
			return true
		}
	}
	return false
}

func failedAgentToolCall(steps []model.AgentStep, name, input string) bool {
	for _, step := range steps {
		if step.Status == model.AgentStepStatusFailed && step.ToolName == name && step.Input == input {
			return true
		}
	}
	return false
}

func normalizeAgentCanvasContext(value AgentCanvasContext) (AgentCanvasContext, error) {
	autonomy, validAutonomy := normalizeAgentAutonomy(value.Autonomy)
	if !validAutonomy {
		return AgentCanvasContext{}, safeMessageError{message: "Agent 自主等级无效"}
	}
	if len(value.SelectedNodeIDs) > 20 {
		return AgentCanvasContext{}, safeMessageError{message: "选中节点过多"}
	}
	if len(value.FocusNodeIDs) > 20 {
		return AgentCanvasContext{}, safeMessageError{message: "关注节点过多"}
	}
	if len(value.Nodes) > 200 {
		return AgentCanvasContext{}, safeMessageError{message: "画布节点过多"}
	}
	selected := make(map[string]struct{}, len(value.SelectedNodeIDs))
	selectedIDs := make([]string, 0, len(value.SelectedNodeIDs))
	for _, id := range value.SelectedNodeIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return AgentCanvasContext{}, safeMessageError{message: "选中节点编号无效"}
		}
		if _, exists := selected[id]; exists {
			return AgentCanvasContext{}, safeMessageError{message: "选中节点编号重复"}
		}
		selected[id] = struct{}{}
		selectedIDs = append(selectedIDs, id)
	}
	nodes := make(map[string]AgentCanvasNode, len(value.Nodes))
	nodeIDs := make([]string, 0, len(value.Nodes))
	for _, node := range value.Nodes {
		node.ID = strings.TrimSpace(node.ID)
		if node.ID == "" {
			return AgentCanvasContext{}, safeMessageError{message: "画布节点编号无效"}
		}
		if _, exists := nodes[node.ID]; exists {
			return AgentCanvasContext{}, safeMessageError{message: "画布节点编号重复"}
		}
		node.Type, node.Title = strings.TrimSpace(node.Type), strings.TrimSpace(node.Title)
		if node.Title == "" {
			return AgentCanvasContext{}, safeMessageError{message: "画布节点标题无效"}
		}
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
			if reference == "" {
				return AgentCanvasContext{}, safeMessageError{message: "画布节点来源编号无效"}
			}
			if _, exists := seenReferences[reference]; exists {
				return AgentCanvasContext{}, safeMessageError{message: "画布节点来源编号重复"}
			}
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
		if connection.From == "" || connection.To == "" {
			continue
		}
		if _, exists := nodes[connection.From]; !exists {
			continue
		}
		if _, exists := nodes[connection.To]; !exists {
			continue
		}
		key := connection.From + ">" + connection.To
		if _, exists := seenConnections[key]; exists {
			continue
		}
		seenConnections[key] = struct{}{}
		connections = append(connections, connection)
	}
	validSelectedIDs := make([]string, 0, len(selectedIDs))
	for _, id := range selectedIDs {
		if _, exists := nodes[id]; exists {
			validSelectedIDs = append(validSelectedIDs, id)
		}
	}
	focusIDs := make([]string, 0, len(value.FocusNodeIDs))
	seenFocus := make(map[string]struct{}, len(value.FocusNodeIDs))
	for _, id := range value.FocusNodeIDs {
		id = strings.TrimSpace(id)
		if _, exists := nodes[id]; !exists {
			continue
		}
		if _, exists := seenFocus[id]; exists {
			continue
		}
		seenFocus[id] = struct{}{}
		focusIDs = append(focusIDs, id)
	}
	result := AgentCanvasContext{Autonomy: autonomy, SelectedNodeIDs: validSelectedIDs, FocusNodeIDs: focusIDs, Nodes: make([]AgentCanvasNode, 0, len(nodes)), Connections: connections}
	for _, id := range nodeIDs {
		node, exists := nodes[id]
		if !exists {
			continue
		}
		result.Nodes = append(result.Nodes, node)
	}
	return result, nil
}

func agentModelCanvasContext(raw string, steps []model.AgentStep) (string, error) {
	var full AgentCanvasContext
	if err := json.Unmarshal([]byte(raw), &full); err != nil {
		return "", errors.New("agent canvas context invalid")
	}
	focus := make(map[string]struct{}, len(full.FocusNodeIDs)+len(full.SelectedNodeIDs))
	for _, id := range append(append([]string{}, full.FocusNodeIDs...), full.SelectedNodeIDs...) {
		focus[id] = struct{}{}
	}
	seeds := make(map[string]struct{}, len(focus))
	for id := range focus {
		seeds[id] = struct{}{}
	}
	for _, connection := range full.Connections {
		if _, ok := seeds[connection.From]; ok {
			focus[connection.To] = struct{}{}
		}
		if _, ok := seeds[connection.To]; ok {
			focus[connection.From] = struct{}{}
		}
	}
	typeCounts := make(map[string]int)
	retainedNodes := make([]AgentCanvasNode, 0, len(focus))
	retained := make(map[string]struct{}, len(focus))
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, node := range full.Nodes {
		typeCounts[node.Type]++
		minX, minY = math.Min(minX, node.X), math.Min(minY, node.Y)
		maxX, maxY = math.Max(maxX, node.X+node.Width), math.Max(maxY, node.Y+node.Height)
		if _, ok := focus[node.ID]; !ok {
			continue
		}
		node.StorageKey = ""
		retained[node.ID] = struct{}{}
		retainedNodes = append(retainedNodes, node)
	}
	connections := make([]AgentCanvasConnection, 0)
	for _, connection := range full.Connections {
		_, fromOK := retained[connection.From]
		_, toOK := retained[connection.To]
		if fromOK && toOK {
			connections = append(connections, connection)
		}
	}
	generatedNodeIDs := make([]string, 0)
	seenGenerated := make(map[string]struct{})
	for _, step := range steps {
		if !agentStepSucceeded(step) {
			continue
		}
		var result SubmitAgentToolResultRequest
		_ = json.Unmarshal([]byte(step.Output), &result)
		ids := append([]string{}, result.NodeIDs...)
		for _, image := range result.Images {
			ids = append(ids, image.NodeID)
		}
		if result.Video != nil {
			ids = append(ids, result.Video.NodeID)
		}
		if result.NodeID != "" {
			ids = append(ids, result.NodeID)
		}
		for _, id := range ids {
			if id == "" {
				continue
			}
			if _, exists := seenGenerated[id]; !exists {
				seenGenerated[id] = struct{}{}
				generatedNodeIDs = append(generatedNodeIDs, id)
			}
		}
	}
	bounds := map[string]float64{}
	if len(full.Nodes) > 0 {
		bounds = map[string]float64{"minX": minX, "minY": minY, "maxX": maxX, "maxY": maxY}
	}
	payload := map[string]any{
		"autonomy": full.Autonomy, "selectedNodeIds": full.SelectedNodeIDs, "focusNodeIds": full.FocusNodeIDs,
		"nodes": retainedNodes, "connections": connections,
		"summary": map[string]any{"nodeCount": len(full.Nodes), "typeCounts": typeCounts, "bounds": bounds, "generatedNodeIds": generatedNodeIDs},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func agentRunNodeAuthorization(run model.AgentRun) (agentNodeAuthorization, error) {
	authorization := agentNodeAuthorization{NodeIDs: map[string]struct{}{}, ImageNodeIDs: map[string]struct{}{}, VideoNodeIDs: map[string]struct{}{}, TextNodeIDs: map[string]struct{}{}}
	var canvasContext AgentCanvasContext
	if err := json.Unmarshal([]byte(run.Context), &canvasContext); err != nil {
		return authorization, errors.New("agent canvas context invalid")
	}
	for _, node := range canvasContext.Nodes {
		authorization.NodeIDs[node.ID] = struct{}{}
		if node.Type == "image" {
			authorization.ImageNodeIDs[node.ID] = struct{}{}
		}
		if node.Type == "video" {
			authorization.VideoNodeIDs[node.ID] = struct{}{}
		}
		if node.Type == "text" {
			authorization.TextNodeIDs[node.ID] = struct{}{}
		}
	}
	steps, err := repository.ListCompletedAgentToolSteps(run.OrganizationID, run.UserID, run.ID)
	if err != nil {
		return authorization, err
	}
	for _, step := range steps {
		var result SubmitAgentToolResultRequest
		if json.Unmarshal([]byte(step.Output), &result) != nil || result.Status != "success" {
			continue
		}
		for _, image := range result.Images {
			authorization.NodeIDs[image.NodeID], authorization.ImageNodeIDs[image.NodeID] = struct{}{}, struct{}{}
		}
		if result.Video != nil {
			authorization.NodeIDs[result.Video.NodeID], authorization.VideoNodeIDs[result.Video.NodeID] = struct{}{}, struct{}{}
		}
		if result.NodeID != "" {
			authorization.NodeIDs[result.NodeID] = struct{}{}
			if step.ToolName == "canvas.add_text" {
				authorization.TextNodeIDs[result.NodeID] = struct{}{}
			}
		}
		for _, id := range result.NodeIDs {
			authorization.NodeIDs[id] = struct{}{}
		}
	}
	return authorization, nil
}

func parseAgentTextToolCall(content string) (string, string, bool) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < len("TOOL_CALL ") || !strings.EqualFold(line[:len("TOOL_CALL ")], "TOOL_CALL ") {
			continue
		}
		payload := strings.TrimSpace(line[len("TOOL_CALL "):])
		var call struct {
			Name      string          `json:"name"`
			Action    string          `json:"action"`
			Tool      string          `json:"tool"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal([]byte(payload), &call) != nil {
			continue
		}
		if call.Name == "" {
			call.Name = call.Action
		}
		if call.Name == "" {
			call.Name = call.Tool
		}
		if call.Name == "" {
			continue
		}
		if len(call.Arguments) == 0 {
			var flat map[string]json.RawMessage
			if json.Unmarshal([]byte(payload), &flat) != nil {
				continue
			}
			delete(flat, "name")
			delete(flat, "action")
			delete(flat, "tool")
			call.Arguments, _ = json.Marshal(flat)
		}
		if len(call.Arguments) == 0 {
			continue
		}
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
		if len(match) < 3 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(match[1]))
		rest := match[2]
		switch name {
		case "canvas.plan":
			summaryMatch := regexp.MustCompile(`(?i)summary\s+is\s+(.+?)(?:,\s*steps\s+is\s+|$)`).FindStringSubmatch(rest)
			stepsMatch := regexp.MustCompile(`(?i)steps\s+is\s+(\[[^\]]*\])`).FindStringSubmatch(rest)
			if len(summaryMatch) < 2 || len(stepsMatch) < 2 {
				continue
			}
			var steps []string
			if json.Unmarshal([]byte(stepsMatch[1]), &steps) != nil {
				continue
			}
			raw, _ := json.Marshal(map[string]any{"summary": strings.Trim(strings.TrimSpace(summaryMatch[1]), `"'`), "steps": steps})
			return name, string(raw), true
		case "image.generate":
			promptMatch := regexp.MustCompile(`(?i)prompt\s+is\s+(.+)`).FindStringSubmatch(rest)
			if len(promptMatch) < 2 {
				continue
			}
			raw, _ := json.Marshal(map[string]any{"prompt": strings.Trim(strings.TrimSpace(promptMatch[1]), `"'`)})
			return name, string(raw), true
		case "video.generate":
			promptMatch := regexp.MustCompile(`(?i)prompt\s+is\s+(.+)`).FindStringSubmatch(rest)
			if len(promptMatch) < 2 {
				continue
			}
			raw, _ := json.Marshal(map[string]any{"prompt": strings.Trim(strings.TrimSpace(promptMatch[1]), `"'`)})
			return name, string(raw), true
		case "canvas.arrange":
			nodesMatch := regexp.MustCompile(`nodes\s+is\s+(\[[^\]]*\])`).FindStringSubmatch(rest)
			if len(nodesMatch) < 2 {
				continue
			}
			var nodeIDs []string
			if err := json.Unmarshal([]byte(nodesMatch[1]), &nodeIDs); err != nil {
				continue
			}
			modeMatch := regexp.MustCompile(`direction\s+is\s+(horizontal|vertical|grid)`).FindStringSubmatch(rest)
			if len(modeMatch) < 2 {
				continue
			}
			gap := 40
			if gapMatch := regexp.MustCompile(`spacing\s+is\s+(\d+)`).FindStringSubmatch(rest); len(gapMatch) > 1 {
				if value, err := strconv.Atoi(gapMatch[1]); err == nil {
					gap = value
				}
			}
			raw, _ := json.Marshal(map[string]any{"nodeIds": nodeIDs, "mode": modeMatch[1], "gap": gap})
			return name, string(raw), true
		case "canvas.delete":
			nodesMatch := regexp.MustCompile(`nodes\s+is\s+(\[[^\]]*\])`).FindStringSubmatch(rest)
			if len(nodesMatch) < 2 {
				continue
			}
			var nodeIDs []string
			if err := json.Unmarshal([]byte(nodesMatch[1]), &nodeIDs); err != nil {
				continue
			}
			raw, _ := json.Marshal(map[string]any{"nodeIds": nodeIDs})
			return name, string(raw), true
		case "canvas.add_text":
			textMatch := regexp.MustCompile(`text\s+is\s+(.+)`).FindStringSubmatch(rest)
			if len(textMatch) < 2 {
				continue
			}
			text := strings.TrimSpace(textMatch[1])
			if quoted := quotedPattern.FindStringSubmatch(text); len(quoted) > 1 {
				text = quoted[1]
			}
			placement := "center"
			if placementMatch := regexp.MustCompile(`placement\s+is\s+(center|right_of_selection)`).FindStringSubmatch(rest); len(placementMatch) > 1 {
				placement = placementMatch[1]
			}
			raw, _ := json.Marshal(map[string]any{"text": text, "placement": placement})
			return name, string(raw), true
		case "canvas.update_text":
			nodeIDMatch := regexp.MustCompile(`nodeId\s+is\s+([^\s]+)`).FindStringSubmatch(rest)
			textMatch := regexp.MustCompile(`text\s+is\s+(.+)`).FindStringSubmatch(rest)
			if len(nodeIDMatch) < 2 || len(textMatch) < 2 {
				continue
			}
			nodeID := strings.Trim(nodeIDMatch[1], `"'`)
			text := strings.TrimSpace(textMatch[1])
			if quoted := quotedPattern.FindStringSubmatch(text); len(quoted) > 1 {
				text = quoted[1]
			}
			raw, _ := json.Marshal(map[string]any{"nodeId": nodeID, "text": text})
			return name, string(raw), true
		}
	}
	return "", "", false
}

func decodeAgentToolArguments(name, value string, authorization agentNodeAuthorization) (any, string, error) {
	name = canonicalAgentToolName(name)
	var arguments any
	switch name {
	case "canvas.plan":
		var input canvasPlanArguments
		if err := decodeAgentJSONValue(value, &input); err != nil {
			return nil, "", errors.New("canvas.plan arguments invalid")
		}
		input.Summary = strings.TrimSpace(input.Summary)
		if input.Summary == "" || utf8.RuneCountInString(input.Summary) > 120 || len(input.Steps) < 2 || len(input.Steps) > 7 {
			return nil, "", errors.New("canvas.plan arguments invalid")
		}
		for i := range input.Steps {
			input.Steps[i] = strings.TrimSpace(input.Steps[i])
			if input.Steps[i] == "" || utf8.RuneCountInString(input.Steps[i]) > 80 {
				return nil, "", errors.New("canvas.plan steps invalid")
			}
		}
		arguments = input
	case "image.generate":
		var input struct {
			Prompt           string   `json:"prompt"`
			Count            *int     `json:"count"`
			ReferenceNodeIDs []string `json:"referenceNodeIds"`
		}
		if err := decodeAgentJSONValue(value, &input); err != nil {
			return nil, "", errors.New("image.generate arguments invalid")
		}
		input.Prompt = strings.TrimSpace(input.Prompt)
		if input.Prompt == "" || utf8.RuneCountInString(input.Prompt) > 8000 {
			return nil, "", errors.New("image.generate prompt invalid")
		}
		count := 1
		if input.Count != nil {
			count = *input.Count
		}
		if count < 1 || count > 4 {
			return nil, "", errors.New("image.generate count invalid")
		}
		seenReferences := make(map[string]struct{}, len(input.ReferenceNodeIDs))
		for i, id := range input.ReferenceNodeIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				return nil, "", errors.New("image.generate referenceNodeIds invalid")
			}
			if _, exists := seenReferences[id]; exists {
				return nil, "", errors.New("image.generate referenceNodeIds invalid")
			}
			if _, valid := authorization.ImageNodeIDs[id]; !valid {
				return nil, "", errors.New("image.generate referenceNodeIds unauthorized")
			}
			seenReferences[id] = struct{}{}
			input.ReferenceNodeIDs[i] = id
		}
		arguments = imageGenerateArguments{Prompt: input.Prompt, Count: count, ReferenceNodeIDs: input.ReferenceNodeIDs}
	case "image.edit":
		var input struct {
			NodeID string `json:"nodeId"`
			Prompt string `json:"prompt"`
			Count  *int   `json:"count"`
		}
		if err := decodeAgentJSONValue(value, &input); err != nil {
			return nil, "", errors.New("image.edit arguments invalid")
		}
		input.NodeID, input.Prompt = strings.TrimSpace(input.NodeID), strings.TrimSpace(input.Prompt)
		if input.NodeID == "" || input.Prompt == "" || utf8.RuneCountInString(input.Prompt) > 8000 {
			return nil, "", errors.New("image.edit arguments invalid")
		}
		if _, valid := authorization.ImageNodeIDs[input.NodeID]; !valid {
			return nil, "", errors.New("image.edit nodeId unauthorized")
		}
		count := 1
		if input.Count != nil {
			count = *input.Count
		}
		if count < 1 || count > 4 {
			return nil, "", errors.New("image.edit count invalid")
		}
		arguments = imageEditArguments{NodeID: input.NodeID, Prompt: input.Prompt, Count: count}
	case "image.inspect":
		var input imageInspectArguments
		if err := decodeAgentJSONValue(value, &input); err != nil || len(input.NodeIDs) < 1 || len(input.NodeIDs) > 4 {
			return nil, "", errors.New("image.inspect arguments invalid")
		}
		input.Criteria = strings.TrimSpace(input.Criteria)
		if input.Criteria == "" || utf8.RuneCountInString(input.Criteria) > 2000 {
			return nil, "", errors.New("image.inspect criteria invalid")
		}
		seen := make(map[string]struct{}, len(input.NodeIDs))
		for i, id := range input.NodeIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				return nil, "", errors.New("image.inspect nodeIds invalid")
			}
			if _, exists := seen[id]; exists {
				return nil, "", errors.New("image.inspect nodeIds invalid")
			}
			if _, valid := authorization.ImageNodeIDs[id]; !valid {
				return nil, "", errors.New("image.inspect nodeIds unauthorized")
			}
			seen[id], input.NodeIDs[i] = struct{}{}, id
		}
		arguments = input
	case "video.generate":
		var input struct {
			Prompt      string `json:"prompt"`
			Duration    *int   `json:"duration"`
			ImageNodeID string `json:"imageNodeId"`
		}
		if err := decodeAgentJSONValue(value, &input); err != nil {
			return nil, "", errors.New("video.generate arguments invalid")
		}
		input.Prompt, input.ImageNodeID = strings.TrimSpace(input.Prompt), strings.TrimSpace(input.ImageNodeID)
		if input.Prompt == "" || utf8.RuneCountInString(input.Prompt) > 8000 {
			return nil, "", errors.New("video.generate prompt invalid")
		}
		duration := 6
		if input.Duration != nil {
			duration = *input.Duration
		}
		if duration < 1 || duration > 20 {
			return nil, "", errors.New("video.generate duration invalid")
		}
		if input.ImageNodeID != "" {
			if _, valid := authorization.ImageNodeIDs[input.ImageNodeID]; !valid {
				return nil, "", errors.New("video.generate imageNodeId unauthorized")
			}
		}
		arguments = videoGenerateArguments{Prompt: input.Prompt, Duration: duration, ImageNodeID: input.ImageNodeID}
	case "video.inspect":
		var input videoInspectArguments
		if err := decodeAgentJSONValue(value, &input); err != nil {
			return nil, "", errors.New("video.inspect arguments invalid")
		}
		input.NodeID, input.Criteria = strings.TrimSpace(input.NodeID), strings.TrimSpace(input.Criteria)
		if input.NodeID == "" || input.Criteria == "" || utf8.RuneCountInString(input.Criteria) > 2000 {
			return nil, "", errors.New("video.inspect arguments invalid")
		}
		if _, valid := authorization.VideoNodeIDs[input.NodeID]; !valid {
			return nil, "", errors.New("video.inspect nodeId unauthorized")
		}
		arguments = input
	case "canvas.arrange":
		var input struct {
			NodeIDs []string `json:"nodeIds"`
			Mode    string   `json:"mode"`
			Gap     *int     `json:"gap"`
		}
		if err := decodeAgentJSONValue(value, &input); err != nil {
			return nil, "", errors.New("canvas.arrange arguments invalid")
		}
		if len(input.NodeIDs) < 2 || len(input.NodeIDs) > 20 {
			return nil, "", errors.New("canvas.arrange nodeIds invalid")
		}
		seen := make(map[string]struct{}, len(input.NodeIDs))
		for i, id := range input.NodeIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				return nil, "", errors.New("canvas.arrange nodeIds invalid")
			}
			if _, exists := seen[id]; exists {
				return nil, "", errors.New("canvas.arrange nodeIds invalid")
			}
			if _, exists := authorization.NodeIDs[id]; !exists {
				return nil, "", errors.New("canvas.arrange nodeIds unauthorized")
			}
			seen[id], input.NodeIDs[i] = struct{}{}, id
		}
		input.Mode = strings.TrimSpace(input.Mode)
		if input.Mode != "horizontal" && input.Mode != "vertical" && input.Mode != "grid" {
			return nil, "", errors.New("canvas.arrange mode invalid")
		}
		gap := 40
		if input.Gap != nil {
			gap = *input.Gap
		}
		if gap < 16 || gap > 400 {
			return nil, "", errors.New("canvas.arrange gap invalid")
		}
		arguments = canvasArrangeArguments{NodeIDs: input.NodeIDs, Mode: input.Mode, Gap: gap}
	case "canvas.add_text":
		var input struct {
			Text          string   `json:"text"`
			Placement     string   `json:"placement"`
			SourceNodeIDs []string `json:"sourceNodeIds"`
		}
		if err := decodeAgentJSONValue(value, &input); err != nil {
			return nil, "", errors.New("canvas.add_text arguments invalid")
		}
		input.Text, input.Placement = strings.TrimSpace(input.Text), strings.TrimSpace(input.Placement)
		if input.Text == "" || utf8.RuneCountInString(input.Text) > 4000 {
			return nil, "", errors.New("canvas.add_text text invalid")
		}
		if input.Placement == "" {
			input.Placement = "center"
		}
		if input.Placement != "center" && input.Placement != "right_of_selection" {
			return nil, "", errors.New("canvas.add_text placement invalid")
		}
		if len(input.SourceNodeIDs) > 20 {
			return nil, "", errors.New("canvas.add_text sourceNodeIds invalid")
		}
		seen := make(map[string]struct{}, len(input.SourceNodeIDs))
		for i, id := range input.SourceNodeIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				return nil, "", errors.New("canvas.add_text sourceNodeIds invalid")
			}
			if _, exists := seen[id]; exists {
				return nil, "", errors.New("canvas.add_text sourceNodeIds invalid")
			}
			if _, exists := authorization.NodeIDs[id]; !exists {
				return nil, "", errors.New("canvas.add_text sourceNodeIds unauthorized")
			}
			seen[id], input.SourceNodeIDs[i] = struct{}{}, id
		}
		arguments = canvasAddTextArguments{Text: input.Text, Placement: input.Placement, SourceNodeIDs: input.SourceNodeIDs}
	case "canvas.delete":
		var input canvasDeleteArguments
		if err := decodeAgentJSONValue(value, &input); err != nil || len(input.NodeIDs) < 1 || len(input.NodeIDs) > 20 {
			return nil, "", errors.New("canvas.delete arguments invalid")
		}
		seen := make(map[string]struct{}, len(input.NodeIDs))
		for i, id := range input.NodeIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				return nil, "", errors.New("canvas.delete nodeIds invalid")
			}
			if _, exists := seen[id]; exists {
				return nil, "", errors.New("canvas.delete nodeIds invalid")
			}
			if _, exists := authorization.NodeIDs[id]; !exists {
				return nil, "", errors.New("canvas.delete nodeIds unauthorized")
			}
			seen[id], input.NodeIDs[i] = struct{}{}, id
		}
		arguments = input
	case "canvas.update_text":
		var input canvasUpdateTextArguments
		if err := decodeAgentJSONValue(value, &input); err != nil {
			return nil, "", errors.New("canvas.update_text arguments invalid")
		}
		input.NodeID, input.Text = strings.TrimSpace(input.NodeID), strings.TrimSpace(input.Text)
		if input.NodeID == "" || utf8.RuneCountInString(input.Text) > 4000 {
			return nil, "", errors.New("canvas.update_text arguments invalid")
		}
		if _, valid := authorization.TextNodeIDs[input.NodeID]; !valid {
			return nil, "", errors.New("canvas.update_text nodeId unauthorized")
		}
		arguments = input
	case "agent.ask_user":
		var input askUserArguments
		if err := decodeAgentJSONValue(value, &input); err != nil {
			return nil, "", errors.New("agent.ask_user arguments invalid")
		}
		input.Question = strings.TrimSpace(input.Question)
		if input.Question == "" || utf8.RuneCountInString(input.Question) > 500 || len(input.Options) < 3 || len(input.Options) > 4 {
			return nil, "", errors.New("agent.ask_user arguments invalid")
		}
		seenOptions := make(map[string]struct{}, len(input.Options))
		for i := range input.Options {
			input.Options[i] = strings.TrimSpace(input.Options[i])
			if input.Options[i] == "" || utf8.RuneCountInString(input.Options[i]) > 120 {
				return nil, "", errors.New("agent.ask_user options invalid")
			}
			if _, exists := seenOptions[input.Options[i]]; exists {
				return nil, "", errors.New("agent.ask_user options invalid")
			}
			seenOptions[input.Options[i]] = struct{}{}
		}
		arguments = input
	case "agent.remember":
		var input agentRememberArguments
		if err := decodeAgentJSONValue(value, &input); err != nil {
			return nil, "", errors.New("agent.remember arguments invalid")
		}
		input.Kind, input.Key, input.Content, input.Scope = strings.TrimSpace(input.Kind), strings.ToLower(strings.TrimSpace(input.Key)), strings.TrimSpace(input.Content), strings.TrimSpace(input.Scope)
		if input.Kind != string(model.AgentMemoryKindPreference) && input.Kind != string(model.AgentMemoryKindFact) && input.Kind != string(model.AgentMemoryKindConstraint) && input.Kind != string(model.AgentMemoryKindExperience) {
			return nil, "", errors.New("agent.remember kind invalid")
		}
		if input.Key == "" || utf8.RuneCountInString(input.Key) > 120 || !regexp.MustCompile(`^[\p{L}\p{N}_.:-]+$`).MatchString(input.Key) || sensitiveAgentMemoryKey(input.Key) {
			return nil, "", errors.New("agent.remember key invalid")
		}
		if input.Content == "" || utf8.RuneCountInString(input.Content) > 1000 {
			return nil, "", errors.New("agent.remember content invalid")
		}
		if input.Scope == "" {
			input.Scope = "project"
		}
		if input.Scope != "project" && input.Scope != "user" {
			return nil, "", errors.New("agent.remember scope invalid")
		}
		if input.Confidence == 0 {
			input.Confidence = 0.8
		}
		if input.Confidence < 0.5 || input.Confidence > 1 {
			return nil, "", errors.New("agent.remember confidence invalid")
		}
		if input.ExpiresInDays < 0 || input.ExpiresInDays > 3650 {
			return nil, "", errors.New("agent.remember expiry invalid")
		}
		arguments = input
	case "agent.forget":
		var input agentForgetArguments
		if err := decodeAgentJSONValue(value, &input); err != nil {
			return nil, "", errors.New("agent.forget arguments invalid")
		}
		input.Key, input.Scope = strings.ToLower(strings.TrimSpace(input.Key)), strings.TrimSpace(input.Scope)
		if input.Key == "" || utf8.RuneCountInString(input.Key) > 120 || !regexp.MustCompile(`^[\p{L}\p{N}_.:-]+$`).MatchString(input.Key) {
			return nil, "", errors.New("agent.forget key invalid")
		}
		if input.Scope == "" {
			input.Scope = "project"
		}
		if input.Scope != "project" && input.Scope != "user" {
			return nil, "", errors.New("agent.forget scope invalid")
		}
		arguments = input
	default:
		return nil, "", errors.New("agent upstream returned unsupported tool call")
	}
	raw, err := json.Marshal(arguments)
	if err != nil {
		return nil, "", err
	}
	return arguments, string(raw), nil
}

func decodeAgentJSONValue(value string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("trailing JSON value")
	}
	return nil
}

func validateAgentToolSuccess(name string, arguments any, request *SubmitAgentToolResultRequest) error {
	if name != "image.inspect" && name != "video.inspect" && request.Inspection != nil {
		return safeMessageError{message: "工具结果包含多余的视觉验收数据"}
	}
	if name != "agent.remember" && name != "agent.forget" && request.Memory != nil {
		return safeMessageError{message: "工具结果包含多余的记忆数据"}
	}
	switch name {
	case "canvas.plan":
		input := arguments.(canvasPlanArguments)
		if request.Plan == nil || request.Video != nil || len(request.Images) != 0 || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" {
			return safeMessageError{message: "执行计划工具结果无效"}
		}
		request.Plan.Summary = strings.TrimSpace(request.Plan.Summary)
		for i := range request.Plan.Steps {
			request.Plan.Steps[i] = strings.TrimSpace(request.Plan.Steps[i])
		}
		if request.Plan.Summary != input.Summary || !sameStringSlice(request.Plan.Steps, input.Steps) {
			return safeMessageError{message: "执行计划工具结果无效"}
		}
	case "image.generate":
		input := arguments.(imageGenerateArguments)
		if request.Plan != nil || request.Video != nil || len(request.Images) < 1 || len(request.Images) > input.Count || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" {
			return safeMessageError{message: "图片生成工具结果无效"}
		}
		for i := range request.Images {
			request.Images[i].NodeID, request.Images[i].StorageKey = strings.TrimSpace(request.Images[i].NodeID), strings.TrimSpace(request.Images[i].StorageKey)
			if request.Images[i].NodeID == "" || request.Images[i].StorageKey == "" {
				return safeMessageError{message: "工具结果图片无效"}
			}
		}
	case "image.edit":
		input := arguments.(imageEditArguments)
		if request.Plan != nil || request.Video != nil || len(request.Images) < 1 || len(request.Images) > input.Count || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" {
			return safeMessageError{message: "图片编辑工具结果无效"}
		}
		for i := range request.Images {
			request.Images[i].NodeID, request.Images[i].StorageKey = strings.TrimSpace(request.Images[i].NodeID), strings.TrimSpace(request.Images[i].StorageKey)
			if request.Images[i].NodeID == "" || request.Images[i].StorageKey == "" {
				return safeMessageError{message: "工具结果图片无效"}
			}
		}
	case "image.inspect", "video.inspect":
		if request.Plan != nil || request.Video != nil || len(request.Images) != 0 || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" || request.Inspection == nil || request.Memory != nil {
			return safeMessageError{message: "媒体视觉验收结果无效"}
		}
		inspection := request.Inspection
		inspection.Status, inspection.Summary, inspection.RevisedPrompt = strings.TrimSpace(inspection.Status), strings.TrimSpace(inspection.Summary), strings.TrimSpace(inspection.RevisedPrompt)
		if inspection.Issues == nil {
			inspection.Issues = []string{}
		}
		if (inspection.Status != "passed" && inspection.Status != "needs_revision" && inspection.Status != "unavailable") || inspection.Summary == "" || utf8.RuneCountInString(inspection.Summary) > 1000 || len(inspection.Issues) > 6 || utf8.RuneCountInString(inspection.RevisedPrompt) > 8000 {
			return safeMessageError{message: "媒体视觉验收结果无效"}
		}
		for i := range inspection.Issues {
			inspection.Issues[i] = strings.TrimSpace(inspection.Issues[i])
			if inspection.Issues[i] == "" || utf8.RuneCountInString(inspection.Issues[i]) > 300 {
				return safeMessageError{message: "媒体视觉验收问题无效"}
			}
		}
		if inspection.Status == "needs_revision" && (len(inspection.Issues) == 0 || inspection.RevisedPrompt == "") {
			return safeMessageError{message: "媒体视觉验收调整建议缺失"}
		}
		if inspection.Status != "needs_revision" && len(inspection.Issues) != 0 {
			return safeMessageError{message: "媒体视觉验收问题无效"}
		}
		if inspection.Status != "needs_revision" && inspection.RevisedPrompt != "" {
			return safeMessageError{message: "媒体视觉验收调整建议无效"}
		}
	case "video.generate":
		if request.Plan != nil || request.Video == nil || len(request.Images) != 0 || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" {
			return safeMessageError{message: "视频生成工具结果无效"}
		}
		request.Video.NodeID, request.Video.StorageKey = strings.TrimSpace(request.Video.NodeID), strings.TrimSpace(request.Video.StorageKey)
		if request.Video.NodeID == "" || request.Video.StorageKey == "" {
			return safeMessageError{message: "工具结果视频无效"}
		}
	case "canvas.arrange":
		input := arguments.(canvasArrangeArguments)
		if request.Plan != nil || request.Video != nil || len(request.Images) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" || !sameTrimmedStringSet(request.NodeIDs, input.NodeIDs) || len(request.Positions) != len(input.NodeIDs) {
			return safeMessageError{message: "画布排列工具结果无效"}
		}
		expected, seen := stringSet(input.NodeIDs), make(map[string]struct{}, len(request.Positions))
		for i := range request.Positions {
			position := &request.Positions[i]
			position.NodeID = strings.TrimSpace(position.NodeID)
			if _, exists := expected[position.NodeID]; !exists || !finite(position.X) || !finite(position.Y) {
				return safeMessageError{message: "画布排列位置无效"}
			}
			if _, exists := seen[position.NodeID]; exists {
				return safeMessageError{message: "画布排列位置重复"}
			}
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
	case "agent.remember":
		input := arguments.(agentRememberArguments)
		if request.Plan != nil || request.Video != nil || len(request.Images) != 0 || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" || request.Memory == nil || request.Memory.Key != input.Key || request.Memory.Scope != input.Scope || request.Memory.Status != "active" {
			return safeMessageError{message: "记忆保存工具结果无效"}
		}
	case "agent.forget":
		input := arguments.(agentForgetArguments)
		if request.Plan != nil || request.Video != nil || len(request.Images) != 0 || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" || request.Memory == nil || request.Memory.Key != input.Key || request.Memory.Scope != input.Scope || request.Memory.Status != "forgotten" {
			return safeMessageError{message: "记忆遗忘工具结果无效"}
		}
	default:
		return safeMessageError{message: "工具名称无效"}
	}
	return nil
}

func agentToolResultOutput(name string, request SubmitAgentToolResultRequest) any {
	if request.Status == "failed" {
		return struct {
			Error string `json:"error"`
		}{request.Error}
	}
	switch name {
	case "canvas.plan":
		return struct {
			Plan *AgentToolPlan `json:"plan"`
		}{request.Plan}
	case "image.generate", "image.edit":
		return struct {
			Images []AgentToolImage `json:"images"`
		}{request.Images}
	case "image.inspect", "video.inspect":
		return struct {
			Inspection *AgentToolInspection `json:"inspection"`
		}{request.Inspection}
	case "video.generate":
		return struct {
			Video *AgentToolVideo `json:"video"`
		}{request.Video}
	case "canvas.arrange":
		return struct {
			NodeIDs   []string            `json:"nodeIds"`
			Positions []AgentToolPosition `json:"positions"`
		}{request.NodeIDs, request.Positions}
	case "canvas.delete":
		return struct {
			NodeIDs []string `json:"nodeIds"`
		}{request.NodeIDs}
	case "canvas.update_text":
		return struct {
			NodeID string `json:"nodeId"`
			Text   string `json:"text"`
		}{request.NodeID, request.Text}
	case "agent.remember", "agent.forget":
		return struct {
			Memory *AgentToolMemory `json:"memory"`
		}{request.Memory}
	default:
		return struct {
			NodeID    string `json:"nodeId"`
			Placement string `json:"placement"`
		}{request.NodeID, request.Placement}
	}
}

func canonicalAgentToolName(name string) string {
	if definition, ok := agentToolDefinitionFor(name); ok {
		return definition.Name
	}
	return name
}

func persistAgentMemoryToolResult(run model.AgentRun, name string, arguments any, result *AgentToolMemory) error {
	if result == nil {
		return safeMessageError{message: "记忆工具结果缺失"}
	}
	session, err := repository.GetAgentSession(run.OrganizationID, run.UserID, run.SessionID)
	if err != nil {
		return err
	}
	timestamp := now()
	projectID := session.ProjectID
	if name == "agent.remember" {
		input := arguments.(agentRememberArguments)
		if input.Scope == "user" {
			projectID = ""
		}
		expiresAt := ""
		if input.ExpiresInDays > 0 {
			expiresAt = time.Now().UTC().Add(time.Duration(input.ExpiresInDays) * 24 * time.Hour).Format(timestampLayout)
		}
		_, err = repository.SaveAgentMemory(model.AgentMemory{ID: newID("agent-memory"), OrganizationID: run.OrganizationID, UserID: run.UserID, ProjectID: projectID, Kind: model.AgentMemoryKind(input.Kind), Key: input.Key, Content: input.Content, SourceRunID: run.ID, SourceMessageID: run.MessageID, Confidence: input.Confidence, Status: model.AgentMemoryStatusActive, ExpiresAt: expiresAt, CreatedAt: timestamp, UpdatedAt: timestamp})
		return err
	}
	input := arguments.(agentForgetArguments)
	if input.Scope == "user" {
		projectID = ""
	}
	_, err = repository.ForgetAgentMemory(run.OrganizationID, run.UserID, projectID, input.Key, timestamp)
	return err
}

func sensitiveAgentMemoryKey(key string) bool {
	lower := strings.ToLower(key)
	for _, term := range []string{"password", "passwd", "token", "secret", "api_key", "apikey", "access_key", "private_key", "密钥", "密码", "令牌"} {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func agentToolSchemas() []any {
	return []any{
		map[string]any{"type": "function", "function": map[string]any{"name": "canvas_plan", "description": "当任务需要两个及以上工具时，先展示执行计划；每个步骤按顺序对应后续一个真实工具及其成功条件，失败后可提交剩余步骤的新计划", "parameters": map[string]any{"type": "object", "properties": map[string]any{"summary": map[string]any{"type": "string", "maxLength": 120}, "steps": map[string]any{"type": "array", "minItems": 2, "maxItems": 7, "items": map[string]any{"type": "string", "maxLength": 80}}}, "required": []string{"summary", "steps"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "image_generate", "description": "根据提示词生成一至四张图片，可通过 referenceNodeIds 引用画布中的图片节点作为参考", "parameters": map[string]any{"type": "object", "properties": map[string]any{"prompt": map[string]any{"type": "string", "maxLength": 8000}, "count": map[string]any{"type": "integer", "minimum": 1, "maximum": 4, "default": 1}, "referenceNodeIds": map[string]any{"type": "array", "maxItems": 6, "uniqueItems": true, "items": map[string]any{"type": "string"}}}, "required": []string{"prompt"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "image_edit", "description": "以画布中一个图片节点为源图进行编辑修改，生成一至四张新图片并自动连线；用户要求修改、替换、调整已有图片时优先使用，而不是重新生成", "parameters": map[string]any{"type": "object", "properties": map[string]any{"nodeId": map[string]any{"type": "string"}, "prompt": map[string]any{"type": "string", "maxLength": 8000}, "count": map[string]any{"type": "integer", "minimum": 1, "maximum": 4, "default": 1}}, "required": []string{"nodeId", "prompt"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "image_inspect", "description": "使用视觉模型对照用户目标验收一至四个图片节点；图片生成后、总结完成前必须调用", "parameters": map[string]any{"type": "object", "properties": map[string]any{"nodeIds": map[string]any{"type": "array", "minItems": 1, "maxItems": 4, "uniqueItems": true, "items": map[string]any{"type": "string"}}, "criteria": map[string]any{"type": "string", "maxLength": 2000}}, "required": []string{"nodeIds", "criteria"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "video_generate", "description": "生成一个视频，可使用画布中相关图片作为参考", "parameters": map[string]any{"type": "object", "properties": map[string]any{"prompt": map[string]any{"type": "string", "maxLength": 8000}, "duration": map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "default": 6}, "imageNodeId": map[string]any{"type": "string"}}, "required": []string{"prompt"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "video_inspect", "description": "按时间顺序抽取关键帧并使用视觉模型对照用户目标验收一个视频节点；视频生成后、总结完成前必须调用", "parameters": map[string]any{"type": "object", "properties": map[string]any{"nodeId": map[string]any{"type": "string"}, "criteria": map[string]any{"type": "string", "maxLength": 2000}}, "required": []string{"nodeId", "criteria"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "canvas_arrange", "description": "重排画布中相关节点", "parameters": map[string]any{"type": "object", "properties": map[string]any{"nodeIds": map[string]any{"type": "array", "minItems": 2, "maxItems": 20, "uniqueItems": true, "items": map[string]any{"type": "string"}}, "mode": map[string]any{"type": "string", "enum": []string{"horizontal", "vertical", "grid"}}, "gap": map[string]any{"type": "integer", "minimum": 16, "maximum": 400, "default": 40}}, "required": []string{"nodeIds", "mode"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "canvas_add_text", "description": "在画布中插入文本节点，可通过 sourceNodeIds 关联画布相关来源节点", "parameters": map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string", "maxLength": 4000}, "placement": map[string]any{"type": "string", "enum": []string{"center", "right_of_selection"}, "default": "center"}, "sourceNodeIds": map[string]any{"type": "array", "maxItems": 20, "uniqueItems": true, "items": map[string]any{"type": "string"}}}, "required": []string{"text"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "canvas_delete", "description": "删除画布中相关且当前仍选中的节点，执行前需要用户确认", "parameters": map[string]any{"type": "object", "properties": map[string]any{"nodeIds": map[string]any{"type": "array", "minItems": 1, "maxItems": 20, "uniqueItems": true, "items": map[string]any{"type": "string"}}}, "required": []string{"nodeIds"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "canvas_update_text", "description": "修改画布中相关且当前仍选中的文本节点，执行前需要用户确认", "parameters": map[string]any{"type": "object", "properties": map[string]any{"nodeId": map[string]any{"type": "string"}, "text": map[string]any{"type": "string", "maxLength": 4000}}, "required": []string{"nodeId", "text"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "agent_ask_user", "description": "需求缺少关键信息时向用户澄清，执行前等待用户回答", "parameters": map[string]any{"type": "object", "properties": map[string]any{"question": map[string]any{"type": "string", "maxLength": 500}, "options": map[string]any{"type": "array", "minItems": 3, "maxItems": 4, "items": map[string]any{"type": "string", "maxLength": 120}}}, "required": []string{"question", "options"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "agent_remember", "description": "在用户明确要求长期记住项目偏好、事实或约束时保存一条可审计记忆；执行前需要用户确认，不要保存敏感信息", "parameters": map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"preference", "fact", "constraint", "experience"}}, "key": map[string]any{"type": "string", "maxLength": 120}, "content": map[string]any{"type": "string", "maxLength": 1000}, "scope": map[string]any{"type": "string", "enum": []string{"project", "user"}, "default": "project"}, "confidence": map[string]any{"type": "number", "minimum": 0.5, "maximum": 1, "default": 0.8}, "expiresInDays": map[string]any{"type": "integer", "minimum": 0, "maximum": 3650, "default": 0}}, "required": []string{"kind", "key", "content"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "agent_forget", "description": "删除一条已保存的长期记忆；执行前需要用户确认", "parameters": map[string]any{"type": "object", "properties": map[string]any{"key": map[string]any{"type": "string", "maxLength": 120}, "scope": map[string]any{"type": "string", "enum": []string{"project", "user"}, "default": "project"}}, "required": []string{"key"}, "additionalProperties": false}}},
	}
}

func sameTrimmedStringSet(values, expected []string) bool {
	if len(values) != len(expected) {
		return false
	}
	expectedSet, seen := stringSet(expected), make(map[string]struct{}, len(values))
	for i, value := range values {
		value = strings.TrimSpace(value)
		if _, exists := expectedSet[value]; !exists {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value], values[i] = struct{}{}, value
	}
	return true
}

func sameStringSlice(values, expected []string) bool {
	if len(values) != len(expected) {
		return false
	}
	for i := range values {
		if values[i] != expected[i] {
			return false
		}
	}
	return true
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
