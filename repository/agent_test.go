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
	if err := testDB.Create(&run).Error; err != nil { t.Fatal(err) }
	if err := testDB.Create(&step).Error; err != nil { t.Fatal(err) }

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
	if _, err := SaveAgentMemory(base); err != nil { t.Fatal(err) }
	base.Content, base.UpdatedAt = "插画", workspaceTestFuture
	if _, err := SaveAgentMemory(base); err != nil { t.Fatal(err) }
	memories, err := ListActiveAgentMemories("organization-a", "user-a", "project-a", 10)
	if err != nil || len(memories) != 1 || memories[0].Content != "插画" { t.Fatalf("updated memories = %#v err=%v", memories, err) }
	if _, err := SaveAgentMemory(model.AgentMemory{ID: "memory-2", OrganizationID: "organization-a", UserID: "user-a", ProjectID: "", Kind: model.AgentMemoryKindFact, Key: "brand-name", Content: "道生画境", Confidence: 0.9, Status: model.AgentMemoryStatusActive, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}); err != nil { t.Fatal(err) }
	memories, err = ListActiveAgentMemories("organization-a", "user-a", "project-a", 10)
	if err != nil || len(memories) != 2 { t.Fatalf("project and user memories = %#v err=%v", memories, err) }
	if _, err := ForgetAgentMemory("organization-a", "user-a", "project-a", "visual-style", workspaceTestFuture); err != nil { t.Fatal(err) }
	memories, err = ListActiveAgentMemories("organization-a", "user-a", "project-a", 10)
	if err != nil || len(memories) != 1 || memories[0].Key != "brand-name" { t.Fatalf("forgotten memory still active = %#v err=%v", memories, err) }
	_ = testDB
}
