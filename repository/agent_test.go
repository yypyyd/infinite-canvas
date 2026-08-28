package repository

import (
	"errors"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestAnswerAgentAskUserRejectsConflictingReplay(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	run := model.AgentRun{ID: "run-ask", OrganizationID: "organization-a", UserID: "user-a", Status: model.AgentRunStatusWaitingConfirmation, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	step := model.AgentStep{ID: "step-ask", OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeTool, Status: model.AgentStepStatusRunning, ToolCallID: "call-ask", ToolName: "agent.ask_user", CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	if err := testDB.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&step).Error; err != nil {
		t.Fatal(err)
	}

	answer := `{"status":"success","answer":"写实风格"}`
	completion := model.AgentStep{ID: "step-completion", OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeCompletion, Status: model.AgentStepStatusRunning, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	event := model.AgentEvent{ID: "event-completed", Type: model.AgentEventToolCompleted, Payload: `{}`, CreatedAt: workspaceTestNow}
	if _, resume, err := AnswerAgentAskUser(run.OrganizationID, run.UserID, run.ID, step.ToolCallID, "approved", answer, workspaceTestNow, event, completion); err != nil || !resume {
		t.Fatalf("first answer: resume=%v err=%v", resume, err)
	}
	if _, resume, err := AnswerAgentAskUser(run.OrganizationID, run.UserID, run.ID, step.ToolCallID, "approved", answer, workspaceTestNow, event, completion); err != nil || resume {
		t.Fatalf("idempotent answer: resume=%v err=%v", resume, err)
	}
	conflict := `{"status":"success","answer":"插画风格"}`
	if _, _, err := AnswerAgentAskUser(run.OrganizationID, run.UserID, run.ID, step.ToolCallID, "approved", conflict, workspaceTestNow, event, completion); !errors.Is(err, ErrAgentToolResultConflict) {
		t.Fatalf("conflicting answer error = %v", err)
	}
}

func TestAgentMemoryIsScopedAndOverwritable(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	base := model.AgentMemory{ID: "memory-1", OrganizationID: "organization-a", UserID: "user-a", ProjectID: "project-a", Kind: model.AgentMemoryKindPreference, Key: "visual-style", Content: "写实", Confidence: 0.8, Status: model.AgentMemoryStatusActive, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	if _, err := SaveAgentMemory(base); err != nil {
		t.Fatal(err)
	}
	base.Content, base.UpdatedAt = "插画", workspaceTestFuture
	if _, err := SaveAgentMemory(base); err != nil {
		t.Fatal(err)
	}
	memories, err := ListActiveAgentMemories("organization-a", "user-a", "project-a", 10)
	if err != nil || len(memories) != 1 || memories[0].Content != "插画" {
		t.Fatalf("updated memories = %#v err=%v", memories, err)
	}
	if _, err := SaveAgentMemory(model.AgentMemory{ID: "memory-2", OrganizationID: "organization-a", UserID: "user-a", ProjectID: "", Kind: model.AgentMemoryKindFact, Key: "brand-name", Content: "道生画境", Confidence: 0.9, Status: model.AgentMemoryStatusActive, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}); err != nil {
		t.Fatal(err)
	}
	memories, err = ListActiveAgentMemories("organization-a", "user-a", "project-a", 10)
	if err != nil || len(memories) != 2 {
		t.Fatalf("project and user memories = %#v err=%v", memories, err)
	}
	if _, err := ForgetAgentMemory("organization-a", "user-a", "project-a", "visual-style", workspaceTestFuture); err != nil {
		t.Fatal(err)
	}
	memories, err = ListActiveAgentMemories("organization-a", "user-a", "project-a", 10)
	if err != nil || len(memories) != 1 || memories[0].Key != "brand-name" {
		t.Fatalf("forgotten memory still active = %#v err=%v", memories, err)
	}
	_ = testDB
}

func TestClaimAgentToolExecutionRenewsOwnerAndAllowsExpiredTakeover(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	run := model.AgentRun{ID: "run-lease", OrganizationID: "organization-a", UserID: "user-a", Status: model.AgentRunStatusWaitingTool, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	step := model.AgentStep{ID: "step-lease", OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeTool, Status: model.AgentStepStatusRunning, ToolCallID: "call-lease", ToolName: "canvas.add_text", CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	if err := testDB.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&step).Error; err != nil {
		t.Fatal(err)
	}
	if err := ClaimAgentToolExecution(run.OrganizationID, run.UserID, run.ID, step.ToolCallID, "token-a", workspaceTestNow, workspaceTestOld); err != nil {
		t.Fatal(err)
	}
	if err := ClaimAgentToolExecution(run.OrganizationID, run.UserID, run.ID, step.ToolCallID, "token-a", workspaceTestFuture, workspaceTestOld); err != nil {
		t.Fatalf("owner renewal: %v", err)
	}
	if err := ClaimAgentToolExecution(run.OrganizationID, run.UserID, run.ID, step.ToolCallID, "token-b", workspaceTestFuture, workspaceTestOld); !errors.Is(err, ErrAgentToolExecutionClaimed) {
		t.Fatalf("active lease takeover error = %v", err)
	}
	if err := ClaimAgentToolExecution(run.OrganizationID, run.UserID, run.ID, step.ToolCallID, "token-b", workspaceTestFuture, workspaceTestFuture); err != nil {
		t.Fatalf("expired lease takeover: %v", err)
	}
}

func TestSubmitAgentToolResultIsIdempotentAndRejectsConflicts(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	run := model.AgentRun{ID: "run-result", OrganizationID: "organization-a", UserID: "user-a", Status: model.AgentRunStatusWaitingTool, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	step := model.AgentStep{ID: "step-result", OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeTool, Status: model.AgentStepStatusRunning, ToolCallID: "call-result", ToolName: "canvas.add_text", ExecutionToken: "token-a", CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	if err := testDB.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&step).Error; err != nil {
		t.Fatal(err)
	}
	output := `{"callId":"call-result","status":"success","nodeId":"node-a","placement":"center"}`
	completion := model.AgentStep{ID: "completion-result", OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeCompletion, Status: model.AgentStepStatusRunning, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	completedEvent := model.AgentEvent{ID: "event-result-completed", Type: model.AgentEventToolCompleted, Payload: `{}`, CreatedAt: workspaceTestNow}
	failedEvent := model.AgentEvent{ID: "event-result-failed", Type: model.AgentEventRunFailed, Payload: `{}`, CreatedAt: workspaceTestNow}
	if _, _, resume, err := SubmitAgentToolResult(run.OrganizationID, run.UserID, run.ID, step.ToolCallID, "token-a", output, "", workspaceTestNow, true, false, completedEvent, failedEvent, completion); err != nil || !resume {
		t.Fatalf("first result: resume=%v err=%v", resume, err)
	}
	if _, _, resume, err := SubmitAgentToolResult(run.OrganizationID, run.UserID, run.ID, step.ToolCallID, "token-a", output, "", workspaceTestNow, true, false, completedEvent, failedEvent, completion); err != nil || resume {
		t.Fatalf("idempotent result: resume=%v err=%v", resume, err)
	}
	conflict := `{"callId":"call-result","status":"success","nodeId":"node-b","placement":"center"}`
	if _, _, _, err := SubmitAgentToolResult(run.OrganizationID, run.UserID, run.ID, step.ToolCallID, "token-a", conflict, "", workspaceTestNow, true, false, completedEvent, failedEvent, completion); !errors.Is(err, ErrAgentToolResultConflict) {
		t.Fatalf("conflicting result error = %v", err)
	}
}

func TestRetryAgentStepReopensOnlyFailedRun(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	run := model.AgentRun{ID: "run-retry", OrganizationID: "organization-a", UserID: "user-a", Status: model.AgentRunStatusFailed, Error: "生成失败", CompletedAt: workspaceTestNow, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	source := model.AgentStep{ID: "step-retry-source", OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeTool, Status: model.AgentStepStatusFailed, ToolCallID: "call-source", ToolName: "image.generate", Input: `{"prompt":"红鞋","count":1}`, Error: "生成失败", CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	if err := testDB.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	retry := model.AgentStep{ID: "step-retry", OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeTool, Status: model.AgentStepStatusRunning, ToolCallID: "call-retry", ToolName: source.ToolName, Input: source.Input, CreatedAt: workspaceTestFuture, UpdatedAt: workspaceTestFuture}
	event := model.AgentEvent{ID: "event-retry", Type: model.AgentEventToolCall, Payload: `{"callId":"call-retry"}`, CreatedAt: workspaceTestFuture}
	updated, err := RetryAgentStep(run.OrganizationID, run.UserID, run.ID, source.ToolCallID, workspaceTestFuture, retry, event)
	if err != nil || updated.Status != model.AgentRunStatusWaitingTool || updated.Error != "" || updated.CompletedAt != "" {
		t.Fatalf("retry run = %#v err=%v", updated, err)
	}
	if _, err := RetryAgentStep(run.OrganizationID, run.UserID, run.ID, source.ToolCallID, workspaceTestFuture, retry, event); !errors.Is(err, ErrAgentStepNotRetryable) {
		t.Fatalf("duplicate retry error = %v", err)
	}
}

func TestRevertAgentRunAppendsAllEventsIdempotently(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	run := model.AgentRun{ID: "run-revert-all", OrganizationID: "organization-a", UserID: "user-a", Status: model.AgentRunStatusCompleted, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	steps := []model.AgentStep{
		{ID: "step-revert-a", OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeTool, Status: model.AgentStepStatusCompleted, ToolCallID: "call-a", ToolName: "image.generate", CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow},
		{ID: "step-revert-b", OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeTool, Status: model.AgentStepStatusCompleted, ToolCallID: "call-b", ToolName: "canvas.arrange", CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow},
	}
	if err := testDB.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&steps).Error; err != nil {
		t.Fatal(err)
	}
	events := []model.AgentEvent{
		{ID: "event-revert-a", Type: model.AgentEventToolReverted, Payload: `{"callId":"call-a","name":"image.generate"}`, CreatedAt: workspaceTestFuture},
		{ID: "event-revert-b", Type: model.AgentEventToolReverted, Payload: `{"callId":"call-b","name":"canvas.arrange"}`, CreatedAt: workspaceTestFuture},
	}
	if _, err := RevertAgentRun(run.OrganizationID, run.UserID, run.ID, []string{"call-a", "call-b"}, workspaceTestFuture, events, model.AgentEvent{}); err != nil {
		t.Fatal(err)
	}
	if _, err := RevertAgentRun(run.OrganizationID, run.UserID, run.ID, []string{"call-a", "call-b"}, workspaceTestFuture, events, model.AgentEvent{}); err != nil {
		t.Fatalf("idempotent revert: %v", err)
	}
	var count int64
	if err := testDB.Model(&model.AgentEvent{}).Where("run_id = ? AND type = ?", run.ID, model.AgentEventToolReverted).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("reverted event count = %d err=%v", count, err)
	}
}

func TestAgentPlanRevisionsAndProgressAreDurable(t *testing.T) {
	setupUserWorkspaceTestDB(t)
	first, err := ReplaceAgentPlan("organization-a", "user-a", "run-plan", "call-plan-1", []string{"生成图片", "验收图片"}, workspaceTestNow)
	if err != nil || len(first) != 2 || first[0].Revision != 1 {
		t.Fatalf("create first plan: %#v %v", first, err)
	}
	if err := StartNextAgentPlanStep("organization-a", "user-a", "run-plan", "call-image", "image.generate", workspaceTestNow); err != nil {
		t.Fatalf("start plan: %v", err)
	}
	if err := FinishAgentPlanStep("organization-a", "user-a", "run-plan", "call-image", true, "", workspaceTestFuture); err != nil {
		t.Fatalf("finish plan: %v", err)
	}
	second, err := ReplaceAgentPlan("organization-a", "user-a", "run-plan", "call-plan-2", []string{"重新生成"}, workspaceTestFuture)
	if err != nil || len(second) != 1 || second[0].Revision != 2 {
		t.Fatalf("create revised plan: %#v %v", second, err)
	}
	all, err := ListAgentPlanSteps("organization-a", "user-a", "run-plan")
	if err != nil || len(all) != 3 || all[0].Status != model.AgentPlanStepStatusCompleted || all[1].Status != model.AgentPlanStepStatusSkipped {
		t.Fatalf("unexpected plan history: %#v %v", all, err)
	}
}

func TestAgentFeedbackAdjustsOnlyExperienceMemoryByDelta(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	run := model.AgentRun{ID: "run-feedback", OrganizationID: "organization-a", UserID: "user-a", Status: model.AgentRunStatusCompleted, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	memories := []model.AgentMemory{
		{ID: "experience", OrganizationID: run.OrganizationID, UserID: run.UserID, ProjectID: "project-a", Kind: model.AgentMemoryKindExperience, Key: "layout", Content: "紧凑排列", SourceRunID: run.ID, Confidence: 0.5, Status: model.AgentMemoryStatusActive, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow},
		{ID: "preference", OrganizationID: run.OrganizationID, UserID: run.UserID, ProjectID: "project-a", Kind: model.AgentMemoryKindPreference, Key: "style", Content: "写实", SourceRunID: run.ID, Confidence: 0.8, Status: model.AgentMemoryStatusActive, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow},
	}
	if err := testDB.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&memories).Error; err != nil {
		t.Fatal(err)
	}
	feedback := model.AgentFeedback{ID: "feedback", OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Signal: model.AgentFeedbackSignalAccepted, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	if _, adjusted, err := SaveAgentFeedback(feedback); err != nil || adjusted != 1 {
		t.Fatalf("save accepted feedback: %d %v", adjusted, err)
	}
	feedback.Signal, feedback.UpdatedAt = model.AgentFeedbackSignalDeleted, workspaceTestFuture
	if _, adjusted, err := SaveAgentFeedback(feedback); err != nil || adjusted != 1 {
		t.Fatalf("replace feedback: %d %v", adjusted, err)
	}
	var experience, preference model.AgentMemory
	_ = testDB.First(&experience, "id = ?", "experience").Error
	_ = testDB.First(&preference, "id = ?", "preference").Error
	if experience.Confidence < 0.399 || experience.Confidence > 0.401 || preference.Confidence != 0.8 {
		t.Fatalf("unexpected confidences: %v %v", experience.Confidence, preference.Confidence)
	}
}
