package router

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
	"github.com/yypyyd/infinite-canvas/service"
	"golang.org/x/crypto/bcrypt"
)

const routerTestPassword = "router-test-password"

type routerTestTenant struct {
	User         model.User
	Organization model.Organization
	Secondary    model.Organization
	Foreign      model.Organization
}

type routerTestResponse struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
	Msg  string          `json:"msg"`
}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "infinite-canvas-router-test-")
	if err != nil {
		panic(err)
	}
	gin.SetMode(gin.TestMode)
	config.Cfg = config.Config{
		StorageDriver:       "sqlite",
		DatabaseDSN:         filepath.Join(dir, "router.db"),
		JWTSecret:           "router-test-secret",
		JWTExpireHours:      1,
		UserStorageQuotaMB:  1,
		QiniuAccessKey:      "router-test-access-key",
		QiniuSecretKey:      "router-test-secret-key",
		QiniuBucket:         "router-test-bucket",
		QiniuRegion:         "as0",
		QiniuDownloadDomain: "https://cdn.example.com",
	}
	code := m.Run()
	if database, databaseErr := repository.DB(); databaseErr == nil {
		if sqlDB, sqlErr := database.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	}
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestOrganizationHeaderSelectsMembershipAndRejectsForgedTenant(t *testing.T) {
	tenant := seedRouterTestTenant(t, "header")
	client, baseURL := loginRouterTestClient(t, tenant.User.Username)

	selected := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/commerce/workspace", nil, map[string]string{"X-Organization-ID": tenant.Secondary.ID})
	if selected.Code != 0 {
		t.Fatalf("select organization response: %#v", selected)
	}
	var workspace model.OrganizationWorkspace
	if err := json.Unmarshal(selected.Data, &workspace); err != nil || workspace.Organization.ID != tenant.Secondary.ID {
		t.Fatalf("unexpected selected organization: %#v, err=%v", workspace.Organization, err)
	}
	savedUser, ok, err := repository.GetUserByID(tenant.User.ID)
	if err != nil || !ok || savedUser.OrganizationID != tenant.Organization.ID {
		t.Fatalf("request header should not persist organization switch: %#v, err=%v", savedUser, err)
	}

	forged := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/commerce/workspace", nil, map[string]string{"X-Organization-ID": tenant.Foreign.ID})
	if forged.Code != 1 || forged.Msg != "你不是该企业成员" {
		t.Fatalf("unexpected forged organization response: %#v", forged)
	}
}

func TestOrganizationSwitchAndMembershipRemovalRecoverSession(t *testing.T) {
	tenant := seedRouterTestTenant(t, "switch")
	client, baseURL := loginRouterTestClient(t, tenant.User.Username)

	switched := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/commerce/organizations/switch", map[string]string{"organizationId": tenant.Secondary.ID}, map[string]string{"X-Organization-ID": tenant.Organization.ID})
	if switched.Code != 0 {
		t.Fatalf("switch organization response: %#v", switched)
	}
	current := routerTestCurrentUser(t, client, baseURL)
	if current.OrganizationID != tenant.Secondary.ID {
		t.Fatalf("current organization = %q, want %q", current.OrganizationID, tenant.Secondary.ID)
	}

	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Where("organization_id = ? AND user_id = ?", tenant.Secondary.ID, tenant.User.ID).Delete(&model.OrganizationMember{}).Error; err != nil {
		t.Fatal(err)
	}
	current = routerTestCurrentUser(t, client, baseURL)
	if current.OrganizationID != tenant.Organization.ID {
		t.Fatalf("fallback organization = %q, want %q", current.OrganizationID, tenant.Organization.ID)
	}
}

func TestLogoutRemovesSessionCookieFromClient(t *testing.T) {
	tenant := seedRouterTestTenant(t, "logout")
	client, baseURL := loginRouterTestClient(t, tenant.User.Username)
	if current := routerTestCurrentUser(t, client, baseURL); current.Role != model.UserRoleUser {
		t.Fatalf("unexpected authenticated user: %#v", current)
	}
	loggedOut := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/auth/logout", nil, nil)
	if loggedOut.Code != 0 {
		t.Fatalf("logout response: %#v", loggedOut)
	}
	if current := routerTestCurrentUser(t, client, baseURL); current.Role != model.UserRoleGuest || current.ID != "" {
		t.Fatalf("expected guest after logout, got %#v", current)
	}
}

func TestWorkspaceFileUploadTicketIsTenantScopedAndRejectsReservedStorageKey(t *testing.T) {
	tenant := seedRouterTestTenant(t, "media-ticket")
	client, baseURL := loginRouterTestClient(t, tenant.User.Username)
	headers := map[string]string{"X-Organization-ID": tenant.Organization.ID}

	response := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/workspace/files/upload-ticket", map[string]any{"storageKey": "image:router-media-a", "mimeType": "image/png", "size": 1024}, headers)
	if response.Code != 0 {
		t.Fatalf("upload ticket response: %#v", response)
	}
	var ticket service.UserFileUploadTicket
	if err := json.Unmarshal(response.Data, &ticket); err != nil {
		t.Fatal(err)
	}
	if !ticket.UploadRequired || ticket.UploadID == "" || ticket.UploadToken == "" || ticket.ObjectKey == "" {
		t.Fatalf("unexpected upload ticket: %#v", ticket)
	}
	reservation, ok, err := repository.GetUserFileUploadReservation(tenant.Organization.ID, tenant.User.ID, ticket.UploadID)
	if err != nil || !ok {
		t.Fatalf("upload reservation not found: %#v, err=%v", reservation, err)
	}
	if reservation.StorageKey != "image:router-media-a" || reservation.ObjectKey != ticket.ObjectKey || !strings.HasPrefix(reservation.ObjectKey, "organizations/"+tenant.Organization.ID+"/uploads/") {
		t.Fatalf("unexpected upload reservation: %#v", reservation)
	}

	reserved := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/workspace/files/upload-ticket", map[string]any{"storageKey": "image:batch-result-forged", "mimeType": "image/png", "size": 1024}, headers)
	if reserved.Code != 1 {
		t.Fatalf("reserved storage key response: %#v", reserved)
	}
	forged := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/workspace/files/confirm", map[string]any{"uploadId": ticket.UploadID, "storageKey": reservation.StorageKey, "objectKey": "organizations/forged/file.png", "mimeType": reservation.MimeType, "size": reservation.Size}, headers)
	if forged.Code != 1 {
		t.Fatalf("forged object key response: %#v", forged)
	}
	if _, ok, err := repository.GetUserFileUploadReservation(tenant.Organization.ID, tenant.User.ID, ticket.UploadID); err != nil || !ok {
		t.Fatalf("forged confirmation should keep reservation, ok=%v, err=%v", ok, err)
	}
}

func TestWorkspaceFileCancelCannotCrossUserBoundary(t *testing.T) {
	tenant := seedRouterTestTenant(t, "media-cancel")
	ownerClient, ownerURL := loginRouterTestClient(t, tenant.User.Username)
	headers := map[string]string{"X-Organization-ID": tenant.Organization.ID}
	response := routerTestJSON(t, ownerClient, http.MethodPost, ownerURL+"/api/workspace/files/upload-ticket", map[string]any{"storageKey": "image:router-media-cancel", "mimeType": "image/png", "size": 1024}, headers)
	var ticket service.UserFileUploadTicket
	if response.Code != 0 || json.Unmarshal(response.Data, &ticket) != nil {
		t.Fatalf("upload ticket response: %#v", response)
	}

	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	password, err := bcrypt.GenerateFromPassword([]byte(routerTestPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	member := model.User{ID: "user-router-media-cancel-member", Username: "router-media-cancel-member", Password: string(password), OrganizationID: tenant.Organization.ID, Role: model.UserRoleUser, Group: "default", AffCode: "aff-router-media-cancel-member", Status: model.UserStatusActive, CreatedAt: "2", UpdatedAt: "2"}
	if err := database.Create(&member).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&model.OrganizationMember{ID: "member-router-media-cancel", OrganizationID: tenant.Organization.ID, UserID: member.ID, Role: model.OrganizationRoleMember, Version: 1, CreatedAt: "2", UpdatedAt: "2"}).Error; err != nil {
		t.Fatal(err)
	}
	memberClient, memberURL := loginRouterTestClient(t, member.Username)
	if cancelled := routerTestJSON(t, memberClient, http.MethodPost, memberURL+"/api/workspace/files/"+ticket.UploadID+"/cancel", nil, headers); cancelled.Code != 0 {
		t.Fatalf("cross-user cancel response: %#v", cancelled)
	}
	if _, ok, err := repository.GetUserFileUploadReservation(tenant.Organization.ID, tenant.User.ID, ticket.UploadID); err != nil || !ok {
		t.Fatalf("cross-user cancel removed reservation, ok=%v, err=%v", ok, err)
	}
	if cancelled := routerTestJSON(t, ownerClient, http.MethodPost, ownerURL+"/api/workspace/files/"+ticket.UploadID+"/cancel", nil, headers); cancelled.Code != 0 {
		t.Fatalf("owner cancel response: %#v", cancelled)
	}
	if _, ok, err := repository.GetUserFileUploadReservation(tenant.Organization.ID, tenant.User.ID, ticket.UploadID); err != nil || ok {
		t.Fatalf("owner cancel did not remove reservation, ok=%v, err=%v", ok, err)
	}
	var deletion model.UserObjectDeletion
	if err := database.First(&deletion, "id = ?", ticket.ObjectKey).Error; err != nil || deletion.OrganizationID != tenant.Organization.ID || deletion.UserID != tenant.User.ID {
		t.Fatalf("unexpected deletion outbox: %#v, err=%v", deletion, err)
	}
}

func TestWorkspaceFileRedirectIsTenantScoped(t *testing.T) {
	tenant := seedRouterTestTenant(t, "media-read")
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	storageKey := "image:shared-router-media"
	files := []model.UserFile{
		{ID: "file-router-media-a", OrganizationID: tenant.Organization.ID, UserID: tenant.User.ID, StorageKey: storageKey, ObjectKey: "organizations/" + tenant.Organization.ID + "/files/a.png", Hash: "hash-a", MimeType: "image/png", Size: 10, CreatedAt: "1", UpdatedAt: "1"},
		{ID: "file-router-media-b", OrganizationID: tenant.Secondary.ID, UserID: tenant.User.ID, StorageKey: storageKey, ObjectKey: "organizations/" + tenant.Secondary.ID + "/files/b.png", Hash: "hash-b", MimeType: "image/png", Size: 20, CreatedAt: "2", UpdatedAt: "2"},
	}
	if err := database.Create(&files).Error; err != nil {
		t.Fatal(err)
	}
	client, baseURL := loginRouterTestClient(t, tenant.User.Username)
	redirectClient := *client
	redirectClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for _, item := range files {
		request, err := http.NewRequest(http.MethodGet, baseURL+"/api/workspace/files/"+storageKey, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-Organization-ID", item.OrganizationID)
		response, err := redirectClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusTemporaryRedirect || response.Header.Get("Cache-Control") != "private, no-store" || !strings.Contains(response.Header.Get("Location"), item.ObjectKey) {
			t.Fatalf("unexpected file redirect for %s: status=%d, location=%q, cache=%q", item.OrganizationID, response.StatusCode, response.Header.Get("Location"), response.Header.Get("Cache-Control"))
		}
		other := files[0].ObjectKey
		if item.OrganizationID == files[0].OrganizationID {
			other = files[1].ObjectKey
		}
		if strings.Contains(response.Header.Get("Location"), other) {
			t.Fatalf("redirect leaked another organization object: %q", response.Header.Get("Location"))
		}
	}
}

func seedRouterTestTenant(t *testing.T, suffix string) routerTestTenant {
	t.Helper()
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	password, err := bcrypt.GenerateFromPassword([]byte(routerTestPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	tenant := routerTestTenant{
		User:         model.User{ID: "user-router-" + suffix, Username: "router-" + suffix, Password: string(password), OrganizationID: "org-router-" + suffix + "-a", Role: model.UserRoleUser, Group: "default", AffCode: "aff-router-" + suffix, Status: model.UserStatusActive, CreatedAt: "1", UpdatedAt: "1"},
		Organization: model.Organization{ID: "org-router-" + suffix + "-a", Name: "企业 A", Slug: "router-" + suffix + "-a", Status: "active", Version: 1, CreatedBy: "user-router-" + suffix, CreatedAt: "1", UpdatedAt: "1"},
		Secondary:    model.Organization{ID: "org-router-" + suffix + "-b", Name: "企业 B", Slug: "router-" + suffix + "-b", Status: "active", Version: 1, CreatedBy: "user-router-" + suffix, CreatedAt: "2", UpdatedAt: "2"},
		Foreign:      model.Organization{ID: "org-router-" + suffix + "-foreign", Name: "其他企业", Slug: "router-" + suffix + "-foreign", Status: "active", Version: 1, CreatedBy: "foreign", CreatedAt: "3", UpdatedAt: "3"},
	}
	if err := database.Create(&tenant.User).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&[]model.Organization{tenant.Organization, tenant.Secondary, tenant.Foreign}).Error; err != nil {
		t.Fatal(err)
	}
	memberships := []model.OrganizationMember{
		{ID: "member-router-" + suffix + "-a", OrganizationID: tenant.Organization.ID, UserID: tenant.User.ID, Role: model.OrganizationRoleOwner, Version: 1, CreatedAt: "1", UpdatedAt: "1"},
		{ID: "member-router-" + suffix + "-b", OrganizationID: tenant.Secondary.ID, UserID: tenant.User.ID, Role: model.OrganizationRoleMember, Version: 1, CreatedAt: "2", UpdatedAt: "2"},
	}
	if err := database.Create(&memberships).Error; err != nil {
		t.Fatal(err)
	}
	return tenant
}

func loginRouterTestClient(t *testing.T, username string) (*http.Client, string) {
	t.Helper()
	server := httptest.NewServer(New())
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	response := routerTestJSON(t, client, http.MethodPost, server.URL+"/api/auth/login", map[string]string{"username": username, "password": routerTestPassword}, nil)
	if response.Code != 0 {
		t.Fatalf("login response: %#v", response)
	}
	return client, server.URL
}

func routerTestCurrentUser(t *testing.T, client *http.Client, baseURL string) model.AuthUser {
	t.Helper()
	response := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/auth/me", nil, nil)
	if response.Code != 0 {
		t.Fatalf("current user response: %#v", response)
	}
	var user model.AuthUser
	if err := json.Unmarshal(response.Data, &user); err != nil {
		t.Fatal(err)
	}
	return user
}

func routerTestJSON(t *testing.T, client *http.Client, method string, url string, body any, headers map[string]string) routerTestResponse {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("%s %s status = %d", method, url, response.StatusCode)
	}
	var result routerTestResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}
