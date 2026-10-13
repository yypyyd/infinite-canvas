package repository

import (
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestWorkspaceSyncVersionsAndTenantIsolation(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	projects := []model.UserProject{
		{ID: "unchanged", OrganizationID: "org-a", Version: 2, Data: `{"title":"unchanged"}`},
		{ID: "changed", OrganizationID: "org-a", Version: 3, Data: `{"title":"changed"}`},
		{ID: "deleted", OrganizationID: "org-a", Version: 4, DeletedAt: workspaceTestNow},
		{ID: "private", OrganizationID: "org-b", Version: 1, Data: `{"title":"private"}`},
	}
	if err := testDB.Create(&projects).Error; err != nil {
		t.Fatal(err)
	}
	versions := map[string]int64{"canvas_project:unchanged": 2, "canvas_project:changed": 2, "canvas_project:deleted": 3}
	changed, assets, records, err := ListUserWorkspaceChanges("org-a", versions)
	if err != nil || len(changed) != 2 || len(assets) != 0 || len(records) != 0 {
		t.Fatalf("unexpected delta: %#v, %v", changed, err)
	}
	for _, item := range changed {
		if item.ID != "changed" && item.ID != "deleted" {
			t.Fatalf("unexpected project: %#v", item)
		}
		if item.ID == "deleted" && item.DeletedAt == "" {
			t.Fatal("deletion was not delivered")
		}
		versions["canvas_project:"+item.ID] = item.Version
	}
	changed, _, _, err = ListUserWorkspaceChanges("org-a", versions)
	if err != nil || len(changed) != 0 {
		t.Fatalf("unchanged sync should be empty: %#v, %v", changed, err)
	}
}
