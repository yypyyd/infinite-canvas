package repository

import (
	"testing"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestParallelAgentToolStepsResumeOnlyAfterAllResults(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	run := model.AgentRun{ID: "run-parallel", OrganizationID: "organization-a", UserID: "user-a", Status: model.AgentRunStatusRunning, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	completion := model.AgentStep{ID: "completion-parallel", OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeCompletion, Status: model.AgentStepStatusRunning, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	if err := testDB.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&completion).Error; err != nil {
		t.Fatal(err)
	}
	subscription := SubscribeAgentRun(run.ID)
	defer subscription.Close()
	steps := []model.AgentStep{
		{ID: "step-parallel-a", OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeTool, Status: model.AgentStepStatusRunning, ToolCallID: "call-a", ToolName: "canvas.add_text", CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow},
		{ID: "step-parallel-b", OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeTool, Status: model.AgentStepStatusRunning, ToolCallID: "call-b", ToolName: "canvas.connect", CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow},
	}
	events := []model.AgentEvent{
		{ID: "event-call-a", Type: model.AgentEventToolCall, Payload: `{"callId":"call-a"}`, CreatedAt: workspaceTestNow},
		{ID: "event-call-b", Type: model.AgentEventToolCall, Payload: `{"callId":"call-b"}`, CreatedAt: workspaceTestNow},
	}
	if err := WaitAgentRunForTools(run.OrganizationID, run.UserID, run.ID, "", steps, workspaceTestNow, events...); err != nil {
		t.Fatal(err)
	}
	select {
	case <-subscription.Wake:
	case <-time.After(time.Second):
		t.Fatal("committed events should wake subscribers")
	}
	saved, err := ListAgentEvents(run.OrganizationID, run.UserID, run.ID, 0, 10)
	if err != nil || len(saved) != 2 || saved[1].Sequence != 2 {
		t.Fatalf("events = %#v err=%v", saved, err)
	}
	for _, step := range steps {
		if err := ClaimAgentToolExecution(run.OrganizationID, run.UserID, run.ID, step.ToolCallID, "token", workspaceTestNow, workspaceTestOld); err != nil {
			t.Fatal(err)
		}
	}
	next := model.AgentStep{ID: "completion-next", OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeCompletion, Status: model.AgentStepStatusRunning, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	completed := model.AgentEvent{ID: "event-done-a", Type: model.AgentEventToolCompleted, Payload: `{}`, CreatedAt: workspaceTestNow}
	failed := model.AgentEvent{ID: "event-failed", Type: model.AgentEventRunFailed, Payload: `{}`, CreatedAt: workspaceTestNow}
	updated, _, resume, err := SubmitAgentToolResult(run.OrganizationID, run.UserID, run.ID, "call-a", "token", `{"status":"success"}`, "", workspaceTestNow, true, false, completed, failed, next)
	if err != nil || resume || updated.Status != model.AgentRunStatusWaitingTool {
		t.Fatalf("first parallel result: run=%s resume=%v err=%v", updated.Status, resume, err)
	}
	completed.ID = "event-done-b"
	updated, _, resume, err = SubmitAgentToolResult(run.OrganizationID, run.UserID, run.ID, "call-b", "token", `{"status":"success"}`, "", workspaceTestNow, true, false, completed, failed, next)
	if err != nil || !resume || updated.Status != model.AgentRunStatusRunning {
		t.Fatalf("last parallel result: run=%s resume=%v err=%v", updated.Status, resume, err)
	}
}

func TestPublishAgentRunEventReachesOnlyThatRun(t *testing.T) {
	first := SubscribeAgentRun("run-x")
	second := SubscribeAgentRun("run-y")
	defer first.Close()
	defer second.Close()
	PublishAgentRunEvent("run-x", model.AgentEvent{ID: "ephemeral", Type: model.AgentEventMessageStream, Payload: `{"content":"你"}`, Sequence: 9})
	select {
	case event := <-first.Events:
		if event.RunID != "run-x" || event.Sequence != 0 {
			t.Fatalf("event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive event")
	}
	select {
	case event := <-second.Events:
		t.Fatalf("other run received %#v", event)
	default:
	}
	first.Close()
	PublishAgentRunEvent("run-x", model.AgentEvent{ID: "after-close"})
}
