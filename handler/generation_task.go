package handler

import (
	"net/http"

	"github.com/yypyyd/infinite-canvas/service"
)

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
