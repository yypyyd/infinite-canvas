package service

import (
	"encoding/json"
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

type AgentEventView struct {
	ID        string               `json:"id"`
	RunID     string               `json:"runId"`
	Sequence  int64                `json:"sequence"`
	Type      model.AgentEventType `json:"type"`
	Data      json.RawMessage      `json:"data"`
	CreatedAt string               `json:"createdAt"`
}

type AgentTimelineRun struct {
	Run              model.AgentRun   `json:"run"`
	MessageID        string           `json:"messageId"`
	ReferenceNodeIDs []string         `json:"referenceNodeIds"`
	Events           []AgentEventView `json:"events"`
	// PendingPresentations 是服务端已经生成完成、还没落到画布的工具调用，页面打开时据此补建节点。
	PendingPresentations []string `json:"pendingPresentations"`
}

// AgentSessionTimeline 是一个会话的完整回放材料：用户与助手消息、每条用户消息对应的运行及其已落库事件。
type AgentSessionTimeline struct {
	Session  model.AgentSession   `json:"session"`
	Messages []model.AgentMessage `json:"messages"`
	Runs     []AgentTimelineRun   `json:"runs"`
}

func ListAgentSessions(user model.AuthUser, projectID string) ([]model.AgentSession, error) {
	projectID = strings.TrimSpace(projectID)
	exists, err := repository.UserProjectExists(user.OrganizationID, user.ID, projectID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, safeMessageError{message: "画布项目不存在"}
	}
	return repository.ListAgentSessions(user.OrganizationID, user.ID, projectID, 50)
}

func GetAgentSessionTimeline(user model.AuthUser, sessionID string) (AgentSessionTimeline, error) {
	session, err := repository.GetAgentSession(user.OrganizationID, user.ID, strings.TrimSpace(sessionID))
	if err != nil || session.Status != model.AgentSessionStatusActive {
		return AgentSessionTimeline{}, safeMessageError{message: "对话记录不存在"}
	}
	messages, err := repository.ListAgentSessionMessages(user.OrganizationID, user.ID, session.ID, 200)
	if err != nil {
		return AgentSessionTimeline{}, err
	}
	runs, err := agentSessionRuns(user, messages)
	if err != nil {
		return AgentSessionTimeline{}, err
	}
	runIDs := make([]string, 0, len(runs))
	for _, run := range runs {
		runIDs = append(runIDs, run.ID)
	}
	events, err := repository.ListAgentEventsByRuns(user.OrganizationID, user.ID, runIDs)
	if err != nil {
		return AgentSessionTimeline{}, err
	}
	eventsByRun := make(map[string][]AgentEventView, len(runs))
	for _, event := range events {
		data := json.RawMessage(event.Payload)
		if !json.Valid(data) {
			data = json.RawMessage(`{}`)
		}
		eventsByRun[event.RunID] = append(eventsByRun[event.RunID], AgentEventView{ID: event.ID, RunID: event.RunID, Sequence: event.Sequence, Type: event.Type, Data: data, CreatedAt: event.CreatedAt})
	}
	pending, err := repository.ListPendingAgentPresentations(user.OrganizationID, user.ID, runIDs)
	if err != nil {
		return AgentSessionTimeline{}, err
	}
	pendingByRun := make(map[string][]string, len(pending))
	for _, step := range pending {
		pendingByRun[step.RunID] = append(pendingByRun[step.RunID], step.ToolCallID)
	}
	timeline := AgentSessionTimeline{Session: session, Messages: messages, Runs: make([]AgentTimelineRun, 0, len(runs))}
	for _, run := range runs {
		// 引用的节点取发送时记在 run.started 里的那一份。
		var started struct {
			ReferenceNodeIDs []string `json:"referenceNodeIds"`
		}
		if events := eventsByRun[run.ID]; len(events) > 0 && events[0].Type == model.AgentEventRunStarted {
			_ = json.Unmarshal(events[0].Data, &started)
		}
		references := started.ReferenceNodeIDs
		if references == nil {
			references = []string{}
		}
		timeline.Runs = append(timeline.Runs, AgentTimelineRun{Run: run, MessageID: run.MessageID, ReferenceNodeIDs: references, Events: eventsByRun[run.ID], PendingPresentations: append([]string{}, pendingByRun[run.ID]...)})
	}
	return timeline, nil
}

// ArchiveAgentSession 归档会话；仍在运行的任务先停止，已开始的生成按实际结果结算。
func ArchiveAgentSession(user model.AuthUser, sessionID string) error {
	if err := RequireOrganizationWrite(user); err != nil {
		return err
	}
	session, err := repository.GetAgentSession(user.OrganizationID, user.ID, strings.TrimSpace(sessionID))
	if err != nil {
		return safeMessageError{message: "对话记录不存在"}
	}
	messages, err := repository.ListAgentSessionMessages(user.OrganizationID, user.ID, session.ID, 200)
	if err != nil {
		return err
	}
	runs, err := agentSessionRuns(user, messages)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if !run.Terminal() {
			if _, err := CancelAgentRun(user, run.ID); err != nil {
				return err
			}
		}
	}
	return repository.ArchiveAgentSession(user.OrganizationID, user.ID, session.ID, now())
}

func agentSessionRuns(user model.AuthUser, messages []model.AgentMessage) ([]model.AgentRun, error) {
	messageIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		if message.Role == model.AgentMessageRoleUser {
			messageIDs = append(messageIDs, message.ID)
		}
	}
	return repository.ListAgentRunsByMessages(user.OrganizationID, user.ID, messageIDs)
}
