package service

import (
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/yypyyd/infinite-canvas/config"
)

type UploadedAssetFile struct {
	URL      string `json:"url"`
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Size     int64  `json:"size"`
	Type     string `json:"type"`
}

const maxAssetFileSize = 80 << 20

func SaveAssetFile(file multipart.File, header *multipart.FileHeader) (UploadedAssetFile, error) {
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxAssetFileSize+1))
	if err != nil {
		return UploadedAssetFile{}, err
	}
	if len(data) == 0 {
		return UploadedAssetFile{}, safeMessageError{message: "文件不能为空"}
	}
	if len(data) > maxAssetFileSize {
		return UploadedAssetFile{}, safeMessageError{message: "文件不能超过 80MB"}
	}
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	assetType := assetTypeFromMime(mimeType)
	if assetType == "" {
		return UploadedAssetFile{}, safeMessageError{message: "仅支持图片、视频或音频文件"}
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		ext = assetFileExt(mimeType)
	}
	id := newID("assetfile")
	name := id + ext
	dir := assetFileDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return UploadedAssetFile{}, err
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return UploadedAssetFile{}, err
	}
	return UploadedAssetFile{URL: "/api/asset-files/" + name, Name: header.Filename, MimeType: mimeType, Size: int64(len(data)), Type: assetType}, nil
}

func AssetFilePath(name string) (string, string, bool) {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == "" {
		return "", "", false
	}
	path := filepath.Join(assetFileDir(), name)
	mimeType := assetFileMime(filepath.Ext(name))
	if _, err := os.Stat(path); err != nil {
		return "", "", false
	}
	return path, mimeType, true
}

func assetFileDir() string {
	dsn := strings.TrimSpace(config.Cfg.DatabaseDSN)
	if dsn == "" || dsn == ":memory:" || strings.HasPrefix(dsn, "file:") {
		return filepath.Join("data", "asset-files")
	}
	if index := strings.Index(dsn, "?"); index >= 0 {
		dsn = dsn[:index]
	}
	return filepath.Join(filepath.Dir(dsn), "asset-files")
}

func assetTypeFromMime(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.HasPrefix(mimeType, "video/"):
		return "video"
	case strings.HasPrefix(mimeType, "audio/"):
		return "audio"
	default:
		return ""
	}
}

func assetFileExt(mimeType string) string {
	switch mimeType {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "video/quicktime":
		return ".mov"
	case "video/webm":
		return ".webm"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/mpeg":
		return ".mp3"
	default:
		if strings.HasPrefix(mimeType, "video/") {
			return ".mp4"
		}
		if strings.HasPrefix(mimeType, "audio/") {
			return ".mp3"
		}
		return ".jpg"
	}
}

func assetFileMime(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
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
