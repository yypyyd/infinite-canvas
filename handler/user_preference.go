package handler

import (
	"encoding/json"
	"net/http"

	"github.com/basketikun/infinite-canvas/service"
)

func UserPreferences(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	result, err := service.UserPreferences(user.ID)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func SaveUserPreferences(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var request json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		Fail(w, "用户偏好格式不正确")
		return
	}
	result, err := service.SaveUserPreferences(user.ID, request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}
