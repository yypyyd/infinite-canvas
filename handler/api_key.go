package handler

import (
	"encoding/json"
	"net/http"

	"github.com/basketikun/infinite-canvas/service"
)

type createUserAPIKeyRequest struct {
	Name string `json:"name"`
}

func UserAPIKeys(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	items, err := service.ListUserAPIKeys(user)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, items)
}

func CreateUserAPIKey(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var request createUserAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		Fail(w, "请求格式不正确")
		return
	}
	item, err := service.CreateUserAPIKey(user, request.Name)
	if err != nil {
		FailError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	OK(w, item)
}

func DeleteUserAPIKey(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	if err := service.DeleteUserAPIKey(user, id); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}
