package repository

import (
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

	items, err := ListStaleRunningGenerationTasks("2026-08-11T09:30:00Z", 100)
	if err != nil {
		t.Fatalf("list stale generation tasks: %v", err)
	}
	if len(items) != 2 || items[0].ID != "stale" || items[1].ID != "boundary" {
		t.Fatalf("unexpected stale generation tasks: %#v", items)
	}
}
