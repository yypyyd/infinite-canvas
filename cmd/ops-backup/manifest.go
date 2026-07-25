package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const backupFormatVersion = 1

type BackupManifest struct {
	FormatVersion int                  `json:"formatVersion"`
	CreatedAt     string               `json:"createdAt"`
	AppVersion    string               `json:"appVersion"`
	Database      DatabaseBackup       `json:"database"`
	ObjectStorage *ObjectStorageBackup `json:"objectStorage,omitempty"`
}

type DatabaseBackup struct {
	Driver string `json:"driver"`
	File   string `json:"file"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type ObjectStorageBackup struct {
	Provider string         `json:"provider"`
	Bucket   string         `json:"bucket"`
	Objects  []ObjectBackup `json:"objects"`
}

type ObjectBackup struct {
	Key         string `json:"key"`
	File        string `json:"file"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	QiniuHash   string `json:"qiniuHash"`
	MimeType    string `json:"mimeType"`
	StorageType int    `json:"storageType"`
}

func writeManifest(root string, manifest BackupManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	filename, err := safeBackupPath(root, "manifest.json")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0600)
}

func readManifest(root string) (BackupManifest, error) {
	filename, err := safeBackupPath(root, "manifest.json")
	if err != nil {
		return BackupManifest{}, err
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return BackupManifest{}, err
	}
	var manifest BackupManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return BackupManifest{}, err
	}
	if manifest.FormatVersion != backupFormatVersion {
		return BackupManifest{}, fmt.Errorf("unsupported backup format version")
	}
	return manifest, validateManifest(root, manifest)
}

func validateManifest(root string, manifest BackupManifest) error {
	if manifest.Database.Driver == "" || manifest.Database.File == "" || manifest.Database.Size < 0 || !validSHA256(manifest.Database.SHA256) {
		return errors.New("invalid database manifest")
	}
	if !strings.HasPrefix(filepath.ToSlash(manifest.Database.File), "database/") {
		return errors.New("invalid database backup path")
	}
	if _, err := safeBackupPath(root, manifest.Database.File); err != nil {
		return err
	}
	if manifest.ObjectStorage == nil {
		return nil
	}
	if manifest.ObjectStorage.Provider != "qiniu-kodo" || manifest.ObjectStorage.Bucket == "" {
		return errors.New("invalid object storage manifest")
	}
	keys, files := map[string]bool{}, map[string]bool{}
	for _, object := range manifest.ObjectStorage.Objects {
		if object.Key == "" || object.QiniuHash == "" || object.Size < 0 || !validSHA256(object.SHA256) || object.File != objectBackupFile(object.Key) {
			return errors.New("invalid object manifest")
		}
		if keys[object.Key] || files[object.File] {
			return errors.New("duplicate object manifest entry")
		}
		keys[object.Key], files[object.File] = true, true
		if _, err := safeBackupPath(root, object.File); err != nil {
			return err
		}
	}
	return nil
}

func safeBackupPath(root, relative string) (string, error) {
	localRelative := filepath.FromSlash(relative)
	if relative == "" || path.IsAbs(relative) || filepath.IsAbs(localRelative) || filepath.VolumeName(localRelative) != "" || strings.Contains(relative, "\\") {
		return "", errors.New("invalid backup path")
	}
	clean := path.Clean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != relative {
		return "", errors.New("invalid backup path")
	}
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootPath, err = filepath.EvalSymlinks(rootPath)
	if err != nil {
		return "", err
	}
	result := filepath.Join(rootPath, filepath.FromSlash(clean))
	if result != rootPath && !strings.HasPrefix(result, rootPath+string(filepath.Separator)) {
		return "", errors.New("backup path escapes root")
	}
	current := rootPath
	for _, part := range strings.Split(filepath.FromSlash(clean), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			break
		}
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("backup path contains symlink")
		}
	}
	return result, nil
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func objectBackupFile(key string) string {
	sum := sha256.Sum256([]byte(key))
	name := hex.EncodeToString(sum[:])
	return path.Join("objects", name[:2], name)
}

func fileDigest(filename string) (int64, string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return 0, "", err
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}

func verifyBackupFile(root, relative string, expectedSize int64, expectedSHA256 string) error {
	filename, err := safeBackupPath(root, relative)
	if err != nil {
		return err
	}
	size, digest, err := fileDigest(filename)
	if err != nil {
		return err
	}
	if size != expectedSize || !strings.EqualFold(digest, expectedSHA256) {
		return errors.New("backup file checksum mismatch")
	}
	return nil
}
