package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/basketikun/infinite-canvas/config"
	"github.com/qiniu/go-sdk/v7/auth/qbox"
	"github.com/qiniu/go-sdk/v7/client"
	"github.com/qiniu/go-sdk/v7/storage"
)

type objectRestoreAction struct {
	Object    ObjectBackup
	Overwrite bool
}

func ensureObjectStorageConfigured() error {
	if strings.TrimSpace(config.Cfg.QiniuAccessKey) == "" || strings.TrimSpace(config.Cfg.QiniuSecretKey) == "" || strings.TrimSpace(config.Cfg.QiniuBucket) == "" {
		return errors.New("qiniu credentials and bucket are required")
	}
	domain, err := url.Parse(strings.TrimSpace(config.Cfg.QiniuDownloadDomain))
	if err != nil || domain.Scheme != "https" || domain.Host == "" {
		return errors.New("qiniu download domain must use https")
	}
	return nil
}

func qiniuMac() *qbox.Mac {
	return qbox.NewMac(config.Cfg.QiniuAccessKey, config.Cfg.QiniuSecretKey)
}

func qiniuBucketManager() *storage.BucketManager {
	return storage.NewBucketManager(qiniuMac(), &storage.Config{UseHTTPS: true})
}

func backupObjects(ctx context.Context, root string) (*ObjectStorageBackup, error) {
	if err := ensureObjectStorageConfigured(); err != nil {
		return nil, err
	}
	manager := qiniuBucketManager()
	result := &ObjectStorageBackup{Provider: "qiniu-kodo", Bucket: config.Cfg.QiniuBucket, Objects: []ObjectBackup{}}
	marker := ""
	for {
		entries, _, nextMarker, hasNext, err := manager.ListFiles(config.Cfg.QiniuBucket, "", "", marker, 1000)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.Status != 0 {
				return nil, errors.New("disabled qiniu object cannot be represented safely")
			}
			object, err := downloadObject(ctx, root, entry)
			if err != nil {
				return nil, err
			}
			result.Objects = append(result.Objects, object)
		}
		if !hasNext {
			break
		}
		if nextMarker == "" || nextMarker == marker {
			return nil, errors.New("qiniu list marker did not advance")
		}
		marker = nextMarker
	}
	return result, nil
}

func downloadObject(ctx context.Context, root string, entry storage.ListItem) (ObjectBackup, error) {
	relative := objectBackupFile(entry.Key)
	filename, err := safeBackupPath(root, relative)
	if err != nil {
		return ObjectBackup{}, err
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return ObjectBackup{}, err
	}
	partial := filename + ".partial"
	defer os.Remove(partial)
	deadline := time.Now().Add(30 * time.Minute).Unix()
	downloadURL := storage.MakePrivateURL(qiniuMac(), strings.TrimRight(config.Cfg.QiniuDownloadDomain, "/"), entry.Key, deadline)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return ObjectBackup{}, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return ObjectBackup{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ObjectBackup{}, errors.New("qiniu object download failed")
	}
	file, err := os.OpenFile(partial, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return ObjectBackup{}, err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, entry.Fsize+1))
	closeErr := file.Close()
	if copyErr != nil {
		return ObjectBackup{}, copyErr
	}
	if closeErr != nil {
		return ObjectBackup{}, closeErr
	}
	if size != entry.Fsize {
		return ObjectBackup{}, errors.New("qiniu object size mismatch")
	}
	if err := os.Rename(partial, filename); err != nil {
		return ObjectBackup{}, err
	}
	return ObjectBackup{Key: entry.Key, File: relative, Size: size, SHA256: hex.EncodeToString(hash.Sum(nil)), QiniuHash: entry.Hash, MimeType: entry.MimeType, StorageType: entry.Type}, nil
}

func preflightObjects(root string, backup ObjectStorageBackup, overwrite bool) ([]objectRestoreAction, error) {
	if err := ensureObjectStorageConfigured(); err != nil {
		return nil, err
	}
	for _, object := range backup.Objects {
		if err := verifyBackupFile(root, object.File, object.Size, object.SHA256); err != nil {
			return nil, err
		}
	}
	manager := qiniuBucketManager()
	actions := make([]objectRestoreAction, 0, len(backup.Objects))
	for _, object := range backup.Objects {
		info, err := manager.Stat(config.Cfg.QiniuBucket, object.Key)
		if err == nil {
			action, err := planObjectRestore(true, info.Hash, info.Fsize, object, overwrite)
			if err != nil {
				return nil, err
			}
			if action != nil {
				actions = append(actions, *action)
			}
			continue
		}
		var qiniuError *client.ErrorInfo
		if !errors.As(err, &qiniuError) || qiniuError.Code != 612 {
			return nil, err
		}
		action, _ := planObjectRestore(false, "", 0, object, overwrite)
		actions = append(actions, *action)
	}
	return actions, nil
}

func planObjectRestore(exists bool, hash string, size int64, object ObjectBackup, overwrite bool) (*objectRestoreAction, error) {
	if !exists {
		return &objectRestoreAction{Object: object}, nil
	}
	if hash == object.QiniuHash && size == object.Size {
		return nil, nil
	}
	if !overwrite {
		return nil, errors.New("qiniu object conflict")
	}
	return &objectRestoreAction{Object: object, Overwrite: true}, nil
}

func restoreObjects(ctx context.Context, root string, actions []objectRestoreAction) error {
	uploader := storage.NewFormUploader(&storage.Config{UseHTTPS: true})
	for _, action := range actions {
		filename, err := safeBackupPath(root, action.Object.File)
		if err != nil {
			return err
		}
		policy := storage.PutPolicy{Scope: config.Cfg.QiniuBucket + ":" + action.Object.Key, Expires: 3600, FileType: action.Object.StorageType}
		if !action.Overwrite {
			policy.InsertOnly = 1
		}
		if err := uploader.PutFile(ctx, nil, policy.UploadToken(qiniuMac()), action.Object.Key, filename, &storage.PutExtra{MimeType: action.Object.MimeType}); err != nil {
			return fmt.Errorf("qiniu object upload failed: %w", err)
		}
		info, err := qiniuBucketManager().Stat(config.Cfg.QiniuBucket, action.Object.Key)
		if err != nil || info.Hash != action.Object.QiniuHash || info.Fsize != action.Object.Size {
			return errors.New("restored qiniu object verification failed")
		}
	}
	return nil
}
