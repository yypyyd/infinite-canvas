package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

// 生成与视觉验收工具由服务端在后台执行：认领执行租约并定时续期，进程内走一遍生成接口，等生成任务终态后把结果写回运行。
// 页面不在线也会继续；进程重启后由定时扫描重新接管，生成请求按请求编号幂等，不会重复扣费。

const (
	agentGenerationTimeout       = 45 * time.Minute
	agentInspectionTimeout       = 5 * time.Minute
	agentGenerationPollInterval  = 3 * time.Second
	agentBackgroundScanInterval  = 30 * time.Second
	agentBackgroundFailedMessage = "工具执行失败，请重试"
)

// AgentAIRequester 由 handler 注册：在进程内走一遍 /api/v1 的生成处理流程，选路、扣费、归档和失败返还都与页面发起的请求一致。
var AgentAIRequester func(ctx context.Context, user model.AuthUser, path, contentType, requestID string, body []byte) []byte

var (
	// agentBackgroundToken 是本进程的执行者令牌；其他实例要等租约过期才能接管。
	agentBackgroundToken = newID("server")
	// agentBackgroundTools 保证同一进程内一个调用只有一个执行者。
	agentBackgroundTools sync.Map
)

type ClaimAgentToolPresentationRequest struct {
	CanvasContext *AgentCanvasContext `json:"canvasContext,omitempty"`
	CallID        string              `json:"callId"`
	Token         string              `json:"token"`
	Done          bool                `json:"done,omitempty"`
	Release       bool                `json:"release,omitempty"`
}

// ClaimAgentToolPresentation 让一个画布页面认领、续期、完成或让出“把服务端生成结果落到画布”这件事；同一画布开着多个页面时只有一个负责。
// 完成时可以带上最新画布，让后续规划看到新节点的位置；页面离开时让出，重新打开的页面可以立刻接手。
func ClaimAgentToolPresentation(user model.AuthUser, runID string, request ClaimAgentToolPresentationRequest) (string, error) {
	if err := RequireOrganizationWrite(user); err != nil {
		return "", err
	}
	runID, callID, token := strings.TrimSpace(runID), strings.TrimSpace(request.CallID), strings.TrimSpace(request.Token)
	if runID == "" || callID == "" || token == "" || len(token) > 191 || token == model.AgentPresentationPending || token == model.AgentPresentationDone {
		return "", safeMessageError{message: "落地认领参数无效"}
	}
	canvasContext := ""
	if request.CanvasContext != nil {
		run, err := repository.GetAgentRun(user.OrganizationID, user.ID, runID)
		if err != nil {
			return "", err
		}
		canvas, err := normalizeAgentCanvasContext(*request.CanvasContext)
		if err != nil {
			return "", err
		}
		canvas.Autonomy = agentRunAutonomy(run)
		payload, err := json.Marshal(canvas)
		if err != nil || len(payload) > 16<<20 {
			return "", safeMessageError{message: "画布上下文过大"}
		}
		canvasContext = string(payload)
	}
	next := token
	if request.Done {
		next = model.AgentPresentationDone
	} else if request.Release {
		next = model.AgentPresentationPending
	}
	timestamp := time.Now().UTC()
	return repository.ClaimAgentToolPresentation(user.OrganizationID, user.ID, runID, callID, token, next, canvasContext, timestamp.Format(timestampLayout), timestamp.Add(-agentToolExecutionLease).Format(timestampLayout))
}

// StartAgentBackgroundToolWorker 定时接管没有执行者的后台工具步骤：服务重启或实例下线后，由存活的实例继续完成。
func StartAgentBackgroundToolWorker() {
	names := []string{}
	for _, definition := range agentToolRegistry {
		if definition.Background {
			names = append(names, definition.Name)
		}
	}
	go func() {
		for {
			steps, err := repository.ListOrphanedAgentToolSteps(names, time.Now().UTC().Add(-agentToolExecutionLease).Format(timestampLayout), 50)
			if err != nil {
				logWorkerError("agent_tool", "orphan_list_failed", err)
			}
			for _, step := range steps {
				startAgentBackgroundTool(step.OrganizationID, step.UserID, step.RunID, step.ToolCallID)
			}
			time.Sleep(agentBackgroundScanInterval)
		}
	}()
}

func startAgentBackgroundTool(organizationID, userID, runID, callID string) {
	if _, running := agentBackgroundTools.LoadOrStore(callID, struct{}{}); running {
		return
	}
	go func() {
		defer agentBackgroundTools.Delete(callID)
		runAgentBackgroundTool(organizationID, userID, runID, callID)
	}()
}

func runAgentBackgroundTool(organizationID, userID, runID, callID string) {
	claim := func() error {
		timestamp := time.Now().UTC()
		return repository.ClaimAgentToolExecution(organizationID, userID, runID, callID, agentBackgroundToken, timestamp.Format(timestampLayout), timestamp.Add(-agentToolExecutionLease).Format(timestampLayout))
	}
	// 认领失败说明步骤已经结束，或另一个实例正在执行。
	if claim() != nil {
		return
	}
	run, err := repository.GetAgentRun(organizationID, userID, runID)
	if err != nil {
		return
	}
	step, err := repository.GetAgentToolStep(organizationID, userID, runID, callID)
	if err != nil {
		return
	}
	user, err := agentRunUser(run)
	if err != nil {
		var unavailable safeMessageError
		if errors.As(err, &unavailable) {
			failWaitingAgentRun(run, "账号不可用，助手已停止")
		}
		return
	}
	timeout := agentGenerationTimeout
	if definition, _ := agentToolDefinitionFor(step.ToolName); definition.MediaCost == 0 {
		timeout = agentInspectionTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	go func() {
		ticker := time.NewTicker(agentToolExecutionLease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 租约丢失说明运行已取消、撤销或被其他实例接管，不再继续。
				if errors.Is(claim(), repository.ErrAgentToolExecutionClaimed) {
					cancel()
					return
				}
			}
		}
	}()
	result := executeAgentBackgroundTool(ctx, user, run, step)
	if errors.Is(ctx.Err(), context.Canceled) || claim() != nil {
		return
	}
	result.CallID, result.ExecutionToken = callID, agentBackgroundToken
	_, err = SubmitAgentToolResult(user, runID, result)
	if err != nil && result.Status == "success" && claim() == nil {
		// 结果没能写回时按失败收尾，避免这一步一直占着运行。
		_, err = SubmitAgentToolResult(user, runID, SubmitAgentToolResultRequest{CallID: callID, ExecutionToken: agentBackgroundToken, Status: "failed", Error: "工具结果保存失败"})
	}
	if err != nil {
		logWorkerError("agent_tool", "result_submit_failed", err, "run_id", runID, "call_id", callID, "tool", step.ToolName)
		failWaitingAgentRun(run, "工具结果保存失败")
	}
}

func executeAgentBackgroundTool(ctx context.Context, user model.AuthUser, run model.AgentRun, step model.AgentStep) (result SubmitAgentToolResultRequest) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logWorkerError("agent_tool", "executor_panicked", fmt.Errorf("%v", recovered), "run_id", run.ID, "call_id", step.ToolCallID, "tool", step.ToolName)
			result = SubmitAgentToolResultRequest{Status: "failed", Error: agentBackgroundFailedMessage}
		}
	}()
	failed := func(err error) SubmitAgentToolResultRequest {
		message := agentBackgroundFailedMessage
		var safe safeMessageError
		if errors.As(err, &safe) {
			message = safe.message
		} else if errors.Is(err, context.DeadlineExceeded) {
			message = "执行超时，已停止等待"
		} else {
			logWorkerError("agent_tool", "executor_failed", err, "run_id", run.ID, "call_id", step.ToolCallID, "tool", step.ToolName)
		}
		return SubmitAgentToolResultRequest{Status: "failed", Error: message}
	}
	steps, err := repository.ListCompletedAgentToolSteps(run.OrganizationID, run.UserID, run.ID)
	if err != nil {
		return failed(err)
	}
	authorization, err := agentRunNodeAuthorization(run)
	if err != nil {
		return failed(err)
	}
	arguments, _, err := decodeAgentToolArguments(step.ToolName, step.Input, authorization)
	if err != nil {
		return failed(err)
	}
	switch input := arguments.(type) {
	case imageInspectArguments:
		inspection := inspectAgentImages(ctx, user, run, steps, step.ToolCallID, input)
		return SubmitAgentToolResultRequest{Status: "success", Inspection: &inspection}
	case videoInspectArguments:
		inspection := inspectAgentVideo(ctx, user, run, steps, step.ToolCallID, input)
		return SubmitAgentToolResultRequest{Status: "success", Inspection: &inspection}
	}
	spec, generates, err := resolveAgentMediaToolSpec(run, steps, step.ToolName, step.ToolCallID, arguments)
	if err != nil || !generates {
		return failed(err)
	}
	storageKeys, err := runAgentGeneration(ctx, user, run, step.ToolCallID, spec)
	if err != nil {
		return failed(err)
	}
	if spec.Media.Type == "video" {
		return SubmitAgentToolResultRequest{Status: "success", Video: &AgentToolVideo{NodeID: spec.Media.NodeIDs[0], StorageKey: storageKeys[0]}}
	}
	images := make([]AgentToolImage, 0, len(spec.Media.NodeIDs))
	for i, nodeID := range spec.Media.NodeIDs[:min(len(spec.Media.NodeIDs), len(storageKeys))] {
		images = append(images, AgentToolImage{NodeID: nodeID, StorageKey: storageKeys[i]})
	}
	return SubmitAgentToolResultRequest{Status: "success", Images: images}
}

// resolveAgentMediaToolSpec 解析生成类工具的规格；验收等不产生媒体的工具返回 false。
func resolveAgentMediaToolSpec(run model.AgentRun, steps []model.AgentStep, name, callID string, arguments any) (agentMediaSpec, bool, error) {
	switch input := arguments.(type) {
	case imageGenerateArguments, imageEditArguments:
		spec, err := resolveAgentImageSpec(run, steps, callID, name, arguments)
		return spec, true, err
	case videoGenerateArguments:
		spec, err := resolveAgentVideoSpec(run, steps, callID, input)
		return spec, true, err
	}
	return agentMediaSpec{}, false, nil
}

// agentBackgroundToolMeta 是随 tool.call 下发的说明：工具由服务端执行；生成类工具附上页面占位和落地要用的规格，
// 规格解析失败时不附带，由执行者按失败回写。
func agentBackgroundToolMeta(run model.AgentRun, name, callID string, arguments any) map[string]any {
	meta := map[string]any{"executor": "server"}
	if spec, generates, err := resolveAgentMediaToolSpec(run, mustListAgentToolSteps(run), name, callID, arguments); generates && err == nil {
		meta["media"] = spec.Media
	}
	return meta
}

// runAgentGeneration 发起（或接上）一次生成并等到任务终态，返回归档后的存储键。
// 请求编号固定为 agent:{run}:{call}：重启后重新执行会接上已有任务，不会重复生成和扣费。
func runAgentGeneration(ctx context.Context, user model.AuthUser, run model.AgentRun, callID string, spec agentMediaSpec) ([]string, error) {
	requestID := "agent:" + run.ID + ":" + callID
	_, responded, err := repository.GetGenerationTaskByRequest(run.OrganizationID, run.UserID, requestID)
	if err != nil {
		return nil, err
	}
	var response <-chan []byte
	if !responded {
		body, contentType, err := spec.body(ctx, run.OrganizationID)
		if err != nil {
			return nil, err
		}
		response = requestAgentAI(ctx, user, spec.Path, contentType, requestID, body)
	}
	ticker := time.NewTicker(agentGenerationPollInterval)
	defer ticker.Stop()
	message := ""
	for {
		task, exists, err := repository.GetGenerationTaskByRequest(run.OrganizationID, run.UserID, requestID)
		if err != nil {
			return nil, err
		}
		switch {
		case exists && task.Status == model.GenerationTaskStatusSuccess:
			if len(task.StorageKeys) == 0 {
				return nil, safeMessageError{message: "生成结果未能保存到工作区"}
			}
			return task.StorageKeys, nil
		case exists && task.Status == model.GenerationTaskStatusFailed:
			return nil, safeMessageError{message: firstPricingNonEmpty(task.ErrorMessage, message, "生成失败")}
		case !exists && responded:
			return nil, safeMessageError{message: firstPricingNonEmpty(message, "生成请求失败")}
		}
		// 接口已返回但任务仍在进行（视频、延后完成的图片）时，由生成任务维护进程查询上游并归档，这里只等任务终态。
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case body := <-response:
			message, responded, response = agentAIResponseError(body), true, nil
		case <-ticker.C:
		}
	}
}

// requestAgentAI 在后台协程里调用生成接口。接口内部的上游请求不随 ctx 取消，调用方按自己的时限决定等多久。
func requestAgentAI(ctx context.Context, user model.AuthUser, path, contentType, requestID string, body []byte) <-chan []byte {
	done := make(chan []byte, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				logWorkerError("agent_tool", "ai_request_panicked", fmt.Errorf("%v", recovered), "request_id", requestID)
				done <- []byte(`{"code":1,"msg":"` + agentBackgroundFailedMessage + `"}`)
			}
		}()
		done <- AgentAIRequester(ctx, user, path, contentType, requestID, body)
	}()
	return done
}

// agentAIResponseError 读出生成接口返回的失败说明；没有失败说明时返回空串。
func agentAIResponseError(body []byte) string {
	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(body, &payload) != nil || payload.Code == 0 {
		return ""
	}
	return payload.Msg
}

// agentRunUser 取回运行所属的用户供后台执行使用；账号被停用后不再继续。
func agentRunUser(run model.AgentRun) (model.AuthUser, error) {
	user, ok, err := repository.GetUserByID(run.UserID)
	if err != nil {
		return model.AuthUser{}, err
	}
	if !ok || user.Status == model.UserStatusBan {
		return model.AuthUser{}, safeMessageError{message: "账号不可用"}
	}
	normalizeUserDefaults(&user)
	auth := model.PublicUser(user)
	auth.OrganizationID = run.OrganizationID
	return auth, nil
}

// failWaitingAgentRun 让一个等待工具结果的运行立即失败。
func failWaitingAgentRun(run model.AgentRun, message string) {
	timestamp := now()
	payload, _ := json.Marshal(map[string]string{"error": message})
	_, _, _ = repository.ExpireWaitingAgentRun(run.OrganizationID, run.UserID, run.ID, newID("agent-event"), message, string(payload), timestamp, timestamp)
	_ = repository.FinalizeAgentPlan(run.OrganizationID, run.UserID, run.ID, message, timestamp)
}
