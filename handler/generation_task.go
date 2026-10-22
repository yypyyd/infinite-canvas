package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/yypyyd/infinite-canvas/service"
)

type acknowledgeGenerationTaskRecoveryRequest struct {
	RequestIDs []string `json:"requestIds"`
}

func AcknowledgeUserGenerationTaskRecoveries(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	var input acknowledgeGenerationTaskRecoveryRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		Fail(w, "请求参数无效")
		return
	}
	if err := service.AcknowledgeGenerationTaskRecoveries(user.OrganizationID, user.ID, input.RequestIDs); err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]bool{"acknowledged": true})
}

func RecoverUserGenerationTask(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	result, err := service.RecoverGenerationTask(user.OrganizationID, user.ID, strings.TrimSpace(r.URL.Query().Get("requestId")))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func RecoverAPIGenerationTask(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	result, err := service.RecoverGenerationTask(user.OrganizationID, user.ID, strings.TrimSpace(r.Header.Get("Idempotency-Key")))
	if err != nil {
		FailError(w, err)
		return
	}
	if result.Status == "running" {
		w.Header().Set("Retry-After", "2")
	}
	OK(w, result)
}

func UserGenerationTasks(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	result, err := service.ListUserGenerationTasks(user.OrganizationID, user.ID, parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func AdminGenerationTasks(w http.ResponseWriter, r *http.Request) {
	result, err := service.ListGenerationTasks(parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func AdminDashboard(w http.ResponseWriter, r *http.Request) {
	result, err := service.AdminDashboard()
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func AdminPoolUsage(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	result, err := service.BuildPoolUsage(r.Context(), days)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func AdminPoolPricingGaps(w http.ResponseWriter, r *http.Request) {
	result, err := service.PoolPricingGaps()
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]any{"items": result})
}

// AdminPoolModels lists the running pool's models (alias targets).
func AdminPoolModels(w http.ResponseWriter, r *http.Request) {
	OK(w, map[string]any{"items": service.PoolModelOptions()})
}

func AdminCreatePoolPricingDrafts(w http.ResponseWriter, r *http.Request) {
	created, err := service.CreatePoolPricingDrafts()
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]any{"created": created})
}
