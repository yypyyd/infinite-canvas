package repository

import (
	"errors"
	"fmt"
	"github.com/yypyyd/infinite-canvas/model"
	"testing"
)

func TestAgentMemoryCommitRequiresActiveConfirmedLease(t *testing.T) {
	for _, scenario := range []string{"wrong-token", "expired", "cancelled", "unconfirmed", "rollback", "success"} {
		t.Run(scenario, func(t *testing.T) {
			db := setupUserWorkspaceTestDB(t)
			run := model.AgentRun{ID: "run", OrganizationID: "org", UserID: "user", Status: model.AgentRunStatusWaitingTool, Context: "old", CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
			step := model.AgentStep{ID: "step", OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeTool, ToolName: "agent.remember", ToolCallID: "call", Status: model.AgentStepStatusRunning, Confirmation: "approved", ExecutionToken: "token", ExecutionAt: workspaceTestNow}
			token := "token"
			switch scenario {
			case "wrong-token":
				token = "other"
			case "expired":
				step.ExecutionAt = workspaceTestOld
			case "cancelled":
				run.Status = model.AgentRunStatusCancelled
			case "unconfirmed":
				step.Confirmation = ""
			}
			if err := db.Create(&run).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&step).Error; err != nil {
				t.Fatal(err)
			}
			completion := model.AgentStep{ID: "completion", OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeCompletion, Status: model.AgentStepStatusRunning}
			if scenario == "rollback" {
				if err := db.Create(&completion).Error; err != nil {
					t.Fatal(err)
				}
			}
			memory := model.AgentMemory{ID: "memory", OrganizationID: run.OrganizationID, UserID: run.UserID, ProjectID: "project", Key: "style", Content: "warm", Status: model.AgentMemoryStatusActive, Confidence: 1, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
			event := model.AgentEvent{ID: "event", Type: model.AgentEventToolCompleted, Payload: "{}", CreatedAt: workspaceTestNow}
			commit := AgentToolCommit{Memory: &memory, Context: "new"}
			_, _, resume, err := SubmitAgentToolResult(run.OrganizationID, run.UserID, run.ID, step.ToolCallID, token, "receipt", "", workspaceTestNow, true, false, event, model.AgentEvent{}, completion, commit)
			var count int64
			if countErr := db.Model(&model.AgentMemory{}).Count(&count).Error; countErr != nil {
				t.Fatal(countErr)
			}
			saved, getErr := GetAgentRun(run.OrganizationID, run.UserID, run.ID)
			if getErr != nil {
				t.Fatal(getErr)
			}
			if scenario != "success" {
				if err == nil || count != 0 || saved.Context != "old" {
					t.Fatalf("rejected commit mutated state: count=%d context=%q err=%v", count, saved.Context, err)
				}
				return
			}
			if err != nil || !resume || count != 1 || saved.Context != "new" {
				t.Fatalf("commit failed: %v", err)
			}
			if err := db.Model(&memory).Update("content", "manual correction").Error; err != nil {
				t.Fatal(err)
			}
			if _, _, resume, err = SubmitAgentToolResult(run.OrganizationID, run.UserID, run.ID, step.ToolCallID, token, "receipt", "", workspaceTestFuture, true, false, event, model.AgentEvent{}, completion, commit); err != nil || resume {
				t.Fatalf("replay: %v", err)
			}
			if err := db.First(&memory, "id = ?", memory.ID).Error; err != nil {
				t.Fatal(err)
			}
			if memory.Content != "manual correction" {
				t.Fatal("replay overwrote user memory")
			}
		})
	}
}

func TestAgentWaitingRunHonorsHeartbeat(t *testing.T) {
	db := setupUserWorkspaceTestDB(t)
	run := model.AgentRun{ID: "run", OrganizationID: "org", UserID: "user", Status: model.AgentRunStatusWaitingTool, UpdatedAt: workspaceTestOld}
	step := model.AgentStep{ID: "step", OrganizationID: run.OrganizationID, UserID: run.UserID, RunID: run.ID, Type: model.AgentStepTypeTool, Status: model.AgentStepStatusRunning, ExecutionToken: "active", ExecutionAt: workspaceTestFuture}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&step).Error; err != nil {
		t.Fatal(err)
	}
	if _, expired, err := ExpireWaitingAgentRun(run.OrganizationID, run.UserID, run.ID, "event", "timeout", "{}", workspaceTestFuture, workspaceTestNow); err != nil || expired {
		t.Fatalf("active tool expired: %v", err)
	}
	if err := db.Model(&step).Update("execution_at", workspaceTestOld).Error; err != nil {
		t.Fatal(err)
	}
	if _, expired, err := ExpireWaitingAgentRun(run.OrganizationID, run.UserID, run.ID, "event", "timeout", "{}", workspaceTestFuture, workspaceTestNow); err != nil || !expired {
		t.Fatalf("abandoned tool not expired: %v", err)
	}
}

func TestAgentCreditCapIsAtomicAndReplayDoesNotCharge(t *testing.T) {
	db := setupUserWorkspaceTestDB(t)
	user := model.User{ID: "user", Username: "agent-budget-user", AffCode: "agent-budget-user", Credits: 100, Status: model.UserStatusActive}
	run := model.AgentRun{ID: "agent-run-budget", OrganizationID: "org", UserID: user.ID, Status: model.AgentRunStatusWaitingTool, MaxCredits: 10}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	create := func(index int) error {
		id := fmt.Sprintf("task-%d", index)
		task := model.GenerationTask{ID: id, OrganizationID: run.OrganizationID, UserID: run.UserID, RequestID: "agent:" + run.ID + ":" + id, Credits: 7, CreditSource: model.CreditSourcePersonal, Status: model.GenerationTaskStatusRunning, CreatedAt: workspaceTestNow}
		_, err := CreateGenerationTaskWithCharge(task, &model.CreditLog{ID: "log-" + id, UserID: user.ID, Amount: -7}, run.ID)
		return err
	}
	errorsCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func(index int) { errorsCh <- create(index) }(i)
	}
	successes, rejected := 0, 0
	for i := 0; i < 2; i++ {
		err := <-errorsCh
		if err == nil {
			successes++
		} else if errors.Is(err, ErrAgentCreditBudgetExceeded) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || rejected != 1 {
		t.Fatalf("success=%d rejected=%d", successes, rejected)
	}
	var task model.GenerationTask
	if err := db.First(&task).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := CreateGenerationTaskWithCharge(task, &model.CreditLog{ID: "duplicate"}, run.ID); !errors.Is(err, ErrGenerationTaskRequestConflict) {
		t.Fatalf("replay error: %v", err)
	}
	if err := db.First(&user, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if user.Credits != 93 {
		t.Fatalf("unexpected balance %d", user.Credits)
	}
	if err := db.Model(&run).Update("status", model.AgentRunStatusCancelled).Error; err != nil {
		t.Fatal(err)
	}
	if err := create(3); !errors.Is(err, ErrAgentRunNotActive) {
		t.Fatalf("cancelled run charged: %v", err)
	}
}

func TestAgentRecoveryAndMemoryManagementAreOwnerScoped(t *testing.T) {
	db := setupUserWorkspaceTestDB(t)
	for i, owner := range []struct{ org, user, project string }{{"org", "user", "project"}, {"org", "user", "other-project"}, {"other-org", "user", "project"}, {"org", "other-user", "project"}} {
		id := fmt.Sprintf("scope-%d", i)
		session := model.AgentSession{ID: id, OrganizationID: owner.org, UserID: owner.user, ProjectID: owner.project}
		run := model.AgentRun{ID: id, SessionID: id, OrganizationID: owner.org, UserID: owner.user, Status: model.AgentRunStatusWaitingConfirmation}
		memory := model.AgentMemory{ID: id, OrganizationID: owner.org, UserID: owner.user, ProjectID: owner.project, Key: "style", Content: "warm", Status: model.AgentMemoryStatusActive}
		for _, record := range []any{&session, &run, &memory} {
			if err := db.Create(record).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	runs, err := ListActiveAgentRuns("org", "user", "project")
	if err != nil || len(runs) != 1 || runs[0].ID != "scope-0" {
		t.Fatalf("recovery scope: %#v %v", runs, err)
	}
	items, total, err := ListManagedAgentMemories("org", "user", "project", model.Query{})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("memory scope: %#v %v", items, err)
	}
	if _, err := GetOwnedAgentMemory("org", "other-user", "scope-0"); err == nil {
		t.Fatal("foreign memory readable")
	}
	if err := UpdateOwnedAgentMemory("org", "other-user", "scope-0", "malicious", workspaceTestNow, false); err != nil {
		t.Fatal(err)
	}
	item, err := GetOwnedAgentMemory("org", "user", "scope-0")
	if err != nil || item.Content != "warm" {
		t.Fatalf("foreign memory changed: %#v %v", item, err)
	}
}
