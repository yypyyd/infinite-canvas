package service

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/qiniu/go-sdk/v7/auth/qbox"
	"github.com/qiniu/go-sdk/v7/client"
	"github.com/qiniu/go-sdk/v7/storage"
	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

type userStorageObjectInfo struct {
	Hash     string
	MimeType string
	Size     int64
}

func currentUserStorageSetting() (model.StorageSetting, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return model.StorageSetting{}, err
	}
	setting := normalizeStorageSetting(settings.Private.Storage)
	if err := validateStorageSetting(setting); err != nil {
		return model.StorageSetting{}, err
	}
	return setting, nil
}

func effectiveStorageDriver(driver string) string {
	if strings.EqualFold(strings.TrimSpace(driver), "local") {
		return "local"
	}
	return "qiniu"
}

func statUserStorageObject(setting model.StorageSetting, driver, objectKey string) (userStorageObjectInfo, error) {
	driver = effectiveStorageDriver(driver)
	if driver == "local" {
		path, err := localStorageObjectPath(setting, objectKey)
		if err != nil {
			return userStorageObjectInfo{}, err
		}
		file, err := os.Open(path)
		if err != nil {
			return userStorageObjectInfo{}, err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || info.IsDir() {
			return userStorageObjectInfo{}, errors.New("local storage object is invalid")
		}
		hash := sha256.New()
		if _, err := io.Copy(hash, file); err != nil {
			return userStorageObjectInfo{}, err
		}
		return userStorageObjectInfo{Hash: hex.EncodeToString(hash.Sum(nil)), MimeType: mimeTypeByObjectPath(objectKey), Size: info.Size()}, nil
	}
	info, err := qiniuBucketManager(setting).Stat(setting.QiniuBucket, objectKey)
	if err != nil {
		return userStorageObjectInfo{}, err
	}
	return userStorageObjectInfo{Hash: info.Hash, MimeType: info.MimeType, Size: info.Fsize}, nil
}

func putUserStorageObject(ctx context.Context, setting model.StorageSetting, driver, objectKey, mimeType, userID string, size int64, body io.Reader) (userStorageObjectInfo, error) {
	driver = effectiveStorageDriver(driver)
	buffered := bufio.NewReader(body)
	header, _ := buffered.Peek(int(min(size, 512)))
	detectedMime := strings.TrimSpace(strings.Split(http.DetectContentType(header), ";")[0])
	detectedType, expectedType := assetTypeFromMime(detectedMime), assetTypeFromMime(mimeType)
	if detectedType != expectedType && (detectedType != "" || detectedMime != "application/octet-stream") {
		return userStorageObjectInfo{}, errors.New("storage object MIME type mismatch")
	}
	body = buffered
	if driver == "local" {
		path, err := localStorageObjectPath(setting, objectKey)
		if err != nil {
			return userStorageObjectInfo{}, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return userStorageObjectInfo{}, err
		}
		temporary, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
		if err != nil {
			return userStorageObjectInfo{}, err
		}
		temporaryPath := temporary.Name()
		defer func() { _ = temporary.Close(); _ = os.Remove(temporaryPath) }()
		hash := sha256.New()
		limited := &io.LimitedReader{R: body, N: size + 1}
		written, err := io.Copy(io.MultiWriter(temporary, hash), limited)
		if err != nil || written != size || limited.N != 1 {
			return userStorageObjectInfo{}, errors.New("local storage object size mismatch")
		}
		if err := temporary.Sync(); err != nil {
			return userStorageObjectInfo{}, err
		}
		if err := temporary.Close(); err != nil {
			return userStorageObjectInfo{}, err
		}
		hashValue := hex.EncodeToString(hash.Sum(nil))
		if err := os.Rename(temporaryPath, path); err != nil {
			existing, statErr := statUserStorageObject(setting, driver, objectKey)
			if statErr != nil || existing.Size != size || existing.Hash != hashValue {
				return userStorageObjectInfo{}, err
			}
			return existing, nil
		}
		return userStorageObjectInfo{Hash: hashValue, MimeType: mimeType, Size: size}, nil
	}
	limited := &io.LimitedReader{R: body, N: size + 1}
	policy := storage.PutPolicy{Scope: setting.QiniuBucket + ":" + objectKey, Expires: 1800, FsizeMin: size, FsizeLimit: size, DetectMime: 1, MimeLimit: assetTypeFromMime(mimeType) + "/*", EndUser: userID}
	var uploaded storage.PutRet
	if err := storage.NewFormUploader(&storage.Config{UseHTTPS: true}).Put(ctx, &uploaded, policy.UploadToken(qiniuMac(setting)), objectKey, limited, size, &storage.PutExtra{MimeType: mimeType}); err != nil {
		return userStorageObjectInfo{}, err
	}
	extra, err := io.Copy(io.Discard, limited)
	if err != nil || extra != 0 {
		return userStorageObjectInfo{}, errors.New("cloud storage object size mismatch")
	}
	return statUserStorageObject(setting, driver, objectKey)
}

func deleteUserStorageObject(setting model.StorageSetting, driver, objectKey string) error {
	if effectiveStorageDriver(driver) == "local" {
		path, err := localStorageObjectPath(setting, objectKey)
		if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	err := qiniuBucketManager(setting).Delete(setting.QiniuBucket, objectKey)
	var qiniuError *client.ErrorInfo
	if errors.As(err, &qiniuError) && qiniuError.Code == 612 {
		return nil
	}
	return err
}

func localStorageObjectPath(setting model.StorageSetting, objectKey string) (string, error) {
	root, err := filepath.Abs(setting.LocalPath)
	if err != nil || filepath.Dir(root) == root {
		return "", errors.New("local storage root is invalid")
	}
	cleanKey := filepath.ToSlash(filepath.Clean(objectKey))
	if cleanKey == "." || strings.HasPrefix(cleanKey, "../") || strings.HasPrefix(cleanKey, "/") || strings.Contains(cleanKey, ":") {
		return "", errors.New("local storage object key is invalid")
	}
	target := filepath.Join(root, filepath.FromSlash(cleanKey))
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("local storage object escapes configured root")
	}
	return target, nil
}

func mimeTypeByObjectPath(objectKey string) string {
	switch strings.ToLower(filepath.Ext(objectKey)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	default:
		return "application/octet-stream"
	}
}

func qiniuMac(setting model.StorageSetting) *qbox.Mac {
	return qbox.NewMac(setting.QiniuAccessKey, setting.QiniuSecretKey)
}

func qiniuBucketManager(setting model.StorageSetting) *storage.BucketManager {
	return storage.NewBucketManager(qiniuMac(setting), &storage.Config{UseHTTPS: true})
}

func qiniuUploadURL(setting model.StorageSetting) string {
	hosts := map[string]string{"z0": "https://upload.qiniup.com", "cn-east-2": "https://upload-cn-east-2.qiniup.com", "z1": "https://upload-z1.qiniup.com", "z2": "https://upload-z2.qiniup.com", "na0": "https://upload-na0.qiniup.com", "as0": "https://upload-as0.qiniup.com"}
	if host := hosts[setting.QiniuRegion]; host != "" {
		return host
	}
	return hosts["as0"]
}

func signedLocalStorageReferenceURL(item model.UserFile, validity time.Duration) (string, bool) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.Cfg.PublicBaseURL), "/")
	if baseURL == "" || !validHTTPSURL(baseURL) || effectiveStorageDriver(item.StorageDriver) != "local" {
		return "", false
	}
	expires := time.Now().UTC().Add(validity).Unix()
	signature := localStorageReferenceSignature(item.ID, expires)
	return fmt.Sprintf("%s/api/media/storage/%s?expires=%d&signature=%s", baseURL, item.ID, expires, signature), true
}

func localStorageReferenceSignature(fileID string, expires int64) string {
	mac := hmac.New(sha256.New, []byte(config.Cfg.JWTSecret))
	_, _ = io.WriteString(mac, fileID+"|"+strconv.FormatInt(expires, 10))
	return hex.EncodeToString(mac.Sum(nil))
}

func OpenPublicLocalStorageReference(fileID string, expires int64, signature string) (*os.File, model.UserFile, error) {
	if fileID == "" || expires < time.Now().UTC().Unix() || expires > time.Now().UTC().Add(2*time.Hour).Unix() {
		return nil, model.UserFile{}, errors.New("local storage reference expired")
	}
	expected := localStorageReferenceSignature(fileID, expires)
	if len(signature) != len(expected) || subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) != 1 {
		return nil, model.UserFile{}, errors.New("local storage reference signature is invalid")
	}
	item, ok, err := repository.GetUserFileByID(fileID)
	if err != nil || !ok || effectiveStorageDriver(item.StorageDriver) != "local" {
		return nil, model.UserFile{}, errors.New("local storage reference does not exist")
	}
	setting, err := currentUserStorageSetting()
	if err != nil {
		return nil, model.UserFile{}, err
	}
	path, err := localStorageObjectPath(setting, item.ObjectKey)
	if err != nil {
		return nil, model.UserFile{}, err
	}
	file, err := os.Open(path)
	return file, item, err
}

func OpenLocalUserWorkspaceFile(user model.AuthUser, storageKey string) (*os.File, model.UserFile, bool) {
	item, ok, err := repository.GetUserFile(user.OrganizationID, strings.TrimSpace(storageKey))
	if err != nil || !ok || effectiveStorageDriver(item.StorageDriver) != "local" {
		return nil, model.UserFile{}, false
	}
	setting, err := currentUserStorageSetting()
	if err != nil {
		return nil, model.UserFile{}, false
	}
	path, err := localStorageObjectPath(setting, item.ObjectKey)
	if err != nil {
		return nil, model.UserFile{}, false
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, model.UserFile{}, false
	}
	return file, item, true
}

func UploadLocalUserWorkspaceFile(ctx context.Context, user model.AuthUser, uploadID, mimeType string, body io.Reader) error {
	if err := RequireOrganizationWrite(user); err != nil {
		return err
	}
	reservation, ok, err := repository.GetUserFileUploadReservation(user.OrganizationID, user.ID, strings.TrimSpace(uploadID))
	if err != nil {
		return err
	}
	if !ok || effectiveStorageDriver(reservation.StorageDriver) != "local" || assetTypeFromMime(mimeType) != assetTypeFromMime(reservation.MimeType) {
		return safeMessageError{message: "本地上传预留不存在或已失效"}
	}
	setting, err := currentUserStorageSetting()
	if err != nil {
		return err
	}
	_, err = putUserStorageObject(ctx, setting, "local", reservation.ObjectKey, reservation.MimeType, user.ID, reservation.Size, body)
	return err
}

func isStorageObjectNotFound(err error) bool {
	var qiniuError *client.ErrorInfo
	return errors.Is(err, os.ErrNotExist) || errors.As(err, &qiniuError) && qiniuError.Code == 612
}

func storageObjectHTTPStatus(err error) int {
	var qiniuError *client.ErrorInfo
	if errors.As(err, &qiniuError) {
		return qiniuError.Code
	}
	return http.StatusInternalServerError
}
