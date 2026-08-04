package router

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
	"github.com/yypyyd/infinite-canvas/service"
)

func TestWorkspaceSaveHTTPIsolatesTenantsAndEnforcesVersions(t *testing.T) {
	tenant := seedRouterTestTenant(t, "workspace-save")
	client, baseURL := loginRouterTestClient(t, tenant.User.Username)
	projectID := "project-router-shared"

	createdA := routerTestSaveWorkspace(t, client, baseURL, tenant.Organization.ID, service.WorkspaceChangeInput{Domain: "canvas_project", ObjectID: projectID, Data: json.RawMessage(`{"title":"A","revision":"a1"}`)})
	createdB := routerTestSaveWorkspace(t, client, baseURL, tenant.Secondary.ID, service.WorkspaceChangeInput{Domain: "canvas_project", ObjectID: projectID, Data: json.RawMessage(`{"title":"B","revision":"b1"}`)})
	if createdA.Code != 0 || createdB.Code != 0 {
		t.Fatalf("create workspace responses: a=%#v b=%#v", createdA, createdB)
	}
	assertRouterTestWorkspaceRevision(t, client, baseURL, tenant.Organization.ID, projectID, "a1", 1)
	assertRouterTestWorkspaceRevision(t, client, baseURL, tenant.Secondary.ID, projectID, "b1", 1)

	updated := routerTestSaveWorkspace(t, client, baseURL, tenant.Organization.ID, service.WorkspaceChangeInput{Domain: "canvas_project", ObjectID: projectID, Version: 1, Data: json.RawMessage(`{"title":"A","revision":"a2"}`)})
	stale := routerTestSaveWorkspace(t, client, baseURL, tenant.Organization.ID, service.WorkspaceChangeInput{Domain: "canvas_project", ObjectID: projectID, Version: 1, Data: json.RawMessage(`{"title":"A","revision":"a3"}`)})
	replayed := routerTestSaveWorkspace(t, client, baseURL, tenant.Organization.ID, service.WorkspaceChangeInput{Domain: "canvas_project", ObjectID: projectID, Version: 1, Data: json.RawMessage(`{"title":"A","revision":"a2"}`)})
	if updated.Code != 0 || stale.Code != 1 || replayed.Code != 0 {
		t.Fatalf("workspace version responses: updated=%#v stale=%#v replayed=%#v", updated, stale, replayed)
	}
	assertRouterTestWorkspaceRevision(t, client, baseURL, tenant.Organization.ID, projectID, "a2", 2)
	assertRouterTestWorkspaceRevision(t, client, baseURL, tenant.Secondary.ID, projectID, "b1", 1)

	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	var historyCount int64
	if err := database.Model(&model.UserProjectVersion{}).Where("organization_id = ? AND project_id = ?", tenant.Organization.ID, projectID).Count(&historyCount).Error; err != nil || historyCount != 1 {
		t.Fatalf("project history count = %d, err=%v", historyCount, err)
	}
}

func TestWorkspaceSaveHTTPRejectsReviewerAndForeignMedia(t *testing.T) {
	tenant := seedRouterTestTenant(t, "workspace-security")
	client, baseURL := loginRouterTestClient(t, tenant.User.Username)
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	membership := database.Model(&model.OrganizationMember{}).Where("organization_id = ? AND user_id = ?", tenant.Organization.ID, tenant.User.ID)
	if err := membership.Update("role", model.OrganizationRoleReviewer).Error; err != nil {
		t.Fatal(err)
	}
	reviewer := routerTestSaveWorkspace(t, client, baseURL, tenant.Organization.ID, service.WorkspaceChangeInput{Domain: "canvas_project", ObjectID: "project-router-reviewer", Data: json.RawMessage(`{"title":"Reviewer"}`)})
	if reviewer.Code != 1 {
		t.Fatalf("reviewer save response: %#v", reviewer)
	}
	if err := database.Model(&model.OrganizationMember{}).Where("organization_id = ? AND user_id = ?", tenant.Organization.ID, tenant.User.ID).Update("role", model.OrganizationRoleOwner).Error; err != nil {
		t.Fatal(err)
	}

	storageKey := "image:router-foreign-media"
	foreignFile := model.UserFile{ID: "file-router-workspace-foreign", OrganizationID: tenant.Secondary.ID, UserID: tenant.User.ID, StorageKey: storageKey, ObjectKey: "organizations/" + tenant.Secondary.ID + "/files/foreign.png", Hash: "foreign", MimeType: "image/png", Size: 10, CreatedAt: "1", UpdatedAt: "1"}
	if err := database.Create(&foreignFile).Error; err != nil {
		t.Fatal(err)
	}
	foreign := routerTestSaveWorkspace(t, client, baseURL, tenant.Organization.ID, service.WorkspaceChangeInput{Domain: "canvas_project", ObjectID: "project-router-foreign-media", Data: json.RawMessage(`{"title":"Foreign","image":"image:router-foreign-media"}`)})
	if foreign.Code != 1 {
		t.Fatalf("foreign media save response: %#v", foreign)
	}
	var projects, references, states int64
	if err := database.Model(&model.UserProject{}).Where("organization_id = ? AND id IN ?", tenant.Organization.ID, []string{"project-router-reviewer", "project-router-foreign-media"}).Count(&projects).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&model.UserFileReference{}).Where("organization_id = ?", tenant.Organization.ID).Count(&references).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&model.UserWorkspaceState{}).Where("organization_id = ?", tenant.Organization.ID).Count(&states).Error; err != nil {
		t.Fatal(err)
	}
	if projects != 0 || references != 0 || states != 0 {
		t.Fatalf("rejected workspace writes persisted: projects=%d references=%d states=%d", projects, references, states)
	}
}

func routerTestSaveWorkspace(t *testing.T, client *http.Client, baseURL, organizationID string, change service.WorkspaceChangeInput) routerTestResponse {
	t.Helper()
	return routerTestJSON(t, client, http.MethodPost, baseURL+"/api/workspace/changes", service.WorkspaceChangeRequest{Changes: []service.WorkspaceChangeInput{change}}, map[string]string{"X-Organization-ID": organizationID})
}

func assertRouterTestWorkspaceRevision(t *testing.T, client *http.Client, baseURL, organizationID, projectID, revision string, version int64) {
	t.Helper()
	response := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/workspace", nil, map[string]string{"X-Organization-ID": organizationID})
	if response.Code != 0 {
		t.Fatalf("workspace response: %#v", response)
	}
	var payload service.WorkspacePayload
	if err := json.Unmarshal(response.Data, &payload); err != nil {
		t.Fatal(err)
	}
	for _, record := range payload.Records {
		if record.Domain != "canvas_project" || record.ObjectID != projectID {
			continue
		}
		var data struct {
			Revision string `json:"revision"`
		}
		if err := json.Unmarshal(record.Data, &data); err != nil || data.Revision != revision || record.Version != version {
			t.Fatalf("unexpected workspace record: %#v data=%#v err=%v", record, data, err)
		}
		return
	}
	t.Fatalf("workspace project %q not found in organization %q", projectID, organizationID)
}
