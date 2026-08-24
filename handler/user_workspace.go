package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

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

func UploadLocalUserWorkspaceFile(w http.ResponseWriter, r *http.Request, uploadID string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 80<<20)
	mimeType := strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])
	if err := service.UploadLocalUserWorkspaceFile(r.Context(), user, uploadID, mimeType, r.Body); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
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
	if file, item, exists := service.OpenLocalUserWorkspaceFile(user, storageKey); exists {
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", item.MimeType)
		w.Header().Set("Content-Length", strconv.FormatInt(item.Size, 10))
		w.Header().Set("Cache-Control", "private, no-store")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.ServeContent(w, r, item.StorageKey, info.ModTime(), file)
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

func PublicLocalStorageReference(w http.ResponseWriter, r *http.Request, fileID string) {
	expires, err := strconv.ParseInt(r.URL.Query().Get("expires"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	file, item, err := service.OpenPublicLocalStorageReference(fileID, expires, r.URL.Query().Get("signature"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", item.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(item.Size, 10))
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = io.Copy(w, io.LimitReader(file, item.Size))
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
