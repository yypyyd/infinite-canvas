package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/basketikun/infinite-canvas/service"
)

func SyncBootstrap(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	result, err := service.SyncBootstrap(user.ID)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func SyncChanges(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	cursor, _ := strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64)
	result, err := service.SyncChanges(user.ID, cursor)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func ApplySyncChanges(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	var request service.SyncChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		Fail(w, "同步数据格式不正确")
		return
	}
	result, err := service.ApplySyncChanges(user.ID, request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func UploadUserCloudFile(w http.ResponseWriter, r *http.Request) {
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
	result, err := service.SaveUserCloudFile(user.ID, r.FormValue("storageKey"), file, header)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func UserCloudFile(w http.ResponseWriter, r *http.Request, storageKey string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	path, mimeType, ok := service.UserCloudFilePath(user.ID, storageKey)
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
