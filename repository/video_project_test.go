package repository

import (
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestSaveVideoProjectNoopKeepsVersion(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	createWorkspaceTestOrganizations(t, testDB, "organization-a")

	timelineJSON := "{\"shots\":[],\"subtitles\":[],\"output\":{\"ratio\":\"16:9\",\"width\":1920,\"height\":1080,\"fps\":30,\"format\":\"mp4\",\"videoCodec\":\"h264\",\"audioCodec\":\"aac\"}}"
	project := model.VideoProject{
		ID:                "video-project-a",
		OrganizationID:    "organization-a",
		Name:              "project-a",
		Description:       "desc",
		DraftTimelineJSON: timelineJSON,
		Status:            model.VideoProjectStatusDraft,
		Version:           1,
		CreatedBy:         "user-a",
		CreatedAt:         workspaceTestNow,
		UpdatedAt:         workspaceTestNow,
	}
	if err := testDB.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	noop := model.VideoProject{
		ID:                project.ID,
		OrganizationID:    project.OrganizationID,
		Name:              project.Name,
		Description:       project.Description,
		DraftTimelineJSON: timelineJSON,
		DraftTimeline:     model.VideoTimeline{},
		Status:            model.VideoProjectStatusDraft,
		Version:           2, // caller always proposes bump; repository must ignore for no-op
		UpdatedAt:         workspaceTestFuture,
	}
	saved, err := SaveVideoProject(noop, 1, nil)
	if err != nil {
		t.Fatalf("noop save: %v", err)
	}
	if saved.Version != 1 {
		t.Fatalf("noop save bumped version to %d, want 1", saved.Version)
	}
	var reloaded model.VideoProject
	if err := testDB.First(&reloaded, "id = ?", project.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Version != 1 || reloaded.UpdatedAt != workspaceTestNow {
		t.Fatalf("noop must leave row unchanged: version=%d updatedAt=%q", reloaded.Version, reloaded.UpdatedAt)
	}

	changed := noop
	changed.Name = "project-a-edited"
	changed.Version = 2
	saved, err = SaveVideoProject(changed, 1, nil)
	if err != nil {
		t.Fatalf("real save: %v", err)
	}
	if saved.Version != 2 || saved.Name != "project-a-edited" {
		t.Fatalf("real save result: %#v", saved)
	}
	if err := testDB.First(&reloaded, "id = ?", project.ID).Error; err != nil {
		t.Fatalf("reload after change: %v", err)
	}
	if reloaded.Version != 2 {
		t.Fatalf("real save version=%d, want 2", reloaded.Version)
	}
}

func TestCreateVideoProjectVersionAfterNoopSave(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	createWorkspaceTestOrganizations(t, testDB, "organization-a")

	timelineJSON := "{\"shots\":[],\"subtitles\":[],\"output\":{\"ratio\":\"16:9\",\"width\":1920,\"height\":1080,\"fps\":30,\"format\":\"mp4\",\"videoCodec\":\"h264\",\"audioCodec\":\"aac\"}}"
	project := model.VideoProject{
		ID:                "video-project-b",
		OrganizationID:    "organization-a",
		Name:              "project-b",
		DraftTimelineJSON: timelineJSON,
		Status:            model.VideoProjectStatusDraft,
		Version:           3,
		CurrentVersion:    0,
		CreatedBy:         "user-a",
		CreatedAt:         workspaceTestNow,
		UpdatedAt:         workspaceTestNow,
	}
	if err := testDB.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	noop := project
	noop.Version = 4
	noop.UpdatedAt = workspaceTestFuture
	saved, err := SaveVideoProject(noop, 3, nil)
	if err != nil {
		t.Fatalf("noop save: %v", err)
	}
	if saved.Version != 3 {
		t.Fatalf("noop version=%d, want 3", saved.Version)
	}

	version := model.VideoProjectVersion{
		ID:             "video-project-version-b1",
		OrganizationID: project.OrganizationID,
		ProjectID:      project.ID,
		CreatedBy:      "user-a",
		CreatedAt:      workspaceTestFuture,
	}
	frozen, err := CreateVideoProjectVersion(saved, timelineJSON, version, nil)
	if err != nil {
		t.Fatalf("freeze after noop save: %v", err)
	}
	if frozen.Version != 1 {
		t.Fatalf("frozen version=%d, want 1", frozen.Version)
	}
	var reloaded model.VideoProject
	if err := testDB.First(&reloaded, "id = ?", project.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.CurrentVersion != 1 || reloaded.Version != 4 || reloaded.Status != model.VideoProjectStatusVersioned {
		t.Fatalf("after freeze: %#v", reloaded)
	}

	stale := saved
	stale.Version = 2
	_, err = CreateVideoProjectVersion(stale, timelineJSON, model.VideoProjectVersion{
		ID:             "video-project-version-stale",
		OrganizationID: project.OrganizationID,
		ProjectID:      project.ID,
		CreatedBy:      "user-a",
		CreatedAt:      workspaceTestFuture,
	}, nil)
	if err != ErrCommerceVersionConflict {
		t.Fatalf("stale freeze err=%v, want conflict", err)
	}
}
