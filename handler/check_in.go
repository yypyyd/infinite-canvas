package handler

import (
	"net/http"

	"github.com/basketikun/infinite-canvas/service"
)

func CheckInStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	result, err := service.CheckInStatus(user.ID)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func CheckIn(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	result, err := service.CheckIn(user.ID)
	if err != nil {
		FailError(w, err)
		return
	}
	result.User.OrganizationID = user.OrganizationID
	result.User, err = service.ApplyEffectiveCredits(result.User)
	if err != nil { FailError(w, err); return }
	OK(w, result)
}
