package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/disintegration/imaging"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

const (
	agentApplyOpsDuplicateMessage = "这些节点和连线已经在画布上了，没有重复添加。"
	agentAutonomyCautious         = "cautious"
	agentAutonomyStandard         = "standard"
	agentAutonomyAutonomous       = "autonomous"
	// agentWaitingToolTimeout 是需要画布页面执行的工具最多等页面多久；页面不在线时任务保留，回到画布后接着执行。
	agentWaitingToolTimeout      = 24 * time.Hour
	agentWaitingToolTimeoutError = "工具结果等待超时"
	agentToolExecutionLease      = 90 * time.Second
	agentRunningTimeout          = 6 * time.Minute
	agentRunningTimeoutError     = "助手运行恢复超时"
)

var (
	agentUserFacingNodeIDPattern = regexp.MustCompile(`(?i)[（(]?\s*(?:id\s*[：:=]\s*)?(?:text|config|image|video|audio|node)-[A-Za-z0-9_-]+\s*[)）]?`)
	agentUserFacingGoalPrefix    = regexp.MustCompile(`(?m)^\s*\*{0,2}目标完成\*{0,2}\s*[：:]\s*`)
	agentEmptyParenPattern       = regexp.MustCompile(`[（(]\s*[)）]`)
	agentMultiSpacePattern       = regexp.MustCompile(`(\S)[ \t]{2,}`)
)

type CreateAgentSessionRequest struct {
	SessionID string `json:"sessionId"`
	ProjectID string `json:"projectId"`
	Title     string `json:"title"`
	Profile   any    `json:"profile"`
}

type AgentCanvasNode struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Title      string             `json:"title"`
	X          float64            `json:"x"`
	Y          float64            `json:"y"`
	Width      float64            `json:"width"`
	Height     float64            `json:"height"`
	Content    string             `json:"content,omitempty"`
	Prompt     string             `json:"prompt,omitempty"`
	References []string           `json:"references,omitempty"`
	StorageKey string             `json:"storageKey,omitempty"`
	Settings   *AgentMediaOptions `json:"settings,omitempty"`
	// KeyframeIDs 是视频配置节点里被指定为关键帧的上游图片节点。
	KeyframeIDs []string `json:"keyframeIds,omitempty"`
	Status      string   `json:"status,omitempty"`
}

type AgentCanvasConnection struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// AgentCanvasViewport 是当前视口在画布坐标系中的可见范围。
type AgentCanvasViewport struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Zoom   float64 `json:"zoom,omitempty"`
}

type AgentCanvasContext struct {
	Autonomy        string                  `json:"autonomy"`
	Viewport        *AgentCanvasViewport    `json:"viewport,omitempty"`
	SelectedNodeIDs []string                `json:"selectedNodeIds"`
	FocusNodeIDs    []string                `json:"focusNodeIds,omitempty"`
	VisibleNodeIDs  []string                `json:"visibleNodeIds,omitempty"`
	Nodes           []AgentCanvasNode       `json:"nodes"`
	Connections     []AgentCanvasConnection `json:"connections"`
	Defaults        map[string]string       `json:"defaults,omitempty"`
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

// AgentToolPlan 是模型维护的工作计划；canvas.plan 每次调用都提交完整的最新状态。
type AgentToolPlan struct {
	Explanation string          `json:"explanation,omitempty"`
	Steps       []AgentPlanItem `json:"steps"`
}

type AgentPlanItem struct {
	Step   string `json:"step"`
	Status string `json:"status"`
}

type AgentToolInspection struct {
	Status        string   `json:"status"`
	Summary       string   `json:"summary"`
	Issues        []string `json:"issues"`
	RevisedPrompt string   `json:"revisedPrompt,omitempty"`
}

type SubmitAgentToolResultRequest struct {
	CanvasContext  *AgentCanvasContext  `json:"canvasContext,omitempty"`
	Observation    *AgentCanvasContext  `json:"observation,omitempty"`
	CallID         string               `json:"callId"`
	ExecutionToken string               `json:"executionToken,omitempty"`
	Status         string               `json:"status"`
	Error          string               `json:"error,omitempty"`
	Images         []AgentToolImage     `json:"images,omitempty"`
	Video          *AgentToolVideo      `json:"video,omitempty"`
	NodeIDs        []string             `json:"nodeIds,omitempty"`
	Positions      []AgentToolPosition  `json:"positions,omitempty"`
	NodeID         string               `json:"nodeId,omitempty"`
	FromNodeID     string               `json:"fromNodeId,omitempty"`
	ToNodeID       string               `json:"toNodeId,omitempty"`
	Text           string               `json:"text,omitempty"`
	Placement      string               `json:"placement,omitempty"`
	Plan           *AgentToolPlan       `json:"plan,omitempty"`
	Skill          *AgentToolSkill      `json:"skill,omitempty"`
	Inspection     *AgentToolInspection `json:"inspection,omitempty"`
	Memory         *AgentToolMemory     `json:"memory,omitempty"`
	Ops            []AgentToolOpResult  `json:"ops,omitempty"`
}

type ConfirmAgentToolRequest struct {
	CanvasContext *AgentCanvasContext `json:"canvasContext,omitempty"`
	CallID        string              `json:"callId"`
	Decision      string              `json:"decision"`
	Answer        string              `json:"answer,omitempty"`
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

type AgentMediaOptions struct {
	Model         string `json:"model,omitempty"`
	Size          string `json:"size,omitempty"`
	Quality       string `json:"quality,omitempty"`
	Resolution    string `json:"resolution,omitempty"`
	GenerateAudio *bool  `json:"generateAudio,omitempty"`
	ConfigNodeID  string `json:"configNodeId,omitempty"`
	Mode          string `json:"mode,omitempty"`
	Count         int    `json:"count,omitempty"`
	Duration      int    `json:"duration,omitempty"`
}

type canvasReadArguments struct {
	NodeIDs []string `json:"nodeIds,omitempty"`
	Query   string   `json:"query,omitempty"`
	Offset  int      `json:"offset,omitempty"`
}

type imageGenerateArguments struct {
	AgentMediaOptions
	Prompt           string   `json:"prompt"`
	Count            int      `json:"count"`
	ReferenceNodeIDs []string `json:"referenceNodeIds,omitempty"`
}

type imageEditArguments struct {
	AgentMediaOptions
	NodeID string `json:"nodeId"`
	Prompt string `json:"prompt"`
	Count  int    `json:"count"`
}

type videoGenerateArguments struct {
	AgentMediaOptions
	Prompt       string   `json:"prompt"`
	Duration     int      `json:"duration"`
	ImageNodeID  string   `json:"imageNodeId,omitempty"`
	ImageNodeIDs []string `json:"imageNodeIds,omitempty"`
	VideoNodeIDs []string `json:"videoNodeIds,omitempty"`
	AudioNodeIDs []string `json:"audioNodeIds,omitempty"`
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

type canvasAddConfigArguments struct {
	AgentMediaOptions
	Prompt        string   `json:"prompt,omitempty"`
	Placement     string   `json:"placement"`
	SourceNodeIDs []string `json:"sourceNodeIds,omitempty"`
}

type canvasConnectArguments struct {
	FromNodeID string `json:"fromNodeId"`
	ToNodeID   string `json:"toNodeId"`
}

type canvasDeleteArguments struct {
	NodeIDs []string `json:"nodeIds"`
}

type canvasUpdateTextArguments struct {
	NodeID string `json:"nodeId"`
	Text   string `json:"text"`
}

type canvasPlanArguments = AgentToolPlan

type askUserOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type askUserArguments struct {
	Header   string          `json:"header,omitempty"`
	Question string          `json:"question"`
	Options  []askUserOption `json:"options"`
}

type skillReadArguments struct {
	SkillID string `json:"skillId"`
}

type agentNodeAuthorization struct {
	NodeIDs       map[string]struct{}
	ImageNodeIDs  map[string]struct{}
	VideoNodeIDs  map[string]struct{}
	AudioNodeIDs  map[string]struct{}
	TextNodeIDs   map[string]struct{}
	ConfigNodeIDs map[string]struct{}
}

type agentToolCall struct {
	ID        string
	Name      string
	Arguments any
	Raw       string
}

type agentCompletion struct {
	Content   string
	Reasoning string
	ToolCall  *agentToolCall
	// ToolCalls 是本轮并行下发的全部调用，第一个与 ToolCall 相同；为空时只看 ToolCall。
	ToolCalls []*agentToolCall
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
	resolvedModel, err := resolveAgentModel(request.Model)
	if err != nil {
		return AgentRunSubmission{}, err
	}
	request.Model = resolvedModel
	canvasContext, err := normalizeAgentCanvasContext(request.CanvasContext)
	if err != nil {
		return AgentRunSubmission{}, err
	}
	contextJSON, err := json.Marshal(canvasContext)
	if err != nil || len(contextJSON) > 16<<20 {
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
	// 发送时引用的节点记在事件里：运行过程中画布上下文会被刷新，回放对话时不能再从上下文里取。
	startedPayload, _ := json.Marshal(map[string]any{"referenceNodeIds": canvasContext.SelectedNodeIDs})
	event := model.AgentEvent{ID: newID("agent-event"), OrganizationID: user.OrganizationID, UserID: user.ID, RunID: runID, Type: model.AgentEventRunStarted, Payload: string(startedPayload), CreatedAt: timestamp}
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
		if len(request.Images) != 0 || request.Video != nil || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" || request.Plan != nil || request.Skill != nil || request.Inspection != nil || request.Memory != nil || request.Observation != nil || len(request.Ops) != 0 {
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
	if toolStep.Status != model.AgentStepStatusRunning && toolStep.Output != "" {
		var saved SubmitAgentToolResultRequest
		if json.Unmarshal([]byte(toolStep.Output), &saved) == nil {
			request.CanvasContext, request.ExecutionToken = nil, ""
			if toolStep.ToolName == "canvas.read" {
				request.Observation = saved.Observation
			}
			encoded, _ := json.Marshal(request)
			if string(encoded) == toolStep.Output {
				return run, nil
			}
		}
		return model.AgentRun{}, safeMessageError{message: "工具结果与已保存回执不一致"}
	}
	if toolStep.ToolName == "canvas.read" && request.Status == "success" && request.CanvasContext == nil {
		return model.AgentRun{}, safeMessageError{message: "读取画布必须提交当前画布状态"}
	}
	authorization, err := agentRunNodeAuthorization(run)
	if err != nil {
		return model.AgentRun{}, err
	}
	toolArguments, _, err := decodeAgentToolArguments(toolStep.ToolName, toolStep.Input, authorization)
	if err != nil {
		return model.AgentRun{}, err
	}
	commit := repository.AgentToolCommit{}
	if request.CanvasContext != nil {
		context, err := normalizeAgentCanvasContext(*request.CanvasContext)
		if err != nil {
			return model.AgentRun{}, err
		}
		context.Autonomy = agentRunAutonomy(run)
		payload, err := json.Marshal(context)
		if err != nil || len(payload) > 16<<20 {
			return model.AgentRun{}, safeMessageError{message: "画布上下文过大"}
		}
		commit.Context = string(payload)
		if toolStep.ToolName == "canvas.read" && request.Status == "success" {
			observation := agentCanvasObservation(context, toolArguments.(canvasReadArguments))
			request.Observation = &observation
		}
		request.CanvasContext = nil
	}
	if request.Status == "success" {
		if err := validateAgentToolSuccess(toolStep.ToolName, toolArguments, &request); err != nil {
			return model.AgentRun{}, err
		}
		if toolStep.ToolName == "agent.remember" || toolStep.ToolName == "agent.forget" {
			memory, err := prepareAgentMemoryToolResult(run, toolStep.ToolName, toolArguments)
			if err != nil {
				return model.AgentRun{}, err
			}
			commit.Memory = &memory
		}
	}
	executionToken := request.ExecutionToken
	request.ExecutionToken = ""
	output, err := json.Marshal(request)
	if err != nil {
		return model.AgentRun{}, err
	}
	if len(output) > 512<<10 {
		return model.AgentRun{}, safeMessageError{message: "工具结果过大"}
	}
	timestamp := now()
	completedPayload, _ := json.Marshal(map[string]any{"callId": request.CallID, "name": toolStep.ToolName, "status": request.Status, "output": agentToolResultOutput(toolStep.ToolName, request)})
	failedPayload, _ := json.Marshal(map[string]string{"error": request.Error})
	completionStep := model.AgentStep{ID: newID("agent-step"), OrganizationID: user.OrganizationID, UserID: user.ID, RunID: runID, Type: model.AgentStepTypeCompletion, Status: model.AgentStepStatusRunning, Input: string(output), StartedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp}
	continueAfterFailure := false
	if request.Status == "failed" && retryableAgentTool(toolStep.ToolName) {
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
		model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventRunFailed, Payload: string(failedPayload), CreatedAt: timestamp}, completionStep, commit,
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
		updatedContext := ""
		if request.CanvasContext != nil {
			canvas, err := normalizeAgentCanvasContext(*request.CanvasContext)
			if err != nil {
				return model.AgentRun{}, err
			}
			canvas.Autonomy = agentRunAutonomy(run)
			encoded, err := json.Marshal(canvas)
			if err != nil || len(encoded) > 16<<20 {
				return model.AgentRun{}, safeMessageError{message: "画布上下文过大"}
			}
			updatedContext = string(encoded)
		}
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
			model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolCompleted, Payload: string(completedPayload), CreatedAt: timestamp}, completionStep, updatedContext,
		)
		if errors.Is(err, repository.ErrAgentToolResultConflict) {
			return model.AgentRun{}, safeMessageError{message: "回答与已保存结果不一致"}
		}
		if err != nil {
			return model.AgentRun{}, err
		}
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

// SubscribeAgentRunEvents 订阅运行的内存事件通知，调用方负责 Close。
func SubscribeAgentRunEvents(runID string) repository.AgentRunSubscription {
	return repository.SubscribeAgentRun(strings.TrimSpace(runID))
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
	// 计划每次更新都会写入一版新修订，诊断只展示最新一版。
	for i := range plan {
		if plan[i].Revision == plan[len(plan)-1].Revision {
			plan = plan[i:]
			break
		}
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
	if reason := agentRunBudgetReason(run, mustListAgentToolSteps(run), step.ToolName); reason != "" {
		return AgentStepRetryResult{}, safeMessageError{message: agentBudgetMessage(reason)}
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
	meta := map[string]any{"retryOf": sourceCallID}
	if agentBackgroundTool(step.ToolName) {
		for key, value := range agentBackgroundToolMeta(run, step.ToolName, callID, arguments) {
			meta[key] = value
		}
	}
	payload, _ := json.Marshal(map[string]any{"callId": callID, "name": step.ToolName, "arguments": arguments, "meta": meta})
	retryStep := model.AgentStep{ID: newID("agent-step"), OrganizationID: user.OrganizationID, UserID: user.ID, RunID: runID, Type: model.AgentStepTypeTool, Status: model.AgentStepStatusRunning, ToolCallID: callID, ToolName: step.ToolName, PresentToken: agentToolPresentation(step.ToolName), Input: raw, StartedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp}
	run, err = repository.RetryAgentStep(user.OrganizationID, user.ID, runID, sourceCallID, timestamp, retryStep, model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolCall, Payload: string(payload), CreatedAt: timestamp})
	if errors.Is(err, repository.ErrAgentStepNotRetryable) {
		return AgentStepRetryResult{}, safeMessageError{message: "该失败步骤已被处理或当前不能重试"}
	}
	if err != nil {
		return AgentStepRetryResult{}, err
	}
	time.AfterFunc(agentWaitingToolTimeout, func() { _ = expireWaitingAgentRun(user.OrganizationID, user.ID, runID) })
	if agentBackgroundTool(step.ToolName) {
		startAgentBackgroundTool(user.OrganizationID, user.ID, runID, callID)
	}
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

func RevertAgentRun(user model.AuthUser, runID string, appliedCallIDs ...[]string) (AgentRunRevertResult, error) {
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
	localApplied := map[string]struct{}{}
	if len(appliedCallIDs) > 0 {
		if len(appliedCallIDs[0]) > 32 {
			return AgentRunRevertResult{}, safeMessageError{message: "撤销工具数量过多"}
		}
		localApplied = stringSet(appliedCallIDs[0])
	}
	for _, step := range steps {
		_, applied := localApplied[step.ToolCallID]
		if !revertibleAgentTool(step.ToolName) || (!agentStepSucceeded(step) && (!applied || step.Confirmation == "rejected")) {
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
	ctx, cancel := context.WithTimeout(context.Background(), agentCompletionDeadline)
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
			completion.ToolCalls = nil
			completion.Content = agentBudgetMessage(reason)
		}
	}
	if completion.ToolCall != nil {
		dispatchAgentToolCalls(run, userGroup, completion)
		return
	}
	payload, _ := json.Marshal(agentMessagePayload(completion.Content, completion.Reasoning))
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
	var safe safeMessageError
	if errors.As(err, &safe) {
		message = safe.message
	} else if errors.Is(err, context.DeadlineExceeded) {
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
	if content, ok := agentNodeCountReply(run, messages); ok {
		return agentCompletion{Content: content}, nil
	}
	pricingRequest := PricingRequest{Model: run.Model, Modality: "text", Operation: "completion", Unit: "request", Quantity: 1}
	selection, err := SelectModelChannel(pricingRequest)
	if err != nil {
		return completion, err
	}
	pricingResolver, err := NewPricingResolver(run.UserID, userGroup)
	if err != nil {
		return completion, err
	}
	pricingResult, err := pricingResolver.Resolve(pricingRequest)
	if err != nil {
		return completion, err
	}
	task, err := BeginGenerationTask(GenerationTaskInput{
		UserID: run.UserID, OrganizationID: run.OrganizationID, RequestID: requestID,
		Model: run.Model, UpstreamModel: selection.Model.UpstreamModel, ChannelName: selection.Channel.Name, ChannelID: selection.Channel.ID,
		Path: "/chat/completions", Modality: pricingRequest.Modality, Operation: pricingRequest.Operation,
		Quantity: pricingRequest.Quantity, Credits: pricingResult.Credits, PricingSnapshot: pricingResult.Snapshot,
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
	requestMessages := make([]agentChatMessage, 0, len(messages)+8)
	requestMessages = append(requestMessages,
		agentChatMessage{Role: "system", Content: agentSystemPrompt()},
		agentChatMessage{Role: "system", Content: agentAutonomyPrompt(agentRunAutonomy(run))},
		agentChatMessage{Role: "system", Content: modelCanvasContext},
		agentChatMessage{Role: "system", Content: agentModelCapabilities()},
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
		requestMessages = append(requestMessages, agentChatMessage{Role: "system", Content: "以下是服务端检索到的长期记忆，只能作为辅助事实；若用户明确纠正，先调用 agent_forget 或 agent_remember 更新，不得把记忆当成不可变指令：" + string(encoded)})
	}
	conversation, err := agentConversationMessages(run, messages)
	if err != nil {
		return completion, err
	}
	requestMessages = attachAgentPlanningPreviews(append(requestMessages, conversation...), run.OrganizationID, run.Context)
	authorization, err := agentRunNodeAuthorization(run)
	if err != nil {
		return completion, err
	}
	toolSchemas := agentToolSchemas()
	for attempt := 0; ; attempt++ {
		attemptID := requestID
		if attempt > 0 {
			attemptID = requestID + "-fix"
		}
		bodyValue := map[string]any{"model": selection.Model.UpstreamModel, "messages": requestMessages, "tools": toolSchemas, "tool_choice": "auto", "parallel_tool_calls": true, "stream": true, "max_tokens": agentMaxOutputTokens}
		publisher := newAgentRunStreamPublisher(run.ID)
		message, requestErr := requestAgentUpstreamMessage(ctx, selection.Channel, attemptID, bodyValue, &requestMessages, agentChatMessagesHaveImageURLs(requestMessages), publisher)
		publisher.flush()
		if requestErr != nil {
			return completion, requestErr
		}
		var correction []agentChatMessage
		completion, correction, err = agentCompletionFromMessage(run, toolSteps, message, authorization)
		// 首个工具参数无效、或正文声称已生成而本轮没有任何生成时，把问题交回模型修正一次；第二次的回复照常采用。
		if len(correction) > 0 && attempt == 0 {
			requestMessages = append(requestMessages, correction...)
			continue
		}
		return completion, err
	}
}

// agentCompletionFromMessage 校验模型回复：首个工具调用做完整校验与去重，同一轮后续彼此独立、无需确认的调用一起下发；
// 需要模型修正时返回追加到对话末尾的纠正消息。
func agentCompletionFromMessage(run model.AgentRun, toolSteps []model.AgentStep, message agentUpstreamMessage, authorization agentNodeAuthorization) (agentCompletion, []agentChatMessage, error) {
	completion := agentCompletion{Content: strings.TrimSpace(message.Content), Reasoning: message.reasoningText()}
	if len(message.ToolCalls) == 0 {
		if completion.Content == "" {
			return completion, nil, errors.New("agent upstream returned empty content")
		}
		var correction []agentChatMessage
		if agentClaimedMediaWithoutTools(completion.Content) && !agentRunGeneratedMedia(toolSteps) {
			correction = []agentChatMessage{
				{Role: "assistant", Content: completion.Content},
				{Role: "system", Content: "上一条回复提到已经生成了内容，但本轮没有调用过生成工具，画布上没有新增的图片或视频。如果用户要求生成，请调用相应工具；如果只是在描述画布上已有的内容，请重新回答，并说清楚那是之前就有的内容。"},
			}
		}
		completion.Content = sanitizeAgentUserFacingText(completion.Content)
		return completion, correction, nil
	}
	// 真实工具调用与伪造的调用文本同时出现时，以真实调用为准并去掉伪造文本。
	completion.Content = stripAgentFakeToolCallText(completion.Content)
	call := message.ToolCalls[0]
	if call.Type != "function" {
		return completion, nil, errors.New("agent upstream returned unsupported tool call")
	}
	toolName := canonicalAgentToolName(call.Function.Name)
	if toolName == "canvas.apply_ops" {
		deduped, skipped, empty := dedupeAgentApplyOps(call.Function.Arguments, toolSteps)
		if empty {
			completion.Content = agentApplyOpsDuplicateMessage
			return completion, nil, nil
		}
		if skipped > 0 {
			call.Function.Arguments = deduped
		}
	}
	arguments, raw, decodeErr := decodeAgentToolArguments(toolName, agentMediaArgumentsWithDefaults(toolName, call.Function.Arguments, run.Context), authorization)
	if decodeErr != nil {
		callID := strings.TrimSpace(call.ID)
		if callID == "" {
			callID = agentTranscriptCallID(newID("agent-tool-call"))
		}
		var content any
		if completion.Content != "" {
			content = completion.Content
		}
		// 参数被截断等情况下不是合法 JSON，原样回放会被部分上游拒绝。
		rawArguments := call.Function.Arguments
		if !json.Valid([]byte(rawArguments)) {
			rawArguments = "{}"
		}
		output, _ := json.Marshal(map[string]string{"status": "failed", "error": "参数无效：" + decodeErr.Error() + "。请按工具定义修正参数后重新调用，不要原样重试。"})
		return completion, []agentChatMessage{
			{Role: "assistant", Content: content, ToolCalls: []agentChatToolCall{{ID: callID, Type: "function", Function: agentChatToolFunction{Name: call.Function.Name, Arguments: rawArguments}}}},
			{Role: "tool", ToolCallID: callID, Content: string(output)},
		}, decodeErr
	}
	if agentMediaRevisionLimitReached(run, toolSteps, toolName) {
		completion.Content = agentMediaRevisionLimitMessage(toolName)
		return completion, nil, nil
	}
	if completedAgentToolCall(toolSteps, toolName, raw) && !agentRepeatableTool(toolName) {
		completion.Content = "检测到重复操作，已停止重复执行并保留现有结果；请检查尚未完成的要求。"
		return completion, nil, nil
	}
	if failedAgentToolCall(toolSteps, toolName, raw) {
		completion.Content = "相同参数的操作已经失败，未重复执行。"
		return completion, nil, nil
	}
	completion.ToolCall = &agentToolCall{ID: newID("agent-tool-call"), Name: toolName, Arguments: arguments, Raw: raw}
	completion.ToolCalls = []*agentToolCall{completion.ToolCall}
	if agentToolAllowsParallel(toolName) {
		for _, extra := range message.ToolCalls[1:] {
			next, ok := agentParallelToolCall(run, toolSteps, completion.ToolCalls, extra, authorization)
			if !ok {
				break
			}
			completion.ToolCalls = append(completion.ToolCalls, next)
		}
	}
	return completion, nil, nil
}

// Only exact, read-only queries bypass the planner. Natural-language actions never do.
func agentNodeCountReply(run model.AgentRun, messages []model.AgentMessage) (string, bool) {
	for _, message := range messages {
		if message.ID != run.MessageID || message.Role != model.AgentMessageRoleUser {
			continue
		}
		switch strings.Trim(message.Content, " \t\r\n?？。") {
		case "多少节点", "多少个节点", "几个节点", "节点数量", "画布有多少个节点", "当前画布有多少个节点":
			var canvas AgentCanvasContext
			if json.Unmarshal([]byte(run.Context), &canvas) == nil {
				return fmt.Sprintf("当前画布有 %d 个节点。", len(canvas.Nodes)), true
			}
		}
	}
	return "", false
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

func sanitizeAgentUserFacingText(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return content
	}
	content = agentUserFacingGoalPrefix.ReplaceAllString(content, "")
	content = agentUserFacingNodeIDPattern.ReplaceAllString(content, "")
	content = agentEmptyParenPattern.ReplaceAllString(content, "")
	return strings.TrimSpace(agentMultiSpacePattern.ReplaceAllString(content, "$1 "))
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

func agentStepInspection(step model.AgentStep) (AgentToolInspection, error) {
	var result SubmitAgentToolResultRequest
	if json.Unmarshal([]byte(step.Output), &result) != nil || result.Status != "success" || result.Inspection == nil {
		return AgentToolInspection{}, errors.New("agent media inspection invalid")
	}
	return *result.Inspection, nil
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
		return "本轮自主等级为谨慎：Skill 声明的检查点全部执行；其余决定优先从上下文推断，只有会显著改变结果且无法可靠推断的信息才使用 agent_ask_user。"
	case agentAutonomyAutonomous:
		return "本轮自主等级为自主：不为创作决定提问，未指定的风格、构图、数量和时长按推荐方案执行，并在交付时用一句话说明假设；只有完全没有可执行的主体或目标时才询问。整项用户任务完成后结束。媒体验收需要修订时按建议调整一次，保留用户指定数量、时长和参考媒体。非破坏性工具失败时可以修改参数重试一次，禁止原参数重放；重试仍失败则说明阻塞。删除、覆盖文本、记忆写入和遗忘仍不得绕过确认。"
	default:
		return "本轮自主等级为标准：未指定的风格、构图、数量和时长采用合理默认值（默认单张图片、6 秒视频），不为这些细节单独提问；只执行 Skill 标明适用于标准模式的检查点。除此之外，只有没有可执行主体或目标、约束冲突、或必须目标无法唯一确定时才询问。整项用户任务完成后结束。"
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
	if step.Confirmation == "rejected" && step.ToolName != "agent.ask_user" {
		return map[string]string{"status": "rejected", "error": "用户拒绝执行该工具调用"}
	}
	var request SubmitAgentToolResultRequest
	if json.Unmarshal([]byte(step.Output), &request) != nil {
		return map[string]string{"status": string(step.Status)}
	}
	if request.Status == "failed" {
		return map[string]string{"status": request.Status, "error": request.Error}
	}
	switch step.ToolName {
	case "canvas.plan":
		return map[string]string{"status": request.Status, "note": "计划已更新"}
	case "skill.read":
		if request.Skill == nil {
			return map[string]string{"status": request.Status}
		}
		return map[string]string{"status": request.Status, "skill": request.Skill.Title, "content": request.Skill.Content}
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
	case "canvas.apply_ops":
		return agentApplyOpsModelResult(step, request)
	case "agent.ask_user":
		var answer struct {
			Answer string `json:"answer"`
		}
		_ = json.Unmarshal([]byte(step.Output), &answer)
		if step.Confirmation == "rejected" || answer.Answer == "" || answer.Answer == "用户跳过了回答" {
			return map[string]any{"status": "skipped", "note": "用户跳过了提问，不要再次询问；按推荐项（第一个选项）继续完成原任务，并在交付时用一句话说明这个假设"}
		}
		return map[string]any{"status": request.Status, "answer": answer.Answer}
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

// agentRepeatableTool 是允许以相同参数再次调用的工具：媒体分批生成、刷新画布、更新计划和读取 Skill。
func agentRepeatableTool(name string) bool {
	return name == "image.generate" || name == "image.edit" || name == "video.generate" || name == "canvas.read" || agentServerTool(name)
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
	if len(value.SelectedNodeIDs) > 10000 {
		return AgentCanvasContext{}, safeMessageError{message: "选中节点过多"}
	}
	if len(value.FocusNodeIDs) > 20 {
		return AgentCanvasContext{}, safeMessageError{message: "关注节点过多"}
	}
	if len(value.VisibleNodeIDs) > 40 {
		return AgentCanvasContext{}, safeMessageError{message: "可见节点过多"}
	}
	if len(value.Nodes) > 10000 || len(value.Connections) > 50000 {
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
		node.Title = truncateAgentRunes(node.Title, 120)
		node.Status = truncateAgentRunes(node.Status, 20)
		if node.Settings != nil {
			for _, value := range []string{node.Settings.Model, node.Settings.Size, node.Settings.Quality, node.Settings.Resolution, node.Settings.Mode} {
				if utf8.RuneCountInString(value) > 120 {
					return AgentCanvasContext{}, safeMessageError{message: "配置节点参数过长"}
				}
			}
		}
		node.Content, node.Prompt, node.StorageKey = strings.TrimSpace(node.Content), strings.TrimSpace(node.Prompt), strings.TrimSpace(node.StorageKey)
		if utf8.RuneCountInString(node.Content) > 4000 || utf8.RuneCountInString(node.Prompt) > 2000 || utf8.RuneCountInString(node.StorageKey) > 512 || len(node.KeyframeIDs) > 20 {
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
	visibleIDs := make([]string, 0, len(value.VisibleNodeIDs))
	seenVisible := make(map[string]struct{}, len(value.VisibleNodeIDs))
	for _, id := range value.VisibleNodeIDs {
		id = strings.TrimSpace(id)
		if _, exists := nodes[id]; !exists {
			continue
		}
		if _, exists := seenVisible[id]; exists {
			continue
		}
		seenVisible[id] = struct{}{}
		visibleIDs = append(visibleIDs, id)
	}
	defaults := map[string]string{}
	for _, key := range []string{"imageModel", "videoModel", "size", "quality", "resolution", "duration", "generateAudio"} {
		defaults[key] = truncateAgentRunes(value.Defaults[key], 120)
	}
	result := AgentCanvasContext{Autonomy: autonomy, SelectedNodeIDs: validSelectedIDs, FocusNodeIDs: focusIDs, VisibleNodeIDs: visibleIDs, Nodes: make([]AgentCanvasNode, 0, len(nodes)), Connections: connections, Defaults: defaults}
	if viewport := value.Viewport; viewport != nil {
		// 视口只是布局参考，数值异常时丢弃而不是拒绝整个请求。
		if finite(viewport.X) && finite(viewport.Y) && finite(viewport.Width) && finite(viewport.Height) && finite(viewport.Zoom) && math.Abs(viewport.X) <= 1e7 && math.Abs(viewport.Y) <= 1e7 && viewport.Width > 0 && viewport.Height > 0 && viewport.Width <= 1e7 && viewport.Height <= 1e7 && viewport.Zoom >= 0 && viewport.Zoom <= 100 {
			copied := *viewport
			result.Viewport = &copied
		}
	}
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
	for _, id := range append(append([]string{}, full.FocusNodeIDs...), full.SelectedNodeIDs[:min(len(full.SelectedNodeIDs), 20)]...) {
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
	for _, id := range full.VisibleNodeIDs {
		focus[id] = struct{}{}
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
		if _, primary := seeds[node.ID]; !primary && len(retainedNodes) >= 60 {
			continue
		}
		node.StorageKey = ""
		node.Content, node.Prompt = truncateAgentRunes(node.Content, 800), truncateAgentRunes(node.Prompt, 800)
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
		"viewport": full.Viewport,
		"autonomy": full.Autonomy, "selectedNodeIds": full.SelectedNodeIDs[:min(len(full.SelectedNodeIDs), 20)], "focusNodeIds": full.FocusNodeIDs, "visibleNodeIds": full.VisibleNodeIDs,
		"nodes": retainedNodes, "connections": connections, "defaults": full.Defaults,
		"coordinates": "x/y 为节点左上角的画布坐标，width/height 为尺寸，y 轴向下；viewport 为当前可见区域，新内容优先放在视口内的空白处并避免与已有节点重叠",
		"summary":     map[string]any{"nodeCount": len(full.Nodes), "selectedCount": len(full.SelectedNodeIDs), "typeCounts": typeCounts, "bounds": bounds, "generatedNodeIds": generatedNodeIDs},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func agentRunNodeAuthorization(run model.AgentRun) (agentNodeAuthorization, error) {
	authorization := agentNodeAuthorization{NodeIDs: map[string]struct{}{}, ImageNodeIDs: map[string]struct{}{}, VideoNodeIDs: map[string]struct{}{}, AudioNodeIDs: map[string]struct{}{}, TextNodeIDs: map[string]struct{}{}, ConfigNodeIDs: map[string]struct{}{}}
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
		if node.Type == "audio" {
			authorization.AudioNodeIDs[node.ID] = struct{}{}
		}
		if node.Type == "config" {
			authorization.ConfigNodeIDs[node.ID] = struct{}{}
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
			if step.ToolName == "canvas.add_config" {
				authorization.ConfigNodeIDs[result.NodeID] = struct{}{}
			}
		}
		for _, id := range result.NodeIDs {
			authorization.NodeIDs[id] = struct{}{}
		}
		if step.ToolName == "canvas.apply_ops" {
			for _, op := range result.Ops {
				if op.NodeID == "" {
					continue
				}
				if op.Op == "add_text" {
					authorization.NodeIDs[op.NodeID], authorization.TextNodeIDs[op.NodeID] = struct{}{}, struct{}{}
				}
				if op.Op == "add_config" {
					authorization.NodeIDs[op.NodeID], authorization.ConfigNodeIDs[op.NodeID] = struct{}{}, struct{}{}
				}
			}
		}
	}
	return authorization, nil
}

func decodeAgentToolArguments(name, value string, authorization agentNodeAuthorization) (any, string, error) {
	name = canonicalAgentToolName(name)
	var arguments any
	switch name {
	case "canvas.read":
		var input canvasReadArguments
		if err := decodeAgentJSONValue(value, &input); err != nil || len(input.NodeIDs) > 20 || utf8.RuneCountInString(input.Query) > 200 || input.Offset < 0 || input.Offset > 10000 {
			return nil, "", errors.New("canvas.read arguments invalid")
		}
		input.Query = strings.TrimSpace(input.Query)
		arguments = input
	case "canvas.plan":
		var input canvasPlanArguments
		if err := decodeAgentJSONValue(value, &input); err != nil {
			return nil, "", errors.New("canvas.plan arguments invalid")
		}
		input.Explanation = truncateAgentRunes(strings.TrimSpace(input.Explanation), 200)
		steps, inProgress := make([]AgentPlanItem, 0, len(input.Steps)), 0
		for _, item := range input.Steps {
			item.Step, item.Status = truncateAgentRunes(strings.TrimSpace(item.Step), 120), strings.TrimSpace(item.Status)
			if item.Step == "" {
				continue
			}
			if item.Status == "" {
				item.Status = "pending"
			}
			if item.Status != "pending" && item.Status != "in_progress" && item.Status != "completed" {
				return nil, "", errors.New("canvas.plan step status must be pending, in_progress or completed")
			}
			if item.Status == "in_progress" {
				inProgress++
			}
			steps = append(steps, item)
		}
		if len(steps) < 1 || len(steps) > 8 || inProgress > 1 {
			return nil, "", errors.New("canvas.plan needs 1-8 steps with at most one in_progress step")
		}
		input.Steps = steps
		arguments = input
	case "skill.read":
		var input skillReadArguments
		if err := decodeAgentJSONValue(value, &input); err != nil {
			return nil, "", errors.New("skill.read arguments invalid")
		}
		input.SkillID = strings.TrimSpace(input.SkillID)
		if _, ok := agentSkillFor(input.SkillID); !ok {
			return nil, "", errors.New("skill.read skillId must be one of " + strings.Join(agentSkillIDs(), ", "))
		}
		arguments = input
	case "image.generate":
		var input struct {
			AgentMediaOptions
			Prompt           string   `json:"prompt"`
			Count            *int     `json:"count"`
			ReferenceNodeIDs []string `json:"referenceNodeIds"`
		}
		if err := decodeAgentJSONValue(value, &input); err != nil {
			return nil, "", errors.New("image.generate arguments invalid")
		}
		input.Prompt = strings.TrimSpace(input.Prompt)
		if (input.Prompt == "" && input.ConfigNodeID == "") || utf8.RuneCountInString(input.Prompt) > 8000 {
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
		if err := validateAgentMediaOptions(input.AgentMediaOptions, authorization); err != nil {
			return nil, "", err
		}
		arguments = imageGenerateArguments{AgentMediaOptions: input.AgentMediaOptions, Prompt: input.Prompt, Count: count, ReferenceNodeIDs: input.ReferenceNodeIDs}
	case "image.edit":
		var input struct {
			AgentMediaOptions
			NodeID string `json:"nodeId"`
			Prompt string `json:"prompt"`
			Count  *int   `json:"count"`
		}
		if err := decodeAgentJSONValue(value, &input); err != nil {
			return nil, "", errors.New("image.edit arguments invalid")
		}
		input.NodeID, input.Prompt = strings.TrimSpace(input.NodeID), strings.TrimSpace(input.Prompt)
		if input.NodeID == "" || (input.Prompt == "" && input.ConfigNodeID == "") || utf8.RuneCountInString(input.Prompt) > 8000 {
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
		if err := validateAgentMediaOptions(input.AgentMediaOptions, authorization); err != nil {
			return nil, "", err
		}
		arguments = imageEditArguments{AgentMediaOptions: input.AgentMediaOptions, NodeID: input.NodeID, Prompt: input.Prompt, Count: count}
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
			AgentMediaOptions
			Prompt       string   `json:"prompt"`
			Duration     *int     `json:"duration"`
			ImageNodeID  string   `json:"imageNodeId"`
			ImageNodeIDs []string `json:"imageNodeIds"`
			VideoNodeIDs []string `json:"videoNodeIds"`
			AudioNodeIDs []string `json:"audioNodeIds"`
		}
		if err := decodeAgentJSONValue(value, &input); err != nil {
			return nil, "", errors.New("video.generate arguments invalid")
		}
		input.Prompt, input.ImageNodeID = strings.TrimSpace(input.Prompt), strings.TrimSpace(input.ImageNodeID)
		if (input.Prompt == "" && input.ConfigNodeID == "") || utf8.RuneCountInString(input.Prompt) > 8000 {
			return nil, "", errors.New("video.generate prompt invalid")
		}
		duration := 6
		if input.Duration != nil {
			duration = *input.Duration
		}
		if duration < 1 || duration > 20 {
			return nil, "", errors.New("video.generate duration invalid")
		}
		imageIDs := append([]string{}, input.ImageNodeIDs...)
		if input.ImageNodeID != "" {
			imageIDs = append([]string{input.ImageNodeID}, imageIDs...)
		}
		normalizedImages, err := normalizeAuthorizedNodeIDs(imageIDs, authorization.ImageNodeIDs, 10, "video.generate imageNodeIds")
		if err != nil {
			return nil, "", err
		}
		normalizedVideos, err := normalizeAuthorizedNodeIDs(input.VideoNodeIDs, authorization.VideoNodeIDs, 3, "video.generate videoNodeIds")
		if err != nil {
			return nil, "", err
		}
		normalizedAudios, err := normalizeAuthorizedNodeIDs(input.AudioNodeIDs, authorization.AudioNodeIDs, 3, "video.generate audioNodeIds")
		if err != nil {
			return nil, "", err
		}
		firstImage := ""
		if len(normalizedImages) > 0 {
			firstImage = normalizedImages[0]
		}
		if err := validateAgentMediaOptions(input.AgentMediaOptions, authorization); err != nil {
			return nil, "", err
		}
		arguments = videoGenerateArguments{AgentMediaOptions: input.AgentMediaOptions, Prompt: input.Prompt, Duration: duration, ImageNodeID: firstImage, ImageNodeIDs: normalizedImages, VideoNodeIDs: normalizedVideos, AudioNodeIDs: normalizedAudios}
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
			input.Placement = "right_of_selection"
		}
		if !validAgentPlacement(input.Placement) {
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
	case "canvas.add_config":
		var input canvasAddConfigArguments
		if err := decodeAgentJSONValue(value, &input); err != nil {
			return nil, "", errors.New("canvas.add_config arguments invalid")
		}
		input.Placement = strings.TrimSpace(input.Placement)
		if input.Placement == "" {
			input.Placement = "right_of_selection"
		}
		if !validAgentPlacement(input.Placement) {
			return nil, "", errors.New("canvas.add_config placement invalid")
		}
		if len(input.SourceNodeIDs) > 20 {
			return nil, "", errors.New("canvas.add_config sourceNodeIds invalid")
		}
		seen := make(map[string]struct{}, len(input.SourceNodeIDs))
		for i, id := range input.SourceNodeIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				return nil, "", errors.New("canvas.add_config sourceNodeIds invalid")
			}
			if _, exists := seen[id]; exists {
				return nil, "", errors.New("canvas.add_config sourceNodeIds invalid")
			}
			if _, exists := authorization.NodeIDs[id]; !exists {
				return nil, "", errors.New("canvas.add_config sourceNodeIds unauthorized")
			}
			seen[id], input.SourceNodeIDs[i] = struct{}{}, id
		}
		if err := validateAgentMediaOptions(input.AgentMediaOptions, authorization); err != nil {
			return nil, "", err
		}
		if utf8.RuneCountInString(input.Prompt) > 8000 {
			return nil, "", errors.New("canvas.add_config prompt invalid")
		}
		arguments = input
	case "canvas.apply_ops":
		input, err := decodeCanvasApplyOps(value, authorization)
		if err != nil {
			return nil, "", err
		}
		arguments = input
	case "canvas.connect":
		var input canvasConnectArguments
		if err := decodeAgentJSONValue(value, &input); err != nil {
			return nil, "", errors.New("canvas.connect arguments invalid")
		}
		input.FromNodeID, input.ToNodeID = strings.TrimSpace(input.FromNodeID), strings.TrimSpace(input.ToNodeID)
		if input.FromNodeID == "" || input.ToNodeID == "" || input.FromNodeID == input.ToNodeID {
			return nil, "", errors.New("canvas.connect arguments invalid")
		}
		if _, exists := authorization.NodeIDs[input.FromNodeID]; !exists {
			return nil, "", errors.New("canvas.connect fromNodeId unauthorized")
		}
		if _, exists := authorization.NodeIDs[input.ToNodeID]; !exists {
			return nil, "", errors.New("canvas.connect toNodeId unauthorized")
		}
		_, fromConfig := authorization.ConfigNodeIDs[input.FromNodeID]
		_, toConfig := authorization.ConfigNodeIDs[input.ToNodeID]
		if fromConfig && toConfig {
			return nil, "", errors.New("canvas.connect cannot link two config nodes")
		}
		arguments = input
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
		input.Header, input.Question = truncateAgentRunes(strings.TrimSpace(input.Header), 40), strings.TrimSpace(input.Question)
		if input.Question == "" || utf8.RuneCountInString(input.Question) > 500 || len(input.Options) < 2 || len(input.Options) > 4 {
			return nil, "", errors.New("agent.ask_user needs a question and 2-4 options")
		}
		seenOptions := make(map[string]struct{}, len(input.Options))
		for i := range input.Options {
			option := &input.Options[i]
			option.Label, option.Description = strings.TrimSpace(option.Label), strings.TrimSpace(option.Description)
			if option.Label == "" || utf8.RuneCountInString(option.Label) > 40 || utf8.RuneCountInString(option.Description) > 160 {
				return nil, "", errors.New("agent.ask_user option label must be 1-40 characters and description at most 160 characters")
			}
			if _, exists := seenOptions[option.Label]; exists {
				return nil, "", errors.New("agent.ask_user option labels must be unique")
			}
			seenOptions[option.Label] = struct{}{}
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
	if request.Plan != nil || request.Skill != nil {
		return safeMessageError{message: "工具结果包含服务端专用数据"}
	}
	if name != "canvas.read" && request.Observation != nil {
		return safeMessageError{message: "工具结果包含多余的画布观察数据"}
	}
	if name != "image.inspect" && name != "video.inspect" && request.Inspection != nil {
		return safeMessageError{message: "工具结果包含多余的视觉验收数据"}
	}
	if name != "agent.remember" && name != "agent.forget" && request.Memory != nil {
		return safeMessageError{message: "工具结果包含多余的记忆数据"}
	}
	if name != "canvas.apply_ops" && len(request.Ops) != 0 {
		return safeMessageError{message: "工具结果包含多余的批量操作数据"}
	}
	switch name {
	case "canvas.apply_ops":
		return validateCanvasApplyOpsResult(arguments.(canvasApplyOpsArguments), request)
	case "canvas.read":
		if request.Observation == nil || len(request.Observation.Nodes) > 20 || request.Plan != nil || request.Video != nil || len(request.Images) != 0 || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" || request.FromNodeID != "" || request.ToNodeID != "" {
			return safeMessageError{message: "画布读取结果无效"}
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
		if request.Plan != nil || request.Video != nil || len(request.Images) != 0 || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.NodeID == "" || request.Text != "" || request.Placement != input.Placement || request.FromNodeID != "" || request.ToNodeID != "" {
			return safeMessageError{message: "文本节点工具结果无效"}
		}
	case "canvas.add_config":
		input := arguments.(canvasAddConfigArguments)
		request.NodeID, request.Placement = strings.TrimSpace(request.NodeID), strings.TrimSpace(request.Placement)
		if request.Plan != nil || request.Video != nil || len(request.Images) != 0 || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.NodeID == "" || request.Text != "" || request.Placement != input.Placement || request.FromNodeID != "" || request.ToNodeID != "" {
			return safeMessageError{message: "配置节点工具结果无效"}
		}
	case "canvas.connect":
		input := arguments.(canvasConnectArguments)
		request.FromNodeID, request.ToNodeID = strings.TrimSpace(request.FromNodeID), strings.TrimSpace(request.ToNodeID)
		if request.Plan != nil || request.Video != nil || len(request.Images) != 0 || len(request.NodeIDs) != 0 || len(request.Positions) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" || request.FromNodeID != input.FromNodeID || request.ToNodeID != input.ToNodeID {
			return safeMessageError{message: "连线工具结果无效"}
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
	case "canvas.read":
		return map[string]any{"observation": request.Observation}
	case "canvas.plan":
		return struct {
			Plan *AgentToolPlan `json:"plan"`
		}{request.Plan}
	case "skill.read":
		if request.Skill == nil {
			return struct{}{}
		}
		return struct {
			Skill AgentToolSkill `json:"skill"`
		}{AgentToolSkill{ID: request.Skill.ID, Title: request.Skill.Title}}
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
	case "canvas.connect":
		return struct {
			FromNodeID string `json:"fromNodeId"`
			ToNodeID   string `json:"toNodeId"`
		}{request.FromNodeID, request.ToNodeID}
	case "canvas.apply_ops":
		return struct {
			NodeIDs []string            `json:"nodeIds"`
			Ops     []AgentToolOpResult `json:"ops"`
		}{request.NodeIDs, request.Ops}
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

func prepareAgentMemoryToolResult(run model.AgentRun, name string, arguments any) (model.AgentMemory, error) {
	session, err := repository.GetAgentSession(run.OrganizationID, run.UserID, run.SessionID)
	if err != nil {
		return model.AgentMemory{}, err
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
		return model.AgentMemory{ID: newID("agent-memory"), OrganizationID: run.OrganizationID, UserID: run.UserID, ProjectID: projectID, Kind: model.AgentMemoryKind(input.Kind), Key: input.Key, Content: input.Content, SourceRunID: run.ID, SourceMessageID: run.MessageID, Confidence: input.Confidence, Status: model.AgentMemoryStatusActive, ExpiresAt: expiresAt, CreatedAt: timestamp, UpdatedAt: timestamp}, nil
	}
	input := arguments.(agentForgetArguments)
	if input.Scope == "user" {
		projectID = ""
	}
	return model.AgentMemory{OrganizationID: run.OrganizationID, UserID: run.UserID, ProjectID: projectID, Key: input.Key, Status: model.AgentMemoryStatusForgotten, ForgottenAt: timestamp, UpdatedAt: timestamp}, nil
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
	schemas := []any{
		map[string]any{"type": "function", "function": map[string]any{"name": "canvas_read", "description": "读取当前画布或按标题、文字、提示词查找节点；返回最多20个节点及相关连线，需要更多时 offset 加20，空结果表示没有匹配；视口外对象也可查询；搜索返回文字摘要，编辑前用nodeIds读取具体内容", "parameters": map[string]any{"type": "object", "properties": map[string]any{"nodeIds": map[string]any{"type": "array", "maxItems": 20, "items": map[string]any{"type": "string"}}, "query": map[string]any{"type": "string", "maxLength": 200}, "offset": map[string]any{"type": "integer", "minimum": 0, "maximum": 10000}}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "canvas_plan", "description": "维护本轮的工作计划，不会改动画布：三步及以上的任务开始时提交全部步骤，之后每推进一步再次调用并提交完整的最新状态（同一时刻最多一个 in_progress）；可以和其他工具并行调用", "parameters": map[string]any{"type": "object", "properties": map[string]any{"explanation": map[string]any{"type": "string", "maxLength": 200, "description": "计划调整的简短说明，可省略"}, "steps": map[string]any{"type": "array", "minItems": 1, "maxItems": 8, "items": map[string]any{"type": "object", "properties": map[string]any{"step": map[string]any{"type": "string", "maxLength": 120}, "status": map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}}}, "required": []string{"step", "status"}, "additionalProperties": false}}}, "required": []string{"steps"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "skill_read", "description": "读取一份专业创作 Skill（方法、检查点和自检清单），内容会附带创作通用规范；产出对应领域的专业结果前调用，同一轮读过的不必重读", "parameters": map[string]any{"type": "object", "properties": map[string]any{"skillId": map[string]any{"type": "string", "enum": agentSkillIDs()}}, "required": []string{"skillId"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "image_generate", "description": "根据提示词生成一至四张图片，可通过 referenceNodeIds 引用画布中的图片节点作为参考", "parameters": map[string]any{"type": "object", "properties": map[string]any{"prompt": map[string]any{"type": "string", "maxLength": 8000}, "count": map[string]any{"type": "integer", "minimum": 1, "maximum": 4, "default": 1}, "referenceNodeIds": map[string]any{"type": "array", "maxItems": 6, "uniqueItems": true, "items": map[string]any{"type": "string"}}}, "required": []string{"prompt"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "image_edit", "description": "以画布中一个图片节点为源图进行编辑修改，生成一至四张新图片并自动连线；用户要求修改、替换、调整已有图片时优先使用，而不是重新生成", "parameters": map[string]any{"type": "object", "properties": map[string]any{"nodeId": map[string]any{"type": "string"}, "prompt": map[string]any{"type": "string", "maxLength": 8000}, "count": map[string]any{"type": "integer", "minimum": 1, "maximum": 4, "default": 1}}, "required": []string{"nodeId", "prompt"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "image_inspect", "description": "使用视觉模型对照用户目标验收一至四个图片节点；图片生成后、总结完成前必须调用", "parameters": map[string]any{"type": "object", "properties": map[string]any{"nodeIds": map[string]any{"type": "array", "minItems": 1, "maxItems": 4, "uniqueItems": true, "items": map[string]any{"type": "string"}}, "criteria": map[string]any{"type": "string", "maxLength": 2000}}, "required": []string{"nodeIds", "criteria"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "video_generate", "description": "生成一个视频；参考面与画布一致，最多 10 张图、3 个视频、3 个音频，实际以所选模型上限为准", "parameters": map[string]any{"type": "object", "properties": map[string]any{"prompt": map[string]any{"type": "string", "maxLength": 8000}, "duration": map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "default": 6}, "imageNodeIds": map[string]any{"type": "array", "maxItems": 10, "uniqueItems": true, "items": map[string]any{"type": "string"}}, "videoNodeIds": map[string]any{"type": "array", "maxItems": 3, "uniqueItems": true, "items": map[string]any{"type": "string"}}, "audioNodeIds": map[string]any{"type": "array", "maxItems": 3, "uniqueItems": true, "items": map[string]any{"type": "string"}}, "imageNodeId": map[string]any{"type": "string"}}, "required": []string{"prompt"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "video_inspect", "description": "按时间顺序抽取关键帧并使用视觉模型对照用户目标验收一个视频节点；视频生成后、总结完成前必须调用", "parameters": map[string]any{"type": "object", "properties": map[string]any{"nodeId": map[string]any{"type": "string"}, "criteria": map[string]any{"type": "string", "maxLength": 2000}}, "required": []string{"nodeId", "criteria"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "canvas_arrange", "description": "重排画布中相关节点", "parameters": map[string]any{"type": "object", "properties": map[string]any{"nodeIds": map[string]any{"type": "array", "minItems": 2, "maxItems": 20, "uniqueItems": true, "items": map[string]any{"type": "string"}}, "mode": map[string]any{"type": "string", "enum": []string{"horizontal", "vertical", "grid"}}, "gap": map[string]any{"type": "integer", "minimum": 16, "maximum": 400, "default": 40}}, "required": []string{"nodeIds", "mode"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "canvas_add_text", "description": "仅当用户明确要求把文案、注释或其他文字放到画布上时插入文本节点，并可用连线关联来源；不得用来承载用户消息、助手回复、总结、问候或闲聊；落位优先选区右侧、下方或当前视口", "parameters": map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string", "maxLength": 4000}, "placement": map[string]any{"type": "string", "enum": []string{"center", "right_of_selection", "below_selection", "viewport"}, "default": "right_of_selection"}, "sourceNodeIds": map[string]any{"type": "array", "maxItems": 20, "uniqueItems": true, "items": map[string]any{"type": "string"}}}, "required": []string{"text"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "canvas_add_config", "description": "按画布语法创建生成配置节点，并用 sourceNodeIds 作为上游输入；后续出图/出视频应连到该配置", "parameters": map[string]any{"type": "object", "properties": map[string]any{"placement": map[string]any{"type": "string", "enum": []string{"center", "right_of_selection", "below_selection", "viewport"}, "default": "right_of_selection"}, "sourceNodeIds": map[string]any{"type": "array", "maxItems": 20, "uniqueItems": true, "items": map[string]any{"type": "string"}}}, "required": []string{}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "canvas_connect", "description": "在两个已有节点之间建立来源连线；配置节点不能互连", "parameters": map[string]any{"type": "object", "properties": map[string]any{"fromNodeId": map[string]any{"type": "string"}, "toNodeId": map[string]any{"type": "string"}}, "required": []string{"fromNodeId", "toNodeId"}, "additionalProperties": false}}},
		agentApplyOpsSchema(),
		map[string]any{"type": "function", "function": map[string]any{"name": "canvas_delete", "description": "删除本轮授权的画布节点，执行前需要用户确认", "parameters": map[string]any{"type": "object", "properties": map[string]any{"nodeIds": map[string]any{"type": "array", "minItems": 1, "maxItems": 20, "uniqueItems": true, "items": map[string]any{"type": "string"}}}, "required": []string{"nodeIds"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "canvas_update_text", "description": "修改本轮授权的文本节点，执行前需要用户确认", "parameters": map[string]any{"type": "object", "properties": map[string]any{"nodeId": map[string]any{"type": "string"}, "text": map[string]any{"type": "string", "maxLength": 4000}}, "required": []string{"nodeId", "text"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "agent_ask_user", "description": "向用户发出一张决策卡并等待回答：只用于满足硬对齐条件的决定或 Skill 声明的检查点；给 2-4 个互斥的完整方案，推荐项放第一位，description 写清差异和推荐理由；用户也可以输入其他回答或跳过。需要的文案、建议等内容应自己撰写，不要向用户索要", "parameters": map[string]any{"type": "object", "properties": map[string]any{"header": map[string]any{"type": "string", "maxLength": 40, "description": "卡片标题，例如“视觉方向”"}, "question": map[string]any{"type": "string", "maxLength": 500}, "options": map[string]any{"type": "array", "minItems": 2, "maxItems": 4, "items": map[string]any{"type": "object", "properties": map[string]any{"label": map[string]any{"type": "string", "maxLength": 40}, "description": map[string]any{"type": "string", "maxLength": 160}}, "required": []string{"label"}, "additionalProperties": false}}}, "required": []string{"question", "options"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "agent_remember", "description": "在用户明确要求长期记住项目偏好、事实或约束时保存一条可审计记忆；执行前需要用户确认，不要保存敏感信息", "parameters": map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"preference", "fact", "constraint", "experience"}}, "key": map[string]any{"type": "string", "maxLength": 120}, "content": map[string]any{"type": "string", "maxLength": 1000}, "scope": map[string]any{"type": "string", "enum": []string{"project", "user"}, "default": "project"}, "confidence": map[string]any{"type": "number", "minimum": 0.5, "maximum": 1, "default": 0.8}, "expiresInDays": map[string]any{"type": "integer", "minimum": 0, "maximum": 3650, "default": 0}}, "required": []string{"kind", "key", "content"}, "additionalProperties": false}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "agent_forget", "description": "删除一条已保存的长期记忆；执行前需要用户确认", "parameters": map[string]any{"type": "object", "properties": map[string]any{"key": map[string]any{"type": "string", "maxLength": 120}, "scope": map[string]any{"type": "string", "enum": []string{"project", "user"}, "default": "project"}}, "required": []string{"key"}, "additionalProperties": false}}},
	}
	for _, schema := range schemas {
		function := schema.(map[string]any)["function"].(map[string]any)
		name := function["name"].(string)
		if name != "image_generate" && name != "image_edit" && name != "video_generate" && name != "canvas_add_config" {
			continue
		}
		properties := function["parameters"].(map[string]any)["properties"].(map[string]any)
		if name != "canvas_add_config" {
			function["parameters"].(map[string]any)["required"] = []string{}
			if name == "image_edit" {
				function["parameters"].(map[string]any)["required"] = []string{"nodeId"}
			}
			properties["prompt"].(map[string]any)["description"] = "提示词；使用 configNodeId 时可省略以使用配置节点当前完整提示词及上游文本"
		}
		for _, key := range []string{"model", "size", "quality", "resolution", "configNodeId"} {
			properties[key] = map[string]any{"type": "string", "maxLength": 120}
		}
		properties["size"].(map[string]any)["description"] = "图片可传像素尺寸如2048x2048或比例，视频传比例如16:9"
		properties["resolution"].(map[string]any)["description"] = "仅用于视频清晰度，图片分辨率使用size像素尺寸"
		if name == "video_generate" {
			delete(properties, "quality")
			delete(properties["duration"].(map[string]any), "default")
		}
		if name == "video_generate" || name == "canvas_add_config" {
			properties["generateAudio"] = map[string]any{"type": "boolean"}
		} else {
			delete(properties, "resolution")
		}
		if name == "canvas_add_config" {
			delete(properties, "configNodeId")
			properties["mode"] = map[string]any{"type": "string", "enum": []string{"image", "video"}}
			properties["prompt"] = map[string]any{"type": "string", "maxLength": 8000}
			properties["count"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 4}
			properties["duration"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 20}
		}
	}
	return schemas
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

func validAgentPlacement(value string) bool {
	switch value {
	case "center", "right_of_selection", "below_selection", "viewport":
		return true
	default:
		return false
	}
}

func normalizeAuthorizedNodeIDs(values []string, authorized map[string]struct{}, max int, label string) ([]string, error) {
	if len(values) > max {
		return nil, errors.New(label + " invalid")
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, id := range values {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, errors.New(label + " invalid")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		if _, valid := authorized[id]; !valid {
			return nil, errors.New(label + " unauthorized")
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}

func agentNeedsPlanningPreviews(content string) bool {
	// Only unambiguous small talk skips previews; visual follow-ups may omit node names.
	switch strings.ToLower(strings.Trim(content, " \t\r\n?？!！。")) {
	case "在吗", "你好", "您好", "谢谢", "hi", "hello", "介绍一下你能做什么", "你能做什么", "你是谁":
		return false
	}
	return true
}

func attachAgentPlanningPreviews(messages []agentChatMessage, organizationID, rawContext string) []agentChatMessage {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		text, _ := messages[i].Content.(string)
		if !agentNeedsPlanningPreviews(text) {
			return messages
		}
		parts := agentPlanningPreviewParts(organizationID, rawContext)
		if len(parts) > 0 {
			messages[i].Content = append([]any{map[string]any{"type": "text", "text": text}}, parts...)
		}
		break
	}
	return messages
}

func agentPlanningPreviewParts(organizationID, rawContext string) []any {
	var full AgentCanvasContext
	if json.Unmarshal([]byte(rawContext), &full) != nil {
		return nil
	}
	focus := make(map[string]struct{}, len(full.FocusNodeIDs)+len(full.SelectedNodeIDs))
	for _, id := range full.FocusNodeIDs {
		focus[id] = struct{}{}
	}
	for _, id := range full.SelectedNodeIDs {
		focus[id] = struct{}{}
	}
	priority := make([]string, 0, 16)
	for _, id := range full.VisibleNodeIDs {
		if _, ok := focus[id]; ok {
			priority = append(priority, id)
		}
	}
	priority = append(priority, full.VisibleNodeIDs...)
	priority = append(priority, full.FocusNodeIDs...)
	priority = append(priority, full.SelectedNodeIDs...)
	nodes := make(map[string]AgentCanvasNode, len(full.Nodes))
	for _, node := range full.Nodes {
		nodes[node.ID] = node
	}
	parts := make([]any, 0, 4)
	seen := make(map[string]struct{}, 4)
	for _, id := range priority {
		if len(parts) >= 4 {
			break
		}
		if _, exists := seen[id]; exists {
			continue
		}
		node, ok := nodes[id]
		if !ok || node.Type != "image" || strings.TrimSpace(node.StorageKey) == "" {
			continue
		}
		part, ok := agentPlanningPreviewPart(organizationID, node.StorageKey)
		if !ok {
			continue
		}
		seen[id] = struct{}{}
		parts = append(parts, part)
	}
	return parts
}

func agentPlanningPreviewPart(organizationID, storageKey string) (any, bool) {
	if dataURL, ok := agentLocalPlanningPreviewDataURL(organizationID, storageKey); ok {
		return map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURL}}, true
	}
	url, ok := organizationFileURL(organizationID, storageKey, "thumb", 5*time.Minute)
	if !ok || unfetchableAgentPreviewURL(url) {
		return nil, false
	}
	return map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}}, true
}

func agentLocalPlanningPreviewDataURL(organizationID, storageKey string) (string, bool) {
	item, ok, err := repository.GetUserFile(organizationID, strings.TrimSpace(storageKey))
	if err != nil || !ok || effectiveStorageDriver(item.StorageDriver) != "local" {
		return "", false
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(item.MimeType)), "image/") {
		return "", false
	}
	setting, err := currentUserStorageSetting()
	if err != nil {
		return "", false
	}
	path, err := localStorageObjectPath(setting, item.ObjectKey)
	if err != nil {
		return "", false
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()
	source, err := imaging.Decode(io.LimitReader(file, 8<<20))
	if err != nil {
		return "", false
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, imaging.Fit(source, 160, 160, imaging.Lanczos), &jpeg.Options{Quality: 75}); err != nil || buf.Len() == 0 || buf.Len() > 120_000 {
		return "", false
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), true
}

func unfetchableAgentPreviewURL(fileURL string) bool {
	return strings.Contains(fileURL, "/api/media/storage/") || strings.Contains(fileURL, "/api/workspace/files/")
}

func agentChatMessagesHaveImageURLs(messages []agentChatMessage) bool {
	for _, message := range messages {
		parts, ok := message.Content.([]any)
		if !ok {
			continue
		}
		for _, part := range parts {
			item, ok := part.(map[string]any)
			if ok && item["type"] == "image_url" {
				return true
			}
		}
	}
	return false
}

func stripAgentPlanningPreviews(messages []agentChatMessage) []agentChatMessage {
	for i, message := range messages {
		parts, ok := message.Content.([]any)
		if !ok {
			continue
		}
		text := ""
		for _, part := range parts {
			item, ok := part.(map[string]any)
			if !ok || item["type"] != "text" {
				continue
			}
			if value, ok := item["text"].(string); ok {
				text = value
			}
		}
		if text != "" {
			messages[i].Content = text
		}
	}
	return messages
}

func truncateAgentUpstreamError(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "empty body"
	}
	return truncateAgentRunes(text, 240)
}

func truncateAgentRunes(value string, max int) string {
	if max <= 0 || utf8.RuneCountInString(value) <= max {
		return value
	}
	return string([]rune(value)[:max])
}

// agentToolAllowsParallel 只允许不需要用户确认的工具与其他调用并行。
func agentToolAllowsParallel(name string) bool {
	if agentToolRequiresConfirmation(name) {
		return false
	}
	_, ok := agentToolDefinitionFor(name)
	return ok
}

func agentParallelToolCall(run model.AgentRun, toolSteps []model.AgentStep, accepted []*agentToolCall, call agentUpstreamToolCall, authorization agentNodeAuthorization) (*agentToolCall, bool) {
	if call.Type != "function" {
		return nil, false
	}
	name := canonicalAgentToolName(call.Function.Name)
	if !agentToolAllowsParallel(name) {
		return nil, false
	}
	if name == "canvas.apply_ops" {
		deduped, skipped, empty := dedupeAgentApplyOps(call.Function.Arguments, toolSteps)
		if empty {
			return nil, false
		}
		if skipped > 0 {
			call.Function.Arguments = deduped
		}
	}
	arguments, raw, err := decodeAgentToolArguments(name, agentMediaArgumentsWithDefaults(name, call.Function.Arguments, run.Context), authorization)
	if err != nil {
		return nil, false
	}
	pending := append([]model.AgentStep{}, toolSteps...)
	for _, item := range accepted {
		if item.Name == name && item.Raw == raw {
			return nil, false
		}
		pending = append(pending, model.AgentStep{Type: model.AgentStepTypeTool, Status: model.AgentStepStatusRunning, ToolName: item.Name, Input: item.Raw})
	}
	if agentRunBudgetReason(run, pending, name) != "" || agentMediaRevisionLimitReached(run, pending, name) {
		return nil, false
	}
	if completedAgentToolCall(toolSteps, name, raw) && !agentRepeatableTool(name) {
		return nil, false
	}
	if failedAgentToolCall(toolSteps, name, raw) {
		return nil, false
	}
	return &agentToolCall{ID: newID("agent-tool-call"), Name: name, Arguments: arguments, Raw: raw}, true
}

// requestAgentUpstreamMessage 优先流式请求（标记为不真正流式的渠道直接用普通请求）；上游不支持流式（4xx 或返回普通 JSON）时自动回退。
// 每次尝试有独立超时；连接类错误最多重试 agentUpstreamMaxRetries 次，但尝试已持续 agentUpstreamSlowDropThreshold 以上才断开的不重试（多为代理空闲超时，重试只会再耗一轮）。
func requestAgentUpstreamMessage(ctx context.Context, channel model.ModelChannel, requestID string, bodyValue map[string]any, requestMessages *[]agentChatMessage, hasPreviewImages bool, publisher *agentRunStreamPublisher) (agentUpstreamMessage, error) {
	streaming := agentChannelStreams(channel)
	plainRetried := false
	quickRetries, lateRetries := 0, 0
	client := &http.Client{}
	modelName, _ := bodyValue["model"].(string)
	for attempt := 1; attempt <= 6; attempt++ {
		bodyValue["stream"] = streaming
		if !streaming {
			delete(bodyValue, "stream_options")
		}
		body, err := json.Marshal(bodyValue)
		if err != nil {
			return agentUpstreamMessage{}, err
		}
		message, statusCode, responseBody, elapsed, err := doAgentUpstreamAttempt(ctx, client, channel, requestID, body, streaming, publisher)
		logAgentUpstreamAttempt(requestID, channel.Name, modelName, attempt, streaming, len(body), statusCode, elapsed, err)
		if err != nil {
			if ctx.Err() != nil {
				return agentUpstreamMessage{}, ctx.Err()
			}
			retry, late := agentUpstreamShouldRetry(err, elapsed, quickRetries, lateRetries)
			if !retry {
				return agentUpstreamMessage{}, err
			}
			delay := agentUpstreamRetryDelay(quickRetries)
			if late {
				lateRetries++
			} else {
				quickRetries++
			}
			select {
			case <-ctx.Done():
				return agentUpstreamMessage{}, ctx.Err()
			case <-time.After(delay):
			}
			continue
		}
		if statusCode >= 200 && statusCode < 300 {
			if !plainRetried && agentStreamNeedsPlainRetry(bodyValue, message) {
				// 部分代理把工具调用写成文本或界面标记，改用非流式重试一次（本就非流式的渠道也重新请求一次）。
				plainRetried = true
				streaming = false
				continue
			}
			return message, nil
		}
		if statusCode == http.StatusBadRequest && hasPreviewImages {
			*requestMessages = stripAgentPlanningPreviews(*requestMessages)
			bodyValue["messages"] = *requestMessages
			hasPreviewImages = false
			continue
		}
		if streaming && statusCode >= 400 && statusCode < 500 && statusCode != http.StatusUnauthorized && statusCode != http.StatusForbidden && statusCode != http.StatusTooManyRequests && statusCode != http.StatusPaymentRequired {
			streaming = false
			continue
		}
		return agentUpstreamMessage{}, fmt.Errorf("agent upstream returned status %d: %s", statusCode, truncateAgentUpstreamError(responseBody))
	}
	return agentUpstreamMessage{}, errors.New("agent upstream request failed")
}

// doAgentUpstreamAttempt 发起一次带独立超时的请求并读完响应；2xx 时解析消息，其余状态返回响应体。
func doAgentUpstreamAttempt(ctx context.Context, client *http.Client, channel model.ModelChannel, requestID string, body []byte, streaming bool, publisher *agentRunStreamPublisher) (agentUpstreamMessage, int, []byte, time.Duration, error) {
	started := time.Now()
	attemptCtx, cancel := context.WithTimeout(ctx, agentUpstreamAttemptTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, BuildModelChannelURL(channel, "/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return agentUpstreamMessage{}, 0, nil, 0, err
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	request.Header.Set("Content-Type", "application/json")
	if streaming {
		request.Header.Set("Accept", "text/event-stream")
	}
	request.Header.Set("X-Request-ID", requestID)
	request.Header.Set("Idempotency-Key", requestID)
	response, err := client.Do(request)
	if err != nil {
		return agentUpstreamMessage{}, 0, nil, time.Since(started), err
	}
	defer response.Body.Close()
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		message, parseErr := readAgentUpstreamSuccess(response, streaming, publisher)
		return message, response.StatusCode, nil, time.Since(started), parseErr
	}
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	return agentUpstreamMessage{}, response.StatusCode, responseBody, time.Since(started), readErr
}

// readAgentUpstreamSuccess 解析 2xx 响应：SSE 流、缺少 Content-Type 的流或普通 JSON。
func readAgentUpstreamSuccess(response *http.Response, streaming bool, publisher *agentRunStreamPublisher) (agentUpstreamMessage, error) {
	if streaming && strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		return parseAgentCompletionStream(response.Body, publisher.content, publisher.reasoning, publisher.toolReady)
	}
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if readErr != nil {
		return agentUpstreamMessage{}, readErr
	}
	trimmed := bytes.TrimSpace(responseBody)
	if streaming && bytes.HasPrefix(trimmed, []byte("data:")) {
		return parseAgentCompletionStream(bytes.NewReader(trimmed), publisher.content, publisher.reasoning, publisher.toolReady)
	}
	return parseAgentCompletionJSON(responseBody)
}
