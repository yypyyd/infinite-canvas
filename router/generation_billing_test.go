package router

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
	"github.com/basketikun/infinite-canvas/service"
)

func TestImageGenerationHTTPBillingAndIdempotency(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if r.URL.Path != "/v1/images/generations" || r.Header.Get("Authorization") != "Bearer router-generation-key" || r.Header.Get("Idempotency-Key") == "" || payload["model"] != "router-upstream-image" {
			http.Error(w, "invalid upstream request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if payload["prompt"] == "fail-switch" {
			database, err := repository.DB()
			if err != nil || database.Model(&model.Organization{}).Where("id = ?", "org-router-generation-shared-switch-a").Update("credit_mode", model.OrganizationCreditModePersonal).Error != nil {
				http.Error(w, "failed to switch credit mode", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"rejected"}}`)
			return
		}
		if payload["prompt"] == "fail" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"rejected"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"created":1,"data":[{"b64_json":"aW1hZ2U="}]}`)
	}))
	defer upstream.Close()

	saved, err := repository.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings := saved
	settings.Public.ModelChannel = model.PublicModelChannelSetting{
		AvailableModels: []string{"router-image-model"},
		Models: []model.ModelDefinition{{ID: "router-image-model", Name: "Router Image", Modality: "image", Operations: []string{"generation"}, ResolutionTiers: []string{"1k"}, Enabled: true}},
		PricingRules: []model.PricingRule{{Model: "router-image-model", Modality: "image", Operation: "generation", Unit: "image", BillingMode: "fixed", Credits: 2, Enabled: true}},
		GroupRatios: map[string]float64{"default": 1},
	}
	settings.Private.Channels = []model.ModelChannel{{
		Name: "router-generation", BaseURL: upstream.URL, APIKey: "router-generation-key", Weight: 1, Enabled: true,
		Models: []model.ChannelModel{{Model: "router-image-model", UpstreamModel: "router-upstream-image", Operations: []string{"generation"}, ResolutionTiers: []string{"1k"}}},
	}}
	if _, err := repository.SaveSettings(settings, "router-generation-settings"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = repository.SaveSettings(saved, "router-generation-settings-cleanup") })

	t.Run("forged organization is rejected before charge", func(t *testing.T) {
		tenant := seedRouterTestTenant(t, "generation-forged")
		setRouterTestCredits(t, tenant.User.ID, 10)
		client, baseURL := loginRouterTestClient(t, tenant.User.Username)
		status, body := routerTestImageGeneration(t, client, baseURL, tenant.Foreign.ID, "router-generation-forged", "success")
		var response routerTestResponse
		if status != http.StatusOK || json.Unmarshal(body, &response) != nil || response.Code != 1 {
			t.Fatalf("forged organization response: status=%d body=%s", status, body)
		}
		assertRouterTestGenerationAccounting(t, tenant, 10, 0, 0, "")
		if upstreamCalls.Load() != 0 {
			t.Fatalf("forged organization reached upstream %d times", upstreamCalls.Load())
		}
	})

	t.Run("successful request charges once and duplicate key is rejected", func(t *testing.T) {
		tenant := seedRouterTestTenant(t, "generation-idempotent")
		setRouterTestCredits(t, tenant.User.ID, 10)
		client, baseURL := loginRouterTestClient(t, tenant.User.Username)
		status, body := routerTestImageGeneration(t, client, baseURL, tenant.Organization.ID, "router-generation-idempotent", "success")
		if status != http.StatusOK || !bytes.Contains(body, []byte(`"data"`)) {
			t.Fatalf("generation response: status=%d body=%s", status, body)
		}
		status, body = routerTestImageGeneration(t, client, baseURL, tenant.Organization.ID, "router-generation-idempotent", "success")
		var duplicate routerTestResponse
		if status != http.StatusOK || json.Unmarshal(body, &duplicate) != nil || duplicate.Code != 1 {
			t.Fatalf("duplicate response: status=%d body=%s", status, body)
		}
		assertRouterTestGenerationAccounting(t, tenant, 6, 1, 1, model.GenerationTaskStatusSuccess)
		if upstreamCalls.Load() != 1 {
			t.Fatalf("upstream calls = %d, want 1", upstreamCalls.Load())
		}
	})

	t.Run("insufficient balance rolls back before upstream", func(t *testing.T) {
		tenant := seedRouterTestTenant(t, "generation-insufficient")
		setRouterTestCredits(t, tenant.User.ID, 3)
		client, baseURL := loginRouterTestClient(t, tenant.User.Username)
		status, body := routerTestImageGeneration(t, client, baseURL, tenant.Organization.ID, "router-generation-insufficient", "success")
		var response routerTestResponse
		if status != http.StatusOK || json.Unmarshal(body, &response) != nil || response.Code != 1 {
			t.Fatalf("insufficient balance response: status=%d body=%s", status, body)
		}
		assertRouterTestGenerationAccounting(t, tenant, 3, 0, 0, "")
		if upstreamCalls.Load() != 1 {
			t.Fatalf("insufficient balance reached upstream, calls=%d", upstreamCalls.Load())
		}
	})

	t.Run("upstream failure refunds in the same request", func(t *testing.T) {
		tenant := seedRouterTestTenant(t, "generation-refund")
		setRouterTestCredits(t, tenant.User.ID, 10)
		client, baseURL := loginRouterTestClient(t, tenant.User.Username)
		status, body := routerTestImageGeneration(t, client, baseURL, tenant.Organization.ID, "router-generation-refund", "fail")
		var response routerTestResponse
		if status != http.StatusOK || json.Unmarshal(body, &response) != nil || response.Code != 1 {
			t.Fatalf("upstream failure response: status=%d body=%s", status, body)
		}
		assertRouterTestGenerationAccounting(t, tenant, 10, 1, 2, model.GenerationTaskStatusFailed)
		if upstreamCalls.Load() != 2 {
			t.Fatalf("upstream calls = %d, want 2", upstreamCalls.Load())
		}
	})

	t.Run("shared mode charges organization without changing personal balance", func(t *testing.T) {
		tenant := seedRouterTestTenant(t, "generation-shared")
		setRouterTestCredits(t, tenant.User.ID, 10)
		setRouterTestOrganizationCredits(t, tenant.Organization.ID, 10, 100, 0)
		client, baseURL := loginRouterTestClient(t, tenant.User.Username)
		status, body := routerTestImageGeneration(t, client, baseURL, tenant.Organization.ID, "router-generation-shared", "success")
		if status != http.StatusOK || !bytes.Contains(body, []byte(`"data"`)) { t.Fatalf("shared generation response: status=%d body=%s", status, body) }
		assertRouterTestSharedGenerationAccounting(t, tenant, 10, 6, 4, model.GenerationTaskStatusSuccess)
		if upstreamCalls.Load() != 3 { t.Fatalf("upstream calls = %d, want 3", upstreamCalls.Load()) }
	})

	t.Run("shared balance and monthly budget stop requests before upstream", func(t *testing.T) {
		tenant := seedRouterTestTenant(t, "generation-shared-limits")
		setRouterTestCredits(t, tenant.User.ID, 10)
		setRouterTestOrganizationCredits(t, tenant.Organization.ID, 3, 100, 0)
		client, baseURL := loginRouterTestClient(t, tenant.User.Username)
		if status, body := routerTestImageGeneration(t, client, baseURL, tenant.Organization.ID, "router-generation-shared-balance", "success"); status != http.StatusOK || !bytes.Contains(body, []byte(`"code":1`)) { t.Fatalf("shared balance response: status=%d body=%s", status, body) }
		setRouterTestOrganizationCredits(t, tenant.Organization.ID, 10, 3, 0)
		if status, body := routerTestImageGeneration(t, client, baseURL, tenant.Organization.ID, "router-generation-shared-budget", "success"); status != http.StatusOK || !bytes.Contains(body, []byte(`"code":1`)) { t.Fatalf("shared budget response: status=%d body=%s", status, body) }
		if upstreamCalls.Load() != 3 { t.Fatalf("shared limits reached upstream, calls=%d", upstreamCalls.Load()) }
	})

	t.Run("shared mode failure refunds organization budget", func(t *testing.T) {
		tenant := seedRouterTestTenant(t, "generation-shared-refund")
		setRouterTestCredits(t, tenant.User.ID, 10)
		setRouterTestOrganizationCredits(t, tenant.Organization.ID, 10, 100, 0)
		client, baseURL := loginRouterTestClient(t, tenant.User.Username)
		status, body := routerTestImageGeneration(t, client, baseURL, tenant.Organization.ID, "router-generation-shared-refund", "fail")
		if status != http.StatusOK || !bytes.Contains(body, []byte(`"code":1`)) { t.Fatalf("shared refund response: status=%d body=%s", status, body) }
		assertRouterTestSharedGenerationAccounting(t, tenant, 10, 10, 0, model.GenerationTaskStatusFailed)
		if upstreamCalls.Load() != 4 { t.Fatalf("upstream calls = %d, want 4", upstreamCalls.Load()) }
	})

	t.Run("shared charge still refunds organization after mode switch", func(t *testing.T) {
		tenant := seedRouterTestTenant(t, "generation-shared-switch")
		setRouterTestCredits(t, tenant.User.ID, 10)
		setRouterTestOrganizationCredits(t, tenant.Organization.ID, 10, 100, 0)
		client, baseURL := loginRouterTestClient(t, tenant.User.Username)
		status, body := routerTestImageGeneration(t, client, baseURL, tenant.Organization.ID, "router-generation-shared-switch", "fail-switch")
		if status != http.StatusOK || !bytes.Contains(body, []byte(`"code":1`)) { t.Fatalf("shared mode switch response: status=%d body=%s", status, body) }
		assertRouterTestSharedGenerationAccounting(t, tenant, 10, 10, 0, model.GenerationTaskStatusFailed)
		if upstreamCalls.Load() != 5 { t.Fatalf("upstream calls = %d, want 5", upstreamCalls.Load()) }
	})
}

func setRouterTestOrganizationCredits(t *testing.T, organizationID string, credits int, budget int, used int) {
	t.Helper()
	database, err := repository.DB()
	if err != nil { t.Fatal(err) }
	if err := database.Model(&model.Organization{}).Where("id = ?", organizationID).Updates(map[string]any{"credit_mode": model.OrganizationCreditModeShared, "credits": credits, "monthly_credit_budget": budget, "monthly_credits_used": used, "credit_budget_month": time.Now().UTC().Format("2006-01")}).Error; err != nil { t.Fatal(err) }
}

func assertRouterTestSharedGenerationAccounting(t *testing.T, tenant routerTestTenant, personalCredits int, organizationCredits int, monthlyUsed int, status model.GenerationTaskStatus) {
	t.Helper()
	database, err := repository.DB()
	if err != nil { t.Fatal(err) }
	var user model.User
	var organization model.Organization
	var tasks []model.GenerationTask
	var logs []model.CreditLog
	if err := database.First(&user, "id = ?", tenant.User.ID).Error; err != nil { t.Fatal(err) }
	if err := database.First(&organization, "id = ?", tenant.Organization.ID).Error; err != nil { t.Fatal(err) }
	if err := database.Where("organization_id = ? AND user_id = ?", tenant.Organization.ID, tenant.User.ID).Find(&tasks).Error; err != nil { t.Fatal(err) }
	if err := database.Where("organization_id = ? AND user_id = ?", tenant.Organization.ID, tenant.User.ID).Find(&logs).Error; err != nil { t.Fatal(err) }
	if user.Credits != personalCredits || organization.Credits != organizationCredits || organization.MonthlyCreditsUsed != monthlyUsed || len(tasks) != 1 || tasks[0].Status != status || tasks[0].CreditSource != model.CreditSourceOrganization {
		t.Fatalf("unexpected shared accounting: user=%#v organization=%#v tasks=%#v", user, organization, tasks)
	}
	wantLogs := 1
	if status == model.GenerationTaskStatusFailed { wantLogs = 2 }
	if len(logs) != wantLogs { t.Fatalf("unexpected shared credit logs: %#v", logs) }
	for _, log := range logs { if log.CreditSource != model.CreditSourceOrganization { t.Fatalf("shared task used personal ledger: %#v", log) } }
}

func routerTestImageGeneration(t *testing.T, client *http.Client, baseURL, organizationID, requestID, prompt string) (int, []byte) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"model": "router-image-model", "prompt": prompt, "n": 2, "size": "1024x1024"})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/images/generations", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Organization-ID", organizationID)
	request.Header.Set("Idempotency-Key", requestID)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, responseBody
}

func setRouterTestCredits(t *testing.T, userID string, credits int) {
	t.Helper()
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&model.User{}).Where("id = ?", userID).Update("credits", credits).Error; err != nil {
		t.Fatal(err)
	}
}

func assertRouterTestGenerationAccounting(t *testing.T, tenant routerTestTenant, credits, taskCount, logCount int, status model.GenerationTaskStatus) {
	t.Helper()
	user, ok, err := repository.GetUserByID(tenant.User.ID)
	if err != nil || !ok || user.Credits != credits {
		t.Fatalf("unexpected user credits: user=%#v ok=%v err=%v", user, ok, err)
	}
	tasks, err := service.ListUserGenerationTasks(tenant.Organization.ID, tenant.User.ID, model.Query{})
	if err != nil || tasks.Total != taskCount {
		t.Fatalf("unexpected generation tasks: %#v err=%v", tasks, err)
	}
	logs, err := service.ListUserCreditLogs(tenant.User.ID, model.Query{})
	if err != nil || logs.Total != logCount {
		t.Fatalf("unexpected credit logs: %#v err=%v", logs, err)
	}
	if taskCount == 1 && (tasks.Items[0].Credits != 4 || tasks.Items[0].RequestID != tenant.User.Username || tasks.Items[0].Status != status) {
		t.Fatalf("unexpected generation task accounting: %#v", tasks.Items[0])
	}
	counts := map[model.CreditLogType]int{}
	for _, item := range logs.Items {
		counts[item.Type]++
		if item.RelatedID == "" {
			t.Fatalf("credit log is not linked to a task: %#v", item)
		}
	}
	if logCount == 1 && counts[model.CreditLogTypeAIConsume] != 1 {
		t.Fatalf("unexpected consumption logs: %#v", logs.Items)
	}
	if logCount == 2 && (counts[model.CreditLogTypeAIConsume] != 1 || counts[model.CreditLogTypeAIRefund] != 1) {
		t.Fatalf("unexpected refund logs: %#v", logs.Items)
	}
}
