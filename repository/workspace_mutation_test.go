package repository

import (
	"errors"
	"fmt"
	"testing"

	"github.com/basketikun/infinite-canvas/model"
)

func workspaceProjectMutation(recordID, data string, expectedVersion int64, storageKeys ...string) model.UserWorkspaceMutation {
	return model.UserWorkspaceMutation{RecordID: recordID, Domain: "canvas_project", ObjectID: "project-shared", Title: "Canvas", Data: data, ExpectedVersion: expectedVersion, StorageKeys: storageKeys}
}

func TestApplyUserWorkspaceMutationsIsolatesProjectsAndRejectsConcurrentStaleWrites(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	createWorkspaceTestOrganizations(t, testDB, "organization-a", "organization-b")
	if _, _, _, err := ApplyUserWorkspaceMutations("organization-a", "user-a", []model.UserWorkspaceMutation{workspaceProjectMutation("version-a-1", `{"revision":"a1"}`, 0)}, workspaceTestNow); err != nil {
		t.Fatalf("create organization A project: %v", err)
	}
	if _, _, _, err := ApplyUserWorkspaceMutations("organization-b", "user-b", []model.UserWorkspaceMutation{workspaceProjectMutation("version-b-1", `{"revision":"b1"}`, 0)}, workspaceTestNow); err != nil {
		t.Fatalf("create organization B project: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for index, data := range []string{`{"revision":"a2"}`, `{"revision":"a3"}`} {
		mutation := workspaceProjectMutation(fmt.Sprintf("version-a-%d", index+2), data, 1)
		go func() {
			<-start
			_, _, _, err := ApplyUserWorkspaceMutations("organization-a", "user-a", []model.UserWorkspaceMutation{mutation}, workspaceTestFuture)
			results <- err
		}()
	}
	close(start)
	succeeded, conflicted := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrWorkspaceVersionConflict):
			conflicted++
		default:
			t.Fatalf("unexpected concurrent save result: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent saves = %d succeeded, %d conflicted", succeeded, conflicted)
	}

	var projectA, projectB model.UserProject
	if err := testDB.First(&projectA, "organization_id = ? AND id = ?", "organization-a", "project-shared").Error; err != nil {
		t.Fatalf("load organization A project: %v", err)
	}
	if err := testDB.First(&projectB, "organization_id = ? AND id = ?", "organization-b", "project-shared").Error; err != nil {
		t.Fatalf("load organization B project: %v", err)
	}
	if projectA.Version != 2 || (projectA.Data != `{"revision":"a2"}` && projectA.Data != `{"revision":"a3"}`) {
		t.Fatalf("unexpected organization A project: %#v", projectA)
	}
	if projectB.Version != 1 || projectB.Data != `{"revision":"b1"}` {
		t.Fatalf("organization B project was affected: %#v", projectB)
	}
}

func TestApplyUserWorkspaceMutationsTreatsSameDataReplayAsIdempotent(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	createWorkspaceTestOrganizations(t, testDB, "organization-a")
	if _, _, _, err := ApplyUserWorkspaceMutations("organization-a", "user-a", []model.UserWorkspaceMutation{workspaceProjectMutation("version-1", `{"revision":1}`, 0)}, workspaceTestOld); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, _, _, err := ApplyUserWorkspaceMutations("organization-a", "user-a", []model.UserWorkspaceMutation{workspaceProjectMutation("version-2", `{"revision":2}`, 1)}, workspaceTestNow); err != nil {
		t.Fatalf("update project: %v", err)
	}
	projects, _, _, err := ApplyUserWorkspaceMutations("organization-a", "user-a", []model.UserWorkspaceMutation{workspaceProjectMutation("version-replay", `{"revision":2}`, 1)}, workspaceTestFuture)
	if err != nil {
		t.Fatalf("replay project save: %v", err)
	}
	if len(projects) != 1 || projects[0].Version != 2 || projects[0].UpdatedAt != workspaceTestNow {
		t.Fatalf("unexpected replay result: %#v", projects)
	}
	var historyCount int64
	if err := testDB.Model(&model.UserProjectVersion{}).Where("organization_id = ? AND project_id = ?", "organization-a", "project-shared").Count(&historyCount).Error; err != nil {
		t.Fatalf("count project history: %v", err)
	}
	if historyCount != 1 {
		t.Fatalf("project history count = %d, want 1", historyCount)
	}
}

func TestApplyUserWorkspaceMutationsKeepsLatestFiftyProjectVersions(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	createWorkspaceTestOrganizations(t, testDB, "organization-a")
	if _, _, _, err := ApplyUserWorkspaceMutations("organization-a", "user-a", []model.UserWorkspaceMutation{workspaceProjectMutation("version-0", `{"revision":0}`, 0)}, workspaceTestOld); err != nil {
		t.Fatalf("create project: %v", err)
	}
	for version := int64(1); version <= 55; version++ {
		mutation := workspaceProjectMutation(fmt.Sprintf("version-%d", version), fmt.Sprintf(`{"revision":%d}`, version), version)
		if _, _, _, err := ApplyUserWorkspaceMutations("organization-a", "user-a", []model.UserWorkspaceMutation{mutation}, workspaceTestNow); err != nil {
			t.Fatalf("save project version %d: %v", version+1, err)
		}
	}
	var versions []int64
	if err := testDB.Model(&model.UserProjectVersion{}).Where("organization_id = ? AND project_id = ?", "organization-a", "project-shared").Order("version asc").Pluck("version", &versions).Error; err != nil {
		t.Fatalf("list project history: %v", err)
	}
	if len(versions) != 50 || versions[0] != 6 || versions[len(versions)-1] != 55 {
		t.Fatalf("retained project versions = %#v, want 6 through 55", versions)
	}
	var project model.UserProject
	if err := testDB.First(&project, "organization_id = ? AND id = ?", "organization-a", "project-shared").Error; err != nil {
		t.Fatalf("load current project: %v", err)
	}
	if project.Version != 56 || project.Data != `{"revision":55}` {
		t.Fatalf("unexpected current project: %#v", project)
	}
}

func TestApplyUserWorkspaceMutationsRollsBackRecordsAndReferencesTogether(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	createWorkspaceTestOrganizations(t, testDB, "organization-a")
	files := []model.UserFile{
		{ID: "file-old", OrganizationID: "organization-a", UserID: "user-a", StorageKey: "image:old", ObjectKey: "organizations/organization-a/files/old.png", Size: 10, UnreferencedAt: workspaceTestOld},
		{ID: "file-new", OrganizationID: "organization-a", UserID: "user-a", StorageKey: "image:new", ObjectKey: "organizations/organization-a/files/new.png", Size: 10, UnreferencedAt: workspaceTestOld},
	}
	if err := testDB.Create(&files).Error; err != nil {
		t.Fatalf("create workspace files: %v", err)
	}
	if _, _, _, err := ApplyUserWorkspaceMutations("organization-a", "user-a", []model.UserWorkspaceMutation{workspaceProjectMutation("version-1", `{"image":"image:old"}`, 0, "image:old")}, workspaceTestNow); err != nil {
		t.Fatalf("create referenced project: %v", err)
	}
	mutations := []model.UserWorkspaceMutation{
		workspaceProjectMutation("version-2", `{"image":"image:new"}`, 1, "image:new"),
		{RecordID: "asset-version-1", Domain: "asset", ObjectID: "asset-a", Data: `{"image":"image:missing"}`, ExpectedVersion: 0, StorageKeys: []string{"image:missing"}},
	}
	if _, _, _, err := ApplyUserWorkspaceMutations("organization-a", "user-a", mutations, workspaceTestFuture); !errors.Is(err, ErrWorkspaceFileMissing) {
		t.Fatalf("missing file save error = %v, want workspace file missing", err)
	}

	var project model.UserProject
	if err := testDB.First(&project, "organization_id = ? AND id = ?", "organization-a", "project-shared").Error; err != nil {
		t.Fatalf("load rolled back project: %v", err)
	}
	if project.Version != 1 || project.Data != `{"image":"image:old"}` {
		t.Fatalf("project update was not rolled back: %#v", project)
	}
	var assetCount, historyCount int64
	if err := testDB.Model(&model.UserAsset{}).Where("organization_id = ? AND id = ?", "organization-a", "asset-a").Count(&assetCount).Error; err != nil {
		t.Fatalf("count rolled back asset: %v", err)
	}
	if err := testDB.Model(&model.UserProjectVersion{}).Where("organization_id = ? AND project_id = ?", "organization-a", "project-shared").Count(&historyCount).Error; err != nil {
		t.Fatalf("count rolled back history: %v", err)
	}
	if assetCount != 0 || historyCount != 0 {
		t.Fatalf("rolled back records remain: assets=%d history=%d", assetCount, historyCount)
	}
	var references []model.UserFileReference
	if err := testDB.Where("organization_id = ? AND domain = ? AND object_id = ?", "organization-a", "canvas_project", "project-shared").Find(&references).Error; err != nil {
		t.Fatalf("load rolled back references: %v", err)
	}
	if len(references) != 1 || references[0].StorageKey != "image:old" {
		t.Fatalf("unexpected references after rollback: %#v", references)
	}
	var oldFile, newFile model.UserFile
	if err := testDB.First(&oldFile, "id = ?", "file-old").Error; err != nil {
		t.Fatalf("load old file: %v", err)
	}
	if err := testDB.First(&newFile, "id = ?", "file-new").Error; err != nil {
		t.Fatalf("load new file: %v", err)
	}
	if oldFile.UnreferencedAt != "" || newFile.UnreferencedAt != workspaceTestOld {
		t.Fatalf("file reference state was not rolled back: old=%q new=%q", oldFile.UnreferencedAt, newFile.UnreferencedAt)
	}
	var state model.UserWorkspaceState
	if err := testDB.First(&state, "organization_id = ?", "organization-a").Error; err != nil {
		t.Fatalf("load workspace state: %v", err)
	}
	if state.UpdatedAt != workspaceTestNow {
		t.Fatalf("workspace state advanced after rollback: %#v", state)
	}
}
