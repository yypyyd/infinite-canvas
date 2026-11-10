package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

// dispatchAgentToolCalls 落库本轮工具调用：计划和 Skill 当场执行，生成与验收交给服务端后台执行，其余下发给画布页面；
// 整批都当场完成时直接进入下一次规划。
func dispatchAgentToolCalls(run model.AgentRun, userGroup string, completion agentCompletion) {
	timestamp := now()
	events := []model.AgentEvent{}
	if completion.Content != "" || completion.Reasoning != "" {
		payload, _ := json.Marshal(agentMessagePayload(completion.Content, completion.Reasoning))
		events = append(events, model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventMessageCommentary, Payload: string(payload), CreatedAt: timestamp})
	}
	newStep := func(call *agentToolCall) model.AgentStep {
		return model.AgentStep{ID: newID("agent-step"), OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeTool, Status: model.AgentStepStatusRunning, ToolCallID: call.ID, ToolName: call.Name, Input: call.Raw, StartedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp}
	}
	if agentToolRequiresConfirmation(completion.ToolCall.Name) {
		payload, _ := json.Marshal(map[string]any{"callId": completion.ToolCall.ID, "name": completion.ToolCall.Name, "arguments": completion.ToolCall.Arguments})
		events = append(events, model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolConfirmationRequired, Payload: string(payload), CreatedAt: timestamp})
		if err := repository.WaitAgentRunForConfirmation(run.OrganizationID, run.UserID, run.ID, completion.Content, newStep(completion.ToolCall), timestamp, events...); err != nil {
			failAgentRunUnlessCancelled(run, err)
		}
		return
	}
	calls := completion.ToolCalls
	if len(calls) == 0 || calls[0] != completion.ToolCall {
		calls = []*agentToolCall{completion.ToolCall}
	}
	steps := make([]model.AgentStep, 0, len(calls))
	pendingCalls, backgroundCalls := 0, []string{}
	for _, call := range calls {
		step := newStep(call)
		if agentServerTool(call.Name) {
			result, extra := executeAgentServerTool(run, call, timestamp)
			output, _ := json.Marshal(result)
			step.Status, step.Output, step.Error, step.CompletedAt = model.AgentStepStatusCompleted, string(output), result.Error, timestamp
			if result.Status != "success" {
				step.Status = model.AgentStepStatusFailed
			}
			completed, _ := json.Marshal(map[string]any{"callId": call.ID, "name": call.Name, "arguments": call.Arguments, "status": result.Status, "output": agentToolResultOutput(call.Name, result)})
			events = append(events, extra...)
			events = append(events, model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolCompleted, Payload: string(completed), CreatedAt: timestamp})
		} else {
			pendingCalls++
			payload := map[string]any{"callId": call.ID, "name": call.Name, "arguments": call.Arguments}
			if call.Name == "canvas.arrange" {
				payload["meta"] = map[string]any{"needsClaim": false}
			}
			if agentBackgroundTool(call.Name) {
				payload["meta"] = agentBackgroundToolMeta(run, call.Name, call.ID, call.Arguments)
				step.PresentToken = agentToolPresentation(call.Name)
				backgroundCalls = append(backgroundCalls, call.ID)
			}
			encoded, _ := json.Marshal(payload)
			events = append(events, model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolCall, Payload: string(encoded), CreatedAt: timestamp})
		}
		steps = append(steps, step)
	}
	if pendingCalls == 0 {
		// 下一次规划的步骤必须排在本批工具之后，回放对话时才能保持调用顺序。
		nextAt := time.Now().UTC().Add(time.Millisecond).Format(timestampLayout)
		next := model.AgentStep{ID: newID("agent-step"), OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeCompletion, Status: model.AgentStepStatusRunning, Input: "{}", StartedAt: nextAt, CreatedAt: nextAt, UpdatedAt: nextAt}
		if err := repository.ContinueAgentRunWithServerTools(run.OrganizationID, run.UserID, run.ID, completion.Content, steps, next, timestamp, events...); err != nil {
			failAgentRunUnlessCancelled(run, err)
			return
		}
		startAgentRun(run, userGroup)
		return
	}
	if err := repository.WaitAgentRunForTools(run.OrganizationID, run.UserID, run.ID, completion.Content, steps, timestamp, events...); err != nil {
		failAgentRunUnlessCancelled(run, err)
		return
	}
	time.AfterFunc(agentWaitingToolTimeout, func() { _ = expireWaitingAgentRun(run.OrganizationID, run.UserID, run.ID) })
	for _, callID := range backgroundCalls {
		startAgentBackgroundTool(run.OrganizationID, run.UserID, run.ID, callID)
	}
}

func executeAgentServerTool(run model.AgentRun, call *agentToolCall, timestamp string) (SubmitAgentToolResultRequest, []model.AgentEvent) {
	result := SubmitAgentToolResultRequest{CallID: call.ID, Status: "success"}
	switch call.Name {
	case "canvas.plan":
		plan := call.Arguments.(canvasPlanArguments)
		items := make([]model.AgentPlanStep, 0, len(plan.Steps))
		for _, item := range plan.Steps {
			status := model.AgentPlanStepStatusPending
			if item.Status == "in_progress" {
				status = model.AgentPlanStepStatusRunning
			} else if item.Status == "completed" {
				status = model.AgentPlanStepStatusCompleted
			}
			items = append(items, model.AgentPlanStep{Title: item.Step, Status: status})
		}
		if _, err := repository.ReplaceAgentPlan(run.OrganizationID, run.UserID, run.ID, call.ID, items, timestamp); err != nil {
			return SubmitAgentToolResultRequest{CallID: call.ID, Status: "failed", Error: "计划保存失败"}, nil
		}
		result.Plan = &plan
		payload, _ := json.Marshal(map[string]any{"callId": call.ID, "explanation": plan.Explanation, "steps": plan.Steps})
		return result, []model.AgentEvent{{ID: newID("agent-event"), Type: model.AgentEventPlanUpdated, Payload: string(payload), CreatedAt: timestamp}}
	case "skill.read":
		input := call.Arguments.(skillReadArguments)
		skill, _ := agentSkillFor(input.SkillID)
		content, err := agentSkillContent(input.SkillID)
		if err != nil {
			return SubmitAgentToolResultRequest{CallID: call.ID, Status: "failed", Error: "Skill 读取失败"}, nil
		}
		result.Skill = &AgentToolSkill{ID: skill.ID, Title: skill.Title, Content: content}
		return result, nil
	default:
		return SubmitAgentToolResultRequest{CallID: call.ID, Status: "failed", Error: "不支持的服务端工具"}, nil
	}
}

func agentMessagePayload(content, reasoning string) map[string]string {
	payload := map[string]string{"content": content}
	if reasoning = truncateAgentRunes(strings.TrimSpace(reasoning), agentReasoningMaxRunes); reasoning != "" {
		payload["reasoning"] = reasoning
	}
	return payload
}

// agentConversationMessages 回放最近几轮对话：每条用户消息后接该轮的工具调用、工具结果和回复，统一使用原生 tool 消息。
func agentConversationMessages(run model.AgentRun, messages []model.AgentMessage) ([]agentChatMessage, error) {
	currentIndex := len(messages) - 1
	for i := range messages {
		if messages[i].ID == run.MessageID {
			currentIndex = i
			break
		}
	}
	userMessages := make([]model.AgentMessage, 0, agentHistoryMaxTurns+1)
	for i := currentIndex; i >= 0 && len(userMessages) <= agentHistoryMaxTurns; i-- {
		if messages[i].Role == model.AgentMessageRoleUser {
			userMessages = append([]model.AgentMessage{messages[i]}, userMessages...)
		}
	}
	messageIDs := make([]string, 0, len(userMessages))
	for _, message := range userMessages {
		messageIDs = append(messageIDs, message.ID)
	}
	runs, err := repository.ListAgentRunsByMessages(run.OrganizationID, run.UserID, messageIDs)
	if err != nil {
		return nil, err
	}
	runByMessage := make(map[string]string, len(runs))
	runIDs := make([]string, 0, len(runs))
	for _, item := range runs {
		runByMessage[item.MessageID] = item.ID
		runIDs = append(runIDs, item.ID)
	}
	steps, err := repository.ListAgentStepsByRuns(run.OrganizationID, run.UserID, runIDs)
	if err != nil {
		return nil, err
	}
	stepsByRun := make(map[string][]model.AgentStep, len(runIDs))
	for _, step := range steps {
		stepsByRun[step.RunID] = append(stepsByRun[step.RunID], step)
	}
	result := make([]agentChatMessage, 0, len(userMessages)*4)
	for index, message := range userMessages {
		current := message.ID == run.MessageID
		content := message.Content
		if !current {
			content = truncateAgentRunes(content, agentHistoryMaxRunes)
		}
		result = append(result, agentChatMessage{Role: "user", Content: content})
		runID, ok := runByMessage[message.ID]
		if !ok {
			continue
		}
		// 当前轮完整回放；最近几轮保留截断后的工具调用与结果；更早的轮次只留最终回复，控制请求体大小。
		transcript := agentRunTranscript(stepsByRun[runID], !current, len(userMessages)-1-index > agentHistoryToolTurns)
		if len(transcript) == 0 && !current {
			transcript = []agentChatMessage{{Role: "assistant", Content: "（这一轮没有完成，未产生回复）"}}
		}
		result = append(result, transcript...)
	}
	return result, nil
}

// agentRunTranscript 把一轮的规划与工具步骤还原成 assistant(tool_calls) + tool 消息；
// compact 截断较早轮次的内容，textOnly 只保留该轮最后一条回复。
func agentRunTranscript(steps []model.AgentStep, compact, textOnly bool) []agentChatMessage {
	result := []agentChatMessage{}
	var pending *agentChatMessage
	var toolMessages []agentChatMessage
	flush := func() {
		if pending != nil && (pending.Content != nil || len(pending.ToolCalls) > 0) {
			result = append(append(result, *pending), toolMessages...)
		}
		pending, toolMessages = nil, nil
	}
	for _, step := range steps {
		if step.Type == model.AgentStepTypeCompletion {
			flush()
			if step.Status != model.AgentStepStatusCompleted {
				continue
			}
			pending = &agentChatMessage{Role: "assistant"}
			if content := strings.TrimSpace(step.Output); content != "" {
				if compact {
					content = truncateAgentRunes(content, agentHistoryMaxRunes)
				}
				pending.Content = content
			}
			continue
		}
		if textOnly || step.Output == "" || (step.Status != model.AgentStepStatusCompleted && step.Status != model.AgentStepStatusFailed) {
			continue
		}
		if pending == nil {
			pending = &agentChatMessage{Role: "assistant"}
		}
		callID := agentTranscriptCallID(step.ToolCallID)
		pending.ToolCalls = append(pending.ToolCalls, agentChatToolCall{ID: callID, Type: "function", Function: agentChatToolFunction{Name: agentToolAlias(step.ToolName), Arguments: step.Input}})
		toolMessages = append(toolMessages, agentChatMessage{Role: "tool", ToolCallID: callID, Content: agentTranscriptToolContent(step, compact)})
	}
	flush()
	if textOnly && len(result) > 1 {
		return result[len(result)-1:]
	}
	return result
}

// agentTranscriptCallID 把内部工具调用编号压成上游接受的短编号（OpenAI 限制 40 个字符）。
func agentTranscriptCallID(callID string) string {
	sum := sha256.Sum256([]byte(callID))
	return "call_" + hex.EncodeToString(sum[:12])
}

func agentTranscriptToolContent(step model.AgentStep, compact bool) string {
	if compact && (step.ToolName == "canvas.read" || step.ToolName == "skill.read") {
		return `{"status":"success","note":"内容已过期，需要时重新读取"}`
	}
	encoded, _ := json.Marshal(agentModelToolResult(step))
	if compact {
		return truncateAgentRunes(string(encoded), 1200)
	}
	return string(encoded)
}
