package router

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func TestUserPreferencesHTTPAreAccountScopedAndValidated(t *testing.T) {
	tenant := seedRouterTestTenant(t, "preferences")
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	other := model.User{
		ID: "user-router-preferences-other", Username: "router-preferences-other", Password: tenant.User.Password,
		OrganizationID: tenant.Secondary.ID, Role: model.UserRoleUser, Group: "default",
		AffCode: "aff-router-preferences-other", Status: model.UserStatusActive, CreatedAt: "2", UpdatedAt: "2",
	}
	membership := model.OrganizationMember{
		ID: "member-router-preferences-other", OrganizationID: tenant.Secondary.ID, UserID: other.ID,
		Role: model.OrganizationRoleMember, Version: 1, CreatedAt: "2", UpdatedAt: "2",
	}
	if err := database.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&membership).Error; err != nil {
		t.Fatal(err)
	}

	client, baseURL := loginRouterTestClient(t, tenant.User.Username)
	empty := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/preferences", nil, nil)
	var preferences map[string]json.RawMessage
	if empty.Code != 0 || json.Unmarshal(empty.Data, &preferences) != nil || len(preferences) != 0 {
		t.Fatalf("unexpected empty preferences: %#v, value=%#v", empty, preferences)
	}
	primaryValue := map[string]any{
		"theme":           "dark",
		"aiConfig":        map[string]string{"imageModel": "router-image", "videoModel": "router-video"},
		"imageQuickTools": map[string]any{"ids": []string{"download", "edit"}, "showLabels": true},
	}
	saved := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/preferences", primaryValue, nil)
	if saved.Code != 0 || json.Unmarshal(saved.Data, &preferences) != nil || string(preferences["theme"]) != `"dark"` {
		t.Fatalf("save preferences response: %#v, value=%#v", saved, preferences)
	}
	accountRead := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/preferences", nil, map[string]string{"X-Organization-ID": tenant.Secondary.ID})
	preferences = nil
	if accountRead.Code != 0 || json.Unmarshal(accountRead.Data, &preferences) != nil || string(preferences["theme"]) != `"dark"` {
		t.Fatalf("account preferences changed with organization header: %#v, value=%#v", accountRead, preferences)
	}
	if response := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/preferences", nil, map[string]string{"X-Organization-ID": tenant.Foreign.ID}); response.Code != 1 {
		t.Fatalf("forged organization preferences response: %#v", response)
	}

	otherClient, otherURL := loginRouterTestClient(t, other.Username)
	otherEmpty := routerTestJSON(t, otherClient, http.MethodGet, otherURL+"/api/preferences", nil, nil)
	preferences = nil
	if otherEmpty.Code != 0 || json.Unmarshal(otherEmpty.Data, &preferences) != nil || len(preferences) != 0 {
		t.Fatalf("preferences leaked to another account: %#v, value=%#v", otherEmpty, preferences)
	}
	otherSaved := routerTestJSON(t, otherClient, http.MethodPost, otherURL+"/api/preferences", map[string]any{"theme": "light"}, nil)
	if otherSaved.Code != 0 {
		t.Fatalf("save other preferences response: %#v", otherSaved)
	}

	invalidValues := []any{
		map[string]any{"unknown": true},
		map[string]any{"theme": "system"},
		map[string]any{"aiConfig": map[string]any{"imageModel": 1}},
		map[string]any{"imageQuickTools": map[string]any{"ids": make([]string, 65)}},
	}
	for _, value := range invalidValues {
		if response := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/preferences", value, nil); response.Code != 1 {
			t.Fatalf("invalid preferences response: %#v", response)
		}
	}
	oversized := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/preferences", map[string]any{"theme": strings.Repeat("x", 70<<10)}, nil)
	if oversized.Code != 1 {
		t.Fatalf("oversized preferences response: %#v", oversized)
	}

	unchanged := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/preferences", nil, nil)
	preferences = nil
	if unchanged.Code != 0 || json.Unmarshal(unchanged.Data, &preferences) != nil || string(preferences["theme"]) != `"dark"` {
		t.Fatalf("invalid write changed preferences: %#v, value=%#v", unchanged, preferences)
	}
	otherUnchanged := routerTestJSON(t, otherClient, http.MethodGet, otherURL+"/api/preferences", nil, nil)
	preferences = nil
	if otherUnchanged.Code != 0 || json.Unmarshal(otherUnchanged.Data, &preferences) != nil || string(preferences["theme"]) != `"light"` {
		t.Fatalf("other account preferences changed: %#v, value=%#v", otherUnchanged, preferences)
	}
}
