package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

func TestProtectedHTTPRoutesRejectUnauthenticatedRequests(t *testing.T) {
	server := httptest.NewServer(New())
	t.Cleanup(server.Close)
	client := &http.Client{}
	requests := []struct {
		method string
		path   string
		body   any
	}{
		{method: http.MethodGet, path: "/api/credit-logs"},
		{method: http.MethodGet, path: "/api/generation-tasks"},
		{method: http.MethodGet, path: "/api/workspace"},
		{method: http.MethodGet, path: "/api/workspace/status"},
		{method: http.MethodGet, path: "/api/preferences"},
		{method: http.MethodPost, path: "/api/preferences", body: map[string]any{"theme": "dark"}},
		{method: http.MethodGet, path: "/api/commerce/workspace"},
		{method: http.MethodPost, path: "/api/v1/images/generations", body: map[string]any{"model": "forbidden"}},
		{method: http.MethodGet, path: "/api/check-in"},
		{method: http.MethodPost, path: "/api/redeem-codes/redeem", body: map[string]any{"code": "forbidden"}},
		{method: http.MethodGet, path: "/api/admin/users"},
		{method: http.MethodGet, path: "/api/admin/operations/health"},
		{method: http.MethodGet, path: "/api/admin/operations/data-consistency"},
		{method: http.MethodPost, path: "/api/admin/operations/data-consistency/repair", body: map[string]any{"issueId": "forged"}},
		{method: http.MethodGet, path: "/api/admin/settings"},
	}
	for _, request := range requests {
		response := routerTestJSON(t, client, request.method, server.URL+request.path, request.body, map[string]string{"X-Organization-ID": "forged-organization"})
		if response.Code != 1 {
			t.Fatalf("unauthenticated %s %s response: %#v", request.method, request.path, response)
		}
	}
}

func TestAdminRoutesRejectUsersAndBannedSessionsLoseAccess(t *testing.T) {
	tenant := seedRouterTestTenant(t, "access-control")
	client, baseURL := loginRouterTestClient(t, tenant.User.Username)
	for _, path := range []string{"/api/admin/users", "/api/admin/operations/health", "/api/admin/operations/data-consistency", "/api/admin/settings"} {
		if response := routerTestJSON(t, client, http.MethodGet, baseURL+path, nil, nil); response.Code != 1 {
			t.Fatalf("non-admin %s response: %#v", path, response)
		}
	}
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&model.User{}).Where("id = ?", tenant.User.ID).Update("status", model.UserStatusBan).Error; err != nil {
		t.Fatal(err)
	}
	if current := routerTestCurrentUser(t, client, baseURL); current.Role != model.UserRoleGuest || current.ID != "" {
		t.Fatalf("banned session remained authenticated: %#v", current)
	}
	if response := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/workspace", nil, map[string]string{"X-Organization-ID": tenant.Organization.ID}); response.Code != 1 {
		t.Fatalf("banned workspace response: %#v", response)
	}
}
