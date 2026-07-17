package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/basketikun/infinite-canvas/config"
	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
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

func SaveUserWorkspaceFile(userID string, storageKey string, file multipart.File, header *multipart.FileHeader) (model.UserFile, error) {
	defer file.Close()
	storageKey = strings.TrimSpace(storageKey)
	if !userStorageKeyPattern.MatchString(storageKey) {
		return model.UserFile{}, safeMessageError{message: "文件存储编号无效"}
	}
	existing, exists, err := repository.GetUserFile(userID, storageKey)
	if err != nil {
		return model.UserFile{}, err
	}
	if exists {
		if _, err := os.Stat(existing.Path); err == nil {
			return existing, nil
		}
	}
	data, err := io.ReadAll(io.LimitReader(file, maxUserFileSize+1))
	if err != nil {
		return model.UserFile{}, err
	}
	if len(data) == 0 {
		return model.UserFile{}, safeMessageError{message: "文件不能为空"}
	}
	if len(data) > maxUserFileSize {
		return model.UserFile{}, safeMessageError{message: "单个文件不能超过 80MB"}
	}
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(data)
	}
	if assetTypeFromMime(mimeType) == "" {
		return model.UserFile{}, safeMessageError{message: "仅支持图片、视频或音频文件"}
	}
	hashBytes := sha256.Sum256(data)
	hash := hex.EncodeToString(hashBytes[:])
	duplicate, duplicated, err := repository.GetUserFileByHash(userID, hash)
	if err != nil {
		return model.UserFile{}, err
	}
	if !duplicated {
		used, _, err := repository.UserStorageUsage(userID)
		if err != nil {
			return model.UserFile{}, err
		}
		if used+int64(len(data)) > userStorageQuotaBytes() {
			return model.UserFile{}, safeMessageError{message: "账号云端存储空间不足"}
		}
	}
	ext := assetFileExt(mimeType)
	path := filepath.Join(userFileDir(), userID, hash[:2], hash[2:4], hash+ext)
	if duplicated {
		path = duplicate.Path
	}
	if _, err := os.Stat(path); err != nil {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return model.UserFile{}, err
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return model.UserFile{}, err
		}
	}
	timestamp := now()
	item := model.UserFile{ID: newID("file"), UserID: userID, StorageKey: storageKey, SHA256: hash, MimeType: mimeType, Size: int64(len(data)), Path: path, CreatedAt: timestamp, UpdatedAt: timestamp}
	if exists {
		item.ID = existing.ID
		item.CreatedAt = existing.CreatedAt
	}
	return repository.SaveUserFile(item)
}

func UserWorkspaceFilePath(userID string, storageKey string) (string, string, bool) {
	item, ok, err := repository.GetUserFile(userID, storageKey)
	if err != nil || !ok {
		return "", "", false
	}
	if _, err := os.Stat(item.Path); err != nil {
		return "", "", false
	}
	return item.Path, item.MimeType, true
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

func userFileDir() string {
	dsn := strings.TrimSpace(config.Cfg.DatabaseDSN)
	if dsn == "" || dsn == ":memory:" || strings.HasPrefix(dsn, "file:") {
		return filepath.Join("data", "user-files")
	}
	if index := strings.Index(dsn, "?"); index >= 0 {
		dsn = dsn[:index]
	}
	return filepath.Join(filepath.Dir(dsn), "user-files")
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
		if err := repository.DeleteUserFile(file.ID); err != nil {
			return err
		}
		remaining, err := repository.CountUserFileHash(userID, file.SHA256)
		if err != nil {
			return err
		}
		if remaining == 0 {
			_ = os.Remove(file.Path)
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
