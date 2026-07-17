package handler

import (
	"encoding/json"
	"net/http"

	"github.com/basketikun/infinite-canvas/service"
)

func UserWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	result, err := service.UserWorkspace(user.ID)
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
	result, err := service.ApplyUserWorkspaceChanges(user.ID, request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func UploadUserWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 81<<20)
	if err := r.ParseMultipartForm(80 << 20); err != nil {
		Fail(w, "读取上传文件失败")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		Fail(w, "请选择要上传的文件")
		return
	}
	result, err := service.SaveUserWorkspaceFile(user.ID, r.FormValue("storageKey"), file, header)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func UserWorkspaceFile(w http.ResponseWriter, r *http.Request, storageKey string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	path, mimeType, ok := service.UserWorkspaceFilePath(user.ID, storageKey)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeFile(w, r, path)
}

func UserStorageStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	result, err := service.GetUserStorageStatus(user.ID)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}
