package service

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/basketikun/infinite-canvas/config"
	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
	"github.com/qiniu/go-sdk/v7/auth/qbox"
	"github.com/qiniu/go-sdk/v7/storage"
)

const maxUserFileSize = 80 << 20

var userStorageKeyPattern = regexp.MustCompile(`^(image|video|audio|file|video-reference|audio-reference):[A-Za-z0-9_-]+$`)

type UserStorageStatus struct {
	UsedBytes    int64  `json:"usedBytes"`
	QuotaBytes   int64  `json:"quotaBytes"`
	FileCount    int64  `json:"fileCount"`
	ProjectCount int    `json:"projectCount"`
	AssetCount   int    `json:"assetCount"`
	LastSavedAt  string `json:"lastSavedAt"`
}

type UserFileUploadRequest struct {
	StorageKey string `json:"storageKey"`
	MimeType   string `json:"mimeType"`
	Size       int64  `json:"size"`
}

type UserFileUploadTicket struct {
	UploadRequired bool            `json:"uploadRequired"`
	UploadURL      string          `json:"uploadUrl,omitempty"`
	UploadToken    string          `json:"uploadToken,omitempty"`
	ObjectKey      string          `json:"objectKey,omitempty"`
	ExpiresAt      string          `json:"expiresAt,omitempty"`
	File           *model.UserFile `json:"file,omitempty"`
}

type UserFileConfirmRequest struct {
	StorageKey string `json:"storageKey"`
	ObjectKey  string `json:"objectKey"`
	MimeType   string `json:"mimeType"`
	Size       int64  `json:"size"`
}

func PrepareUserWorkspaceFileUpload(userID string, request UserFileUploadRequest) (UserFileUploadTicket, error) {
	storageKey, mimeType, err := normalizeUserFileInput(request.StorageKey, request.MimeType, request.Size)
	if err != nil {
		return UserFileUploadTicket{}, err
	}
	if err := ensureQiniuStorageConfigured(); err != nil {
		return UserFileUploadTicket{}, err
	}
	existing, exists, err := repository.GetUserFile(userID, storageKey)
	if err != nil {
		return UserFileUploadTicket{}, err
	}
	if exists {
		if info, statErr := qiniuBucketManager().Stat(config.Cfg.QiniuBucket, existing.ObjectKey); statErr == nil && info.Fsize == existing.Size {
			return UserFileUploadTicket{UploadRequired: false, File: &existing}, nil
		}
	}
	used, _, err := repository.UserStorageUsage(userID)
	if err != nil {
		return UserFileUploadTicket{}, err
	}
	if exists {
		used -= existing.Size
	}
	if used+request.Size > userStorageQuotaBytes() {
		return UserFileUploadTicket{}, safeMessageError{message: "账号云端存储空间不足"}
	}
	objectKey := userFileObjectKey(userID, storageKey, mimeType)
	policy := storage.PutPolicy{
		Scope:       config.Cfg.QiniuBucket + ":" + objectKey,
		Expires:     600,
		FsizeMin:    request.Size,
		FsizeLimit:  request.Size,
		DetectMime:  1,
		MimeLimit:   "image/*;video/*;audio/*",
		EndUser:     userID,
		ReturnBody:  `{"key":"$(key)","hash":"$(etag)","size":$(fsize),"mimeType":"$(mimeType)"}`,
	}
	expiresAt := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	return UserFileUploadTicket{UploadRequired: true, UploadURL: qiniuUploadURL(), UploadToken: policy.UploadToken(qiniuMac()), ObjectKey: objectKey, ExpiresAt: expiresAt}, nil
}

func ConfirmUserWorkspaceFileUpload(userID string, request UserFileConfirmRequest) (model.UserFile, error) {
	storageKey, mimeType, err := normalizeUserFileInput(request.StorageKey, request.MimeType, request.Size)
	if err != nil {
		return model.UserFile{}, err
	}
	if err := ensureQiniuStorageConfigured(); err != nil {
		return model.UserFile{}, err
	}
	expectedKey := userFileObjectKey(userID, storageKey, mimeType)
	if strings.TrimSpace(request.ObjectKey) != expectedKey {
		return model.UserFile{}, safeMessageError{message: "云端文件路径无效"}
	}
	info, err := qiniuBucketManager().Stat(config.Cfg.QiniuBucket, expectedKey)
	if err != nil {
		return model.UserFile{}, safeMessageError{message: "云端文件尚未上传完成"}
	}
	if info.Fsize != request.Size || assetTypeFromMime(info.MimeType) == "" {
		return model.UserFile{}, safeMessageError{message: "云端文件信息校验失败"}
	}
	existing, exists, err := repository.GetUserFile(userID, storageKey)
	if err != nil {
		return model.UserFile{}, err
	}
	used, _, err := repository.UserStorageUsage(userID)
	if err != nil {
		return model.UserFile{}, err
	}
	if exists {
		used -= existing.Size
	}
	if used+info.Fsize > userStorageQuotaBytes() {
		_ = qiniuBucketManager().Delete(config.Cfg.QiniuBucket, expectedKey)
		return model.UserFile{}, safeMessageError{message: "账号云端存储空间不足"}
	}
	timestamp := now()
	item := model.UserFile{ID: newID("file"), UserID: userID, StorageKey: storageKey, ObjectKey: expectedKey, Hash: info.Hash, MimeType: info.MimeType, Size: info.Fsize, CreatedAt: timestamp, UpdatedAt: timestamp}
	if exists {
		item.ID = existing.ID
		item.CreatedAt = existing.CreatedAt
	}
	saved, err := repository.SaveUserFile(item)
	if err != nil {
		return model.UserFile{}, err
	}
	if exists && existing.ObjectKey != "" && existing.ObjectKey != expectedKey {
		_ = qiniuBucketManager().Delete(config.Cfg.QiniuBucket, existing.ObjectKey)
	}
	return saved, nil
}

func UserWorkspaceFileURL(userID string, storageKey string) (string, bool) {
	item, ok, err := repository.GetUserFile(userID, storageKey)
	if err != nil || !ok || strings.TrimSpace(item.ObjectKey) == "" || ensureQiniuStorageConfigured() != nil {
		return "", false
	}
	deadline := time.Now().Add(10 * time.Minute).Unix()
	return storage.MakePrivateURL(qiniuMac(), strings.TrimRight(config.Cfg.QiniuDownloadDomain, "/"), item.ObjectKey, deadline), true
}

func UserWorkspaceFileExists(userID string, storageKey string) bool {
	item, ok, err := repository.GetUserFile(userID, storageKey)
	if err != nil || !ok || ensureQiniuStorageConfigured() != nil {
		return false
	}
	info, err := qiniuBucketManager().Stat(config.Cfg.QiniuBucket, item.ObjectKey)
	return err == nil && info.Fsize == item.Size
}

func GetUserStorageStatus(userID string) (UserStorageStatus, error) {
	used, count, err := repository.UserStorageUsage(userID)
	if err != nil {
		return UserStorageStatus{}, err
	}
	projects, err := repository.ListUserProjects(userID)
	if err != nil {
		return UserStorageStatus{}, err
	}
	assets, err := repository.ListUserAssets(userID)
	if err != nil {
		return UserStorageStatus{}, err
	}
	state, _, err := repository.GetUserWorkspaceState(userID)
	status := UserStorageStatus{UsedBytes: used, QuotaBytes: userStorageQuotaBytes(), FileCount: count, ProjectCount: len(projects), AssetCount: len(assets)}
	if err == nil {
		status.LastSavedAt = state.UpdatedAt
	}
	return status, err
}

func userStorageQuotaBytes() int64 {
	quotaMB := config.Cfg.UserStorageQuotaMB
	if quotaMB <= 0 {
		quotaMB = 5120
	}
	return quotaMB << 20
}

func cleanupUserWorkspaceFiles(userID string) error {
	projects, err := repository.ListUserProjects(userID)
	if err != nil {
		return err
	}
	usedKeys := make(map[string]bool)
	for _, project := range projects {
		collectUserStorageKeys(json.RawMessage(project.Data), usedKeys)
	}
	assets, err := repository.ListUserAssets(userID)
	if err != nil {
		return err
	}
	for _, asset := range assets {
		collectUserStorageKeys(json.RawMessage(asset.Data), usedKeys)
	}
	files, err := repository.ListUserFiles(userID)
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, file := range files {
		createdAt, _ := time.Parse(time.RFC3339, file.CreatedAt)
		if usedKeys[file.StorageKey] || createdAt.IsZero() || createdAt.After(cutoff) {
			continue
		}
		if err := qiniuBucketManager().Delete(config.Cfg.QiniuBucket, file.ObjectKey); err != nil {
			return err
		}
		if err := repository.DeleteUserFile(file.ID); err != nil {
			return err
		}
	}
	return nil
}

func collectUserStorageKeys(value any, keys map[string]bool) {
	switch item := value.(type) {
	case json.RawMessage:
		var decoded any
		if json.Unmarshal(item, &decoded) == nil {
			collectUserStorageKeys(decoded, keys)
		}
	case string:
		if userStorageKeyPattern.MatchString(item) {
			keys[item] = true
		}
	case []any:
		for _, child := range item {
			collectUserStorageKeys(child, keys)
		}
	case map[string]any:
		for _, child := range item {
			collectUserStorageKeys(child, keys)
		}
	}
}

func normalizeUserFileInput(storageKey, mimeType string, size int64) (string, string, error) {
	storageKey = strings.TrimSpace(storageKey)
	mimeType = strings.TrimSpace(strings.Split(mimeType, ";")[0])
	if !userStorageKeyPattern.MatchString(storageKey) {
		return "", "", safeMessageError{message: "文件存储编号无效"}
	}
	if size <= 0 || size > maxUserFileSize {
		return "", "", safeMessageError{message: "单个文件必须在 80MB 以内"}
	}
	if assetTypeFromMime(mimeType) == "" {
		return "", "", safeMessageError{message: "仅支持图片、视频或音频文件"}
	}
	return storageKey, mimeType, nil
}

func userFileObjectKey(userID, storageKey, mimeType string) string {
	parts := strings.SplitN(storageKey, ":", 2)
	return path.Join("users", userID, parts[0], parts[1]+assetFileExt(mimeType))
}

func ensureQiniuStorageConfigured() error {
	if strings.TrimSpace(config.Cfg.QiniuAccessKey) == "" || strings.TrimSpace(config.Cfg.QiniuSecretKey) == "" || strings.TrimSpace(config.Cfg.QiniuBucket) == "" || strings.TrimSpace(config.Cfg.QiniuDownloadDomain) == "" {
		return safeMessageError{message: "云端文件存储尚未配置"}
	}
	return nil
}

func qiniuMac() *qbox.Mac {
	return qbox.NewMac(config.Cfg.QiniuAccessKey, config.Cfg.QiniuSecretKey)
}

func qiniuBucketManager() *storage.BucketManager {
	cfg := storage.Config{UseHTTPS: true}
	return storage.NewBucketManager(qiniuMac(), &cfg)
}

func qiniuUploadURL() string {
	hosts := map[string]string{
		"z0": "https://upload.qiniup.com", "cn-east-2": "https://upload-cn-east-2.qiniup.com", "z1": "https://upload-z1.qiniup.com",
		"z2": "https://upload-z2.qiniup.com", "na0": "https://upload-na0.qiniup.com", "as0": "https://upload-as0.qiniup.com",
	}
	if host := hosts[strings.TrimSpace(config.Cfg.QiniuRegion)]; host != "" {
		return host
	}
	return hosts["as0"]
}
