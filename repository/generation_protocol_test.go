package repository

import (
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestGenerationProtocolSnapshotIsSavedOnlyOnce(t *testing.T) {
	db := setupUserWorkspaceTestDB(t)
	task := model.GenerationTask{ID: "protocol", OrganizationID: "org", UserID: "user", RequestID: "protocol", Status: model.GenerationTaskStatusRunning, CreatedAt: workspaceTestNow}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := &model.GenerationProtocolSnapshot{ChannelID: "original", BaseURL: "https://original.example.com", Operation: "image.generation", DeadlineAt: workspaceTestFuture}
	if err := SaveGenerationProtocol(task, snapshot, workspaceTestNow); err != nil {
		t.Fatal(err)
	}
	if err := SaveGenerationProtocol(task, &model.GenerationProtocolSnapshot{ChannelID: "replacement"}, workspaceTestNow); err == nil {
		t.Fatal("second submission was allowed")
	}
	var saved model.GenerationTask
	if err := db.First(&saved, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.ProtocolSnapshot == nil || saved.ProtocolSnapshot.ChannelID != "original" {
		t.Fatal("snapshot was overwritten")
	}
	// Protocol tasks can resume promptly, but batch tasks and leased submissions remain excluded.
	items, err := ListStaleRunningGenerationTasks(workspaceTestOld, workspaceTestNow, 100)
	if err != nil || len(items) != 1 {
		t.Fatalf("protocol task was not scheduled: %v, %v", items, err)
	}
	if err := db.Model(&task).Updates(map[string]any{"batch_job_id": "batch", "batch_item_id": "item"}).Error; err != nil {
		t.Fatal(err)
	}
	items, err = ListStaleRunningGenerationTasks(workspaceTestFuture, workspaceTestFuture, 100)
	if err != nil || len(items) != 0 {
		t.Fatalf("batch task leaked into ordinary worker: %v, %v", items, err)
	}
}
