package handler

import (
	"net/http"

	"github.com/yypyyd/infinite-canvas/service"
)

func ReferralDashboard(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	result, err := service.GetReferralDashboard(user, parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}
