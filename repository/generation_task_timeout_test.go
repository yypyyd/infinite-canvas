package repository

import (
	"fmt"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestListStaleRunningGenerationTasks(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	tasks := []model.GenerationTask{
		{ID: "stale", OrganizationID: "organization-a", UserID: "user-a", RequestID: "request-stale", Status: model.GenerationTaskStatusRunning, CreatedAt: "2026-08-11T09:29:59Z", UpdatedAt: "2026-08-11T09:29:59Z"},
		{ID: "boundary", OrganizationID: "organization-a", UserID: "user-a", RequestID: "request-boundary", Status: model.GenerationTaskStatusRunning, CreatedAt: "2026-08-11T09:30:00Z", UpdatedAt: "2026-08-11T09:30:00Z"},
		{ID: "recent", OrganizationID: "organization-a", UserID: "user-a", RequestID: "request-recent", Status: model.GenerationTaskStatusRunning, CreatedAt: "2026-08-11T09:30:01Z", UpdatedAt: "2026-08-11T09:30:01Z"},
		{ID: "settled", OrganizationID: "organization-a", UserID: "user-a", RequestID: "request-settled", Status: model.GenerationTaskStatusSuccess, CreatedAt: "2026-08-11T09:00:00Z", UpdatedAt: "2026-08-11T09:10:00Z"},
		{ID: "batch", OrganizationID: "organization-a", UserID: "user-a", RequestID: "request-batch", BatchJobID: "job-a", BatchItemID: "item-a", Status: model.GenerationTaskStatusRunning, CreatedAt: "2026-08-11T09:00:00Z", UpdatedAt: "2026-08-11T09:00:00Z"},
	}
	if err := testDB.Create(&tasks).Error; err != nil {
		t.Fatalf("create generation tasks: %v", err)
	}

	items, err := ListStaleRunningGenerationTasks("2026-08-11T09:30:00Z", "2026-08-11T09:31:00Z", 100)
	if err != nil {
		t.Fatalf("list stale generation tasks: %v", err)
	}
	if len(items) != 2 || items[0].ID != "stale" || items[1].ID != "boundary" {
		t.Fatalf("unexpected stale generation tasks: %#v", items)
	}
}

func TestGenerationTaskReconcileLease(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	item := model.GenerationTask{ID: "leased", OrganizationID: "org-a", UserID: "user-a", RequestID: "request", Status: model.GenerationTaskStatusRunning, CreatedAt: workspaceTestOld}
	if err := testDB.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan model.GenerationTask, 2)
	errors := make(chan error, 2)
	for i := range 2 {
		go func() {
			<-start
			task, claimed, err := ClaimGenerationTaskReconcile(item.ID, fmt.Sprintf("worker-%d", i), workspaceTestNow, workspaceTestFuture)
			if !claimed {
				task = model.GenerationTask{}
			}
			results <- task
			errors <- err
		}()
	}
	close(start)
	var owner model.GenerationTask
	claimed := 0
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
		if task := <-results; task.ID != "" {
			claimed++
			owner = task
		}
	}
	if claimed != 1 {
		t.Fatalf("expected one lease owner, got %d", claimed)
	}
	items, err := ListStaleRunningGenerationTasks(workspaceTestNow, workspaceTestNow, 100)
	if err != nil || len(items) != 0 {
		t.Fatalf("leased task should be excluded: %#v, %v", items, err)
	}
	newOwner, ok, err := ClaimGenerationTaskReconcile(item.ID, "takeover", workspaceTestFuture, "2026-07-25T12:00:00.000Z")
	if err != nil || !ok {
		t.Fatalf("expired lease should be reclaimed: %v", err)
	}
	if err := ReleaseGenerationTaskReconcile(item.ID, owner.ReconcileLeaseToken, workspaceTestFuture); err != nil {
		t.Fatal(err)
	}
	owner.UpdatedAt = workspaceTestFuture
	if err := CompleteGenerationTask(owner); err != nil {
		t.Fatal(err)
	}
	if err := UpdateGenerationTaskRecovery(item.ID, "upstream", `{}`, nil, workspaceTestFuture, owner.ReconcileLeaseToken); err == nil {
		t.Fatal("old owner must not write recovery results")
	}
	if _, err := FailGenerationTaskAndRefund(owner, nil); err == nil {
		t.Fatal("old owner must not refund")
	}
	var saved model.GenerationTask
	if err := testDB.First(&saved, "id = ?", item.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Status != model.GenerationTaskStatusRunning || saved.ReconcileLeaseToken != newOwner.ReconcileLeaseToken {
		t.Fatalf("old owner changed the new lease: %#v", saved)
	}
	if err := ReleaseGenerationTaskReconcile(item.ID, newOwner.ReconcileLeaseToken, "2026-07-25T11:01:00.000Z"); err != nil {
		t.Fatal(err)
	}
	items, err = ListStaleRunningGenerationTasks(workspaceTestFuture, workspaceTestFuture, 100)
	if err != nil || len(items) != 0 {
		t.Fatalf("retry delay should be respected: %#v, %v", items, err)
	}
}
