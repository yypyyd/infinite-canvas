package handler

import (
	"encoding/json"
	"net/http"

	"github.com/yypyyd/infinite-canvas/service"
)

func UserWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	result, err := service.UserWorkspace(user)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func SaveUserWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	var request service.WorkspaceChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		Fail(w, "账号数据格式不正确")
		return
	}
	result, err := service.ApplyUserWorkspaceChanges(user, request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PrepareUserWorkspaceFileUpload(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var request service.UserFileUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		Fail(w, "文件上传信息格式不正确")
		return
	}
	result, err := service.PrepareUserWorkspaceFileUpload(user, request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func ConfirmUserWorkspaceFileUpload(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var request service.UserFileConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		Fail(w, "文件确认信息格式不正确")
		return
	}
	result, err := service.ConfirmUserWorkspaceFileUpload(user, request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func CancelUserWorkspaceFileUpload(w http.ResponseWriter, r *http.Request, uploadID string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	if err := service.CancelUserWorkspaceFileUpload(user, uploadID); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}

func UserWorkspaceFile(w http.ResponseWriter, r *http.Request, storageKey string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	if r.Method == http.MethodHead {
		if !service.UserWorkspaceFileExists(user, storageKey) {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	fileURL, ok := service.UserWorkspaceFileURL(user, storageKey, r.URL.Query().Get("variant"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	http.Redirect(w, r, fileURL, http.StatusTemporaryRedirect)
}

func UserStorageStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	result, err := service.GetUserStorageStatus(user)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}
