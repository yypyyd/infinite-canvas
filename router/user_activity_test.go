package router

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

func TestUserActivityHTTPIsScopedByAccountAndOrganization(t *testing.T) {
	tenant := seedRouterTestTenant(t, "user-activity")
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	other := model.User{
		ID: "user-router-activity-other", Username: "router-activity-other", Password: tenant.User.Password,
		OrganizationID: tenant.Organization.ID, Role: model.UserRoleUser, Group: "default",
		AffCode: "aff-router-activity-other", Status: model.UserStatusActive, CreatedAt: "2", UpdatedAt: "2",
	}
	if err := database.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	member := model.OrganizationMember{
		ID: "member-router-activity-other", OrganizationID: tenant.Organization.ID, UserID: other.ID,
		Role: model.OrganizationRoleMember, Version: 1, CreatedAt: "2", UpdatedAt: "2",
	}
	if err := database.Create(&member).Error; err != nil {
		t.Fatal(err)
	}
	tasks := []model.GenerationTask{
		{ID: "task-router-activity-primary", OrganizationID: tenant.Organization.ID, UserID: tenant.User.ID, RequestID: "request-router-activity-primary", Model: "owner-primary-model", Modality: "image", Operation: "generation", Quantity: 1, Credits: 2, Status: model.GenerationTaskStatusSuccess, CreatedAt: "1", UpdatedAt: "1"},
		{ID: "task-router-activity-secondary", OrganizationID: tenant.Secondary.ID, UserID: tenant.User.ID, RequestID: "request-router-activity-secondary", Model: "owner-secondary-model", Modality: "video", Operation: "generation", Quantity: 1, Credits: 3, Status: model.GenerationTaskStatusRunning, CreatedAt: "2", UpdatedAt: "2"},
		{ID: "task-router-activity-other", OrganizationID: tenant.Organization.ID, UserID: other.ID, RequestID: "request-router-activity-other", Model: "foreign-secret-model", Modality: "image", Operation: "generation", Quantity: 1, Credits: 4, Status: model.GenerationTaskStatusFailed, CreatedAt: "3", UpdatedAt: "3"},
	}
	if err := database.Create(&tasks).Error; err != nil {
		t.Fatal(err)
	}
	logs := []model.CreditLog{
		{ID: "credit-router-activity-primary", UserID: tenant.User.ID, Type: model.CreditLogTypeAIConsume, Amount: -2, Balance: 8, RelatedID: tasks[0].ID, Remark: "primary consume", CreatedAt: "1"},
		{ID: "credit-router-activity-secondary", UserID: tenant.User.ID, Type: model.CreditLogTypeAIConsume, Amount: -3, Balance: 5, RelatedID: tasks[1].ID, Remark: "secondary consume", CreatedAt: "2"},
		{ID: "credit-router-activity-other", UserID: other.ID, Type: model.CreditLogTypeAIRefund, Amount: 4, Balance: 10, RelatedID: tasks[2].ID, Remark: "foreign-credit-secret", CreatedAt: "3"},
	}
	if err := database.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}

	client, baseURL := loginRouterTestClient(t, tenant.User.Username)
	primary := map[string]string{"X-Organization-ID": tenant.Organization.ID}
	secondary := map[string]string{"X-Organization-ID": tenant.Secondary.ID}
	assertTaskList := func(headers map[string]string, url string, expectedID string, expectedTotal int) {
		t.Helper()
		response := routerTestJSON(t, client, http.MethodGet, baseURL+url, nil, headers)
		var list model.GenerationTaskList
		if response.Code != 0 || json.Unmarshal(response.Data, &list) != nil || list.Total != expectedTotal || len(list.Items) != expectedTotal {
			t.Fatalf("unexpected generation task response: %#v, list=%#v", response, list)
		}
		if expectedTotal == 1 && list.Items[0].ID != expectedID {
			t.Fatalf("generation task = %q, want %q", list.Items[0].ID, expectedID)
		}
	}
	assertTaskList(primary, "/api/generation-tasks", tasks[0].ID, 1)
	assertTaskList(secondary, "/api/generation-tasks", tasks[1].ID, 1)
	assertTaskList(primary, "/api/generation-tasks?keyword=foreign-secret-model", "", 0)
	if response := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/generation-tasks", nil, map[string]string{"X-Organization-ID": tenant.Foreign.ID}); response.Code != 1 {
		t.Fatalf("forged organization generation task response: %#v", response)
	}

	creditResponse := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/credit-logs", nil, secondary)
	var creditList model.CreditLogList
	if creditResponse.Code != 0 || json.Unmarshal(creditResponse.Data, &creditList) != nil || creditList.Total != 2 || len(creditList.Items) != 2 {
		t.Fatalf("unexpected account credit logs: %#v, list=%#v", creditResponse, creditList)
	}
	for _, log := range creditList.Items {
		if log.UserID != tenant.User.ID || log.ID == logs[2].ID {
			t.Fatalf("credit log leaked another account: %#v", log)
		}
	}
	filteredCredits := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/credit-logs?keyword=foreign-credit-secret", nil, nil)
	var filteredCreditList model.CreditLogList
	if filteredCredits.Code != 0 || json.Unmarshal(filteredCredits.Data, &filteredCreditList) != nil || filteredCreditList.Total != 0 || len(filteredCreditList.Items) != 0 {
		t.Fatalf("credit keyword bypassed account scope: %#v, list=%#v", filteredCredits, filteredCreditList)
	}

	otherClient, otherURL := loginRouterTestClient(t, other.Username)
	otherTasks := routerTestJSON(t, otherClient, http.MethodGet, otherURL+"/api/generation-tasks", nil, primary)
	var otherTaskList model.GenerationTaskList
	if otherTasks.Code != 0 || json.Unmarshal(otherTasks.Data, &otherTaskList) != nil || otherTaskList.Total != 1 || len(otherTaskList.Items) != 1 || otherTaskList.Items[0].ID != tasks[2].ID {
		t.Fatalf("unexpected other account tasks: %#v, list=%#v", otherTasks, otherTaskList)
	}
	otherCredits := routerTestJSON(t, otherClient, http.MethodGet, otherURL+"/api/credit-logs", nil, nil)
	var otherCreditList model.CreditLogList
	if otherCredits.Code != 0 || json.Unmarshal(otherCredits.Data, &otherCreditList) != nil || otherCreditList.Total != 1 || len(otherCreditList.Items) != 1 || otherCreditList.Items[0].ID != logs[2].ID {
		t.Fatalf("unexpected other account credits: %#v, list=%#v", otherCredits, otherCreditList)
	}
}
