package service

import (
	"encoding/json"
	"errors"
	"log"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/basketikun/infinite-canvas/config"
	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
	"github.com/qiniu/go-sdk/v7/auth/qbox"
	"github.com/qiniu/go-sdk/v7/client"
	"github.com/qiniu/go-sdk/v7/storage"
)

const (
	maxUserFileSize = 80 << 20
	userFileUploadLifetime = 20 * time.Minute
	userFileUploadCleanupGrace = 15 * time.Minute
)

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
	UploadID       string          `json:"uploadId,omitempty"`
	UploadURL      string          `json:"uploadUrl,omitempty"`
	UploadToken    string          `json:"uploadToken,omitempty"`
	ObjectKey      string          `json:"objectKey,omitempty"`
	ExpiresAt      string          `json:"expiresAt,omitempty"`
	File           *model.UserFile `json:"file,omitempty"`
}

type UserFileConfirmRequest struct {
	UploadID   string `json:"uploadId"`
	StorageKey string `json:"storageKey"`
	ObjectKey  string `json:"objectKey"`
	MimeType   string `json:"mimeType"`
	Size       int64  `json:"size"`
}

func PrepareUserWorkspaceFileUpload(user model.AuthUser, request UserFileUploadRequest) (UserFileUploadTicket, error) {
	if err := RequireOrganizationWrite(user); err != nil { return UserFileUploadTicket{}, err }
	organizationID, userID := user.OrganizationID, user.ID
	storageKey, mimeType, err := normalizeUserFileInput(request.StorageKey, request.MimeType, request.Size)
	if err != nil {
		return UserFileUploadTicket{}, err
	}
	if err := ensureQiniuStorageConfigured(); err != nil {
		return UserFileUploadTicket{}, err
	}
	requestTime := time.Now().UTC()
	timestamp := requestTime.Format(timestampLayout)
	if err := repository.ConsumeUserFileUploadRateLimit(organizationID, userID, requestTime.Truncate(time.Minute).Format(timestampLayout), timestamp); errors.Is(err, repository.ErrWorkspaceUploadRateExceeded) { return UserFileUploadTicket{}, safeMessageError{message: "上传请求过于频繁，请稍后重试"} } else if err != nil { return UserFileUploadTicket{}, err }
	existing, exists, err := repository.GetUserFile(organizationID, storageKey)
	if err != nil {
		return UserFileUploadTicket{}, err
	}
	replaceExisting := false
	if exists {
		info, statErr := qiniuBucketManager().Stat(config.Cfg.QiniuBucket, existing.ObjectKey)
		var qiniuError *client.ErrorInfo
		if errors.As(statErr, &qiniuError) && qiniuError.Code == 612 {
			replaceExisting = true
		} else if statErr != nil {
			return UserFileUploadTicket{}, statErr
		} else if existing.Hash == "" || info.Hash != existing.Hash || info.Fsize != existing.Size || assetTypeFromMime(info.MimeType) != assetTypeFromMime(existing.MimeType) {
			replaceExisting = true
		}
	}
	uploadID := newID("upload")
	objectKey := userFileUploadObjectKey(organizationID, uploadID, mimeType)
	expiresAtTime := requestTime.Add(userFileUploadLifetime)
	expiresAt := expiresAtTime.Format(timestampLayout)
	replaceObjectKey := ""
	if replaceExisting { replaceObjectKey = existing.ObjectKey }
	reservation := model.UserFileUploadReservation{ID: uploadID, OrganizationID: organizationID, UserID: userID, StorageKey: storageKey, ObjectKey: objectKey, MimeType: mimeType, Size: request.Size, ReplaceExisting: replaceExisting, ReplaceObjectKey: replaceObjectKey, ExpiresAt: expiresAt, CleanupAfter: expiresAtTime.Add(userFileUploadCleanupGrace).Format(timestampLayout), CreatedAt: timestamp}
	reservation, err = repository.ReserveUserFileUpload(reservation, userStorageQuotaBytes(), timestamp)
	if errors.Is(err, repository.ErrWorkspaceStorageQuotaExceeded) { return UserFileUploadTicket{}, safeMessageError{message: "企业云端存储空间不足"} }
	if errors.Is(err, repository.ErrWorkspaceTemporaryQuotaExceeded) { return UserFileUploadTicket{}, safeMessageError{message: "企业临时文件清理空间已满，请稍后重试"} }
	if errors.Is(err, repository.ErrWorkspaceFileLimitExceeded) { return UserFileUploadTicket{}, safeMessageError{message: "企业文件数量已达上限"} }
	if errors.Is(err, repository.ErrWorkspaceUploadReservationLimitExceeded) { return UserFileUploadTicket{}, safeMessageError{message: "企业临时文件数量已达上限，请稍后重试"} }
	if err != nil { return UserFileUploadTicket{}, err }
	policy := storage.PutPolicy{
		Scope:       config.Cfg.QiniuBucket + ":" + objectKey,
		Expires:     uint64(userFileUploadLifetime / time.Second),
		FsizeMin:    request.Size,
		FsizeLimit:  request.Size,
		DetectMime:  1,
		MimeLimit:   assetTypeFromMime(mimeType) + "/*",
		EndUser:     userID,
		ReturnBody:  `{"key":"$(key)","hash":"$(etag)","size":$(fsize),"mimeType":"$(mimeType)"}`,
	}
	return UserFileUploadTicket{UploadRequired: true, UploadID: reservation.ID, UploadURL: qiniuUploadURL(), UploadToken: policy.UploadToken(qiniuMac()), ObjectKey: objectKey, ExpiresAt: expiresAt}, nil
}

func ConfirmUserWorkspaceFileUpload(user model.AuthUser, request UserFileConfirmRequest) (model.UserFile, error) {
	if err := RequireOrganizationWrite(user); err != nil { return model.UserFile{}, err }
	organizationID, userID := user.OrganizationID, user.ID
	uploadID := strings.TrimSpace(request.UploadID)
	if uploadID == "" { return model.UserFile{}, safeMessageError{message: "上传预留编号无效"} }
	storageKey, mimeType, err := normalizeUserFileInput(request.StorageKey, request.MimeType, request.Size)
	if err != nil {
		return model.UserFile{}, err
	}
	if err := ensureQiniuStorageConfigured(); err != nil {
		return model.UserFile{}, err
	}
	reservation, ok, err := repository.GetUserFileUploadReservation(organizationID, userID, uploadID)
	if err != nil { return model.UserFile{}, err }
	if !ok || reservation.StorageKey != storageKey || reservation.MimeType != mimeType || reservation.Size != request.Size { return model.UserFile{}, safeMessageError{message: "上传预留不存在或已失效"} }
	expectedKey := reservation.ObjectKey
	if strings.TrimSpace(request.ObjectKey) != expectedKey {
		return model.UserFile{}, safeMessageError{message: "云端文件路径无效"}
	}
	info, err := qiniuBucketManager().Stat(config.Cfg.QiniuBucket, expectedKey)
	if err != nil {
		return model.UserFile{}, safeMessageError{message: "云端文件尚未上传完成"}
	}
	if info.Fsize != request.Size || assetTypeFromMime(info.MimeType) != assetTypeFromMime(reservation.MimeType) {
		return model.UserFile{}, safeMessageError{message: "云端文件信息校验失败"}
	}
	timestamp := now()
	saved, err := repository.ConfirmUserFileUpload(organizationID, userID, uploadID, newID("file"), info.Hash, info.MimeType, info.Fsize, userStorageQuotaBytes(), timestamp)
	if errors.Is(err, repository.ErrWorkspaceUploadReservationUnavailable) { return model.UserFile{}, safeMessageError{message: "上传预留不存在或已过期，请重新上传"} }
	if errors.Is(err, repository.ErrWorkspaceStorageQuotaExceeded) { return model.UserFile{}, safeMessageError{message: "企业云端存储空间不足"} }
	if errors.Is(err, repository.ErrWorkspaceFileConflict) { return model.UserFile{}, safeMessageError{message: "文件存储编号已对应其他内容，请重新生成编号后上传"} }
	if err != nil { return model.UserFile{}, err }
	return saved, nil
}

func CancelUserWorkspaceFileUpload(user model.AuthUser, uploadID string) error {
	organization, _, err := EnsureOrganization(user)
	if err != nil { return err }
	uploadID = strings.TrimSpace(uploadID)
	if uploadID == "" { return safeMessageError{message: "上传预留编号无效"} }
	return repository.CancelUserFileUploadReservation(organization.ID, user.ID, uploadID, now())
}

func UserWorkspaceFileURL(user model.AuthUser, storageKey string) (string, bool) {
	return organizationFileURL(user.OrganizationID, storageKey, 10*time.Minute)
}

func organizationFileURL(organizationID string, storageKey string, validity time.Duration) (string, bool) {
	item, ok, err := repository.GetUserFile(organizationID, storageKey)
	if err != nil || !ok || strings.TrimSpace(item.ObjectKey) == "" || ensureQiniuStorageConfigured() != nil {
		return "", false
	}
	deadline := time.Now().Add(validity).Unix()
	return storage.MakePrivateURL(qiniuMac(), strings.TrimRight(config.Cfg.QiniuDownloadDomain, "/"), item.ObjectKey, deadline), true
}

func UserWorkspaceFileExists(user model.AuthUser, storageKey string) bool {
	organizationID := user.OrganizationID
	item, ok, err := repository.GetUserFile(organizationID, storageKey)
	if err != nil || !ok || ensureQiniuStorageConfigured() != nil {
		return false
	}
	info, err := qiniuBucketManager().Stat(config.Cfg.QiniuBucket, item.ObjectKey)
	return err == nil && item.Hash != "" && info.Hash == item.Hash && info.Fsize == item.Size && assetTypeFromMime(info.MimeType) == assetTypeFromMime(item.MimeType)
}

func GetUserStorageStatus(user model.AuthUser) (UserStorageStatus, error) {
	organizationID := user.OrganizationID
	used, count, err := repository.UserStorageUsage(organizationID)
	if err != nil {
		return UserStorageStatus{}, err
	}
	projects, assets, err := repository.UserWorkspaceCounts(organizationID)
	if err != nil {
		return UserStorageStatus{}, err
	}
	state, _, err := repository.GetUserWorkspaceState(organizationID)
	status := UserStorageStatus{UsedBytes: used, QuotaBytes: userStorageQuotaBytes(), FileCount: count, ProjectCount: int(projects), AssetCount: int(assets)}
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

func cleanupUserWorkspaceFiles(organizationID string) error {
	timestamp := now()
	return repository.CollectUserFileGarbage(organizationID, timestamp, time.Now().UTC().Add(-24*time.Hour).Format(timestampLayout), userStorageQuotaBytes())
}

func cleanupPendingUserObjects() error {
	for index := 0; index < 50; index++ {
		claimedAt := time.Now().UTC()
		item, claimed, err := repository.ClaimUserObjectDeletion(claimedAt.Format(timestampLayout), claimedAt.Add(2*time.Minute).Format(timestampLayout), newID("delete-lease"))
		if err != nil { return err }
		if !claimed { return nil }
		deletionErr := qiniuBucketManager().Delete(config.Cfg.QiniuBucket, item.ObjectKey)
		var qiniuError *client.ErrorInfo
		if errors.As(deletionErr, &qiniuError) && qiniuError.Code == 612 { deletionErr = nil }
		message := ""
		if deletionErr != nil { message = "对象存储删除失败"; log.Printf("delete workspace object failed deletion=%s error_type=%T", item.ID, deletionErr) }
		delay := time.Minute * time.Duration(1<<min(item.Attempts-1, 6))
		if err := repository.FinishUserObjectDeletion(item, deletionErr == nil, message, time.Now().UTC().Add(delay).Format(timestampLayout), now()); err != nil { return err }
	}
	return nil
}

func StartUserFileMaintenanceWorker() {
	go func() {
		for {
			timestamp := now()
			cutoff := time.Now().UTC().Add(-24*time.Hour).Format(timestampLayout)
			organizationIDs, err := repository.ListUserFileMaintenanceOrganizations(timestamp, cutoff)
			if err != nil { log.Printf("list workspace file maintenance organizations failed: %v", err) }
			for _, organizationID := range organizationIDs {
				if err := cleanupUserWorkspaceFiles(organizationID); err != nil { log.Printf("cleanup workspace files failed organization=%s err=%v", organizationID, err) }
			}
			if err := cleanupPendingUserObjects(); err != nil { log.Printf("cleanup workspace objects failed: %v", err) }
			time.Sleep(time.Minute)
		}
	}()
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
	if len(storageKey) > 191 || !userStorageKeyPattern.MatchString(storageKey) {
		return "", "", safeMessageError{message: "文件存储编号无效"}
	}
	if size <= 0 || size > maxUserFileSize {
		return "", "", safeMessageError{message: "单个文件必须在 80MB 以内"}
	}
	assetType := assetTypeFromMime(mimeType)
	if assetType == "" {
		return "", "", safeMessageError{message: "仅支持图片、视频或音频文件"}
	}
	prefix, suffix, _ := strings.Cut(storageKey, ":")
	if strings.HasPrefix(strings.ToLower(suffix), "batch-result-") { return "", "", safeMessageError{message: "文件存储编号使用了系统保留命名空间"} }
	expectedType := map[string]string{"image": "image", "video": "video", "video-reference": "video", "audio": "audio", "audio-reference": "audio"}[prefix]
	if expectedType != "" && expectedType != assetType { return "", "", safeMessageError{message: "文件类型与存储编号不匹配"} }
	return storageKey, mimeType, nil
}

func userFileUploadObjectKey(organizationID, uploadID, mimeType string) string {
	return path.Join("organizations", organizationID, "uploads", uploadID+assetFileExt(mimeType))
}

func batchProductionResultObjectKey(organizationID, uploadID, mimeType string) string {
	return path.Join("organizations", organizationID, "batch-results", uploadID+assetFileExt(mimeType))
}

func ensureQiniuStorageConfigured() error {
	if strings.TrimSpace(config.Cfg.QiniuAccessKey) == "" || strings.TrimSpace(config.Cfg.QiniuSecretKey) == "" || strings.TrimSpace(config.Cfg.QiniuBucket) == "" || strings.TrimSpace(config.Cfg.QiniuDownloadDomain) == "" {
		return safeMessageError{message: "云端文件存储尚未配置"}
	}
	if !validHTTPSURL(config.Cfg.QiniuDownloadDomain) { return safeMessageError{message: "七牛私有下载域名必须使用 HTTPS"} }
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
