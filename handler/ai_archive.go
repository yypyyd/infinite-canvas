package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/service"
)

func archiveImageGenerationResponse(ctx context.Context, user model.AuthUser, task model.GenerationTask, body []byte) ([]byte, []string) {
	return archiveImageGenerationResponseWithOptions(ctx, user, task, body, "", "")
}

func archiveImageGenerationResponseWithOptions(ctx context.Context, user model.AuthUser, task model.GenerationTask, body []byte, responseFormat, authorization string) ([]byte, []string) {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return body, nil
	}
	items, ok := payload["data"].([]any)
	if !ok {
		return body, nil
	}
	storageKeys := make([]string, 0, len(items))
	transformed := false
	for index, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		data, mimeType, err := generatedImageBytesWithAuthorization(ctx, item, authorization)
		if err != nil {
			log.Printf("AI image archive read failed: task=%s index=%d err=%v", task.ID, index+1, err)
			continue
		}
		if strings.EqualFold(strings.TrimSpace(responseFormat), "b64_json") {
			item["b64_json"] = base64.StdEncoding.EncodeToString(data)
			delete(item, "url")
			transformed = true
		}
		file, err := service.ArchiveGeneratedFile(ctx, user, task.ID, "image", index+1, mimeType, int64(len(data)), bytes.NewReader(data))
		if err != nil {
			log.Printf("AI image archive save failed: task=%s index=%d err=%v", task.ID, index+1, err)
			continue
		}
		item["storage_key"], item["mime_type"], item["bytes"] = file.StorageKey, file.MimeType, file.Size
		storageKeys = append(storageKeys, file.StorageKey)
	}
	if !transformed && len(storageKeys) == 0 {
		return body, nil
	}
	archived, err := json.Marshal(payload)
	if err != nil {
		return body, nil
	}
	return archived, storageKeys
}

func generatedArchiveContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), 15*time.Minute)
}

func generatedImageBytes(ctx context.Context, item map[string]any) ([]byte, string, error) {
	return generatedImageBytesWithAuthorization(ctx, item, "")
}

func generatedImageBytesWithAuthorization(ctx context.Context, item map[string]any, authorization string) ([]byte, string, error) {
	if encoded, _ := item["b64_json"].(string); strings.TrimSpace(encoded) != "" {
		mimeType := "image/png"
		if comma := strings.Index(encoded, ","); comma >= 0 && strings.Contains(strings.ToLower(encoded[:comma]), "base64") {
			if prefix := strings.TrimPrefix(encoded[:comma], "data:"); strings.HasPrefix(prefix, "image/") {
				mimeType = strings.TrimSuffix(prefix, ";base64")
			}
			encoded = encoded[comma+1:]
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(data) == 0 {
			return nil, "", errors.New("generated image base64 is invalid")
		}
		if detected := http.DetectContentType(data); strings.HasPrefix(detected, "image/") {
			mimeType = detected
		}
		return data, mimeType, nil
	}
	if rawURL, _ := item["url"].(string); strings.TrimSpace(rawURL) != "" {
		return service.ReadGeneratedURLWithAuthorization(ctx, rawURL, "image", authorization)
	}
	return nil, "", errors.New("generated image is missing")
}

func archiveCompletedVideo(ctx context.Context, user model.AuthUser, task model.GenerationTask, request *http.Request, body []byte) ([]byte, model.UserFile, error) {
	if len(task.StorageKeys) > 0 {
		return attachVideoStorage(body, task.StorageKeys[0], "", 0), model.UserFile{StorageKey: task.StorageKeys[0]}, nil
	}
	contentURL := strings.TrimSuffix(request.URL.String(), "/") + "/content"
	contentRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, contentURL, nil)
	if err != nil {
		return body, model.UserFile{}, err
	}
	contentRequest.Header.Set("Authorization", request.Header.Get("Authorization"))
	response, err := http.DefaultClient.Do(contentRequest)
	if err != nil {
		return body, model.UserFile{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return body, model.UserFile{}, errors.New("generated video content is unavailable")
	}
	mimeType := strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = "video/mp4"
	}
	if !strings.HasPrefix(mimeType, "video/") {
		return body, model.UserFile{}, errors.New("generated video metadata is invalid")
	}
	file, err := service.ArchiveGeneratedStream(ctx, user, task.ID, "video", 1, mimeType, response.ContentLength, response.Body)
	if err != nil {
		return body, model.UserFile{}, err
	}
	return attachVideoStorage(body, file.StorageKey, file.MimeType, file.Size), file, nil
}

func archiveVideoContentResponse(ctx context.Context, user model.AuthUser, task model.GenerationTask, response *http.Response, body []byte) (model.UserFile, error) {
	mimeType := strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(body)
	}
	if !strings.HasPrefix(mimeType, "video/") {
		mimeType = "video/mp4"
	}
	return service.ArchiveGeneratedFile(ctx, user, task.ID, "video", 1, mimeType, int64(len(body)), bytes.NewReader(body))
}

func setArchivedGenerationHeaders(w http.ResponseWriter, file model.UserFile) {
	if file.StorageKey == "" {
		return
	}
	w.Header().Set("X-Storage-Key", file.StorageKey)
	if file.Size > 0 {
		w.Header().Set("X-Storage-Bytes", fmt.Sprint(file.Size))
	}
	if file.MimeType != "" {
		w.Header().Set("X-Storage-Mime-Type", file.MimeType)
	}
}

func attachVideoStorage(body []byte, storageKey, mimeType string, size int64) []byte {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return body
	}
	target := payload
	if data, ok := payload["data"].(map[string]any); ok {
		target = data
	}
	target["status"] = "completed"
	target["storage_key"] = storageKey
	if mimeType != "" {
		target["mime_type"] = mimeType
	}
	if size > 0 {
		target["bytes"] = size
	}
	result, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return result
}

func proxyArchivedGenerationContent(w http.ResponseWriter, r *http.Request, user model.AuthUser, storageKey string) bool {
	fileURL, ok := service.UserWorkspaceFileURL(user, storageKey, "")
	if !ok {
		return false
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, fileURL, nil)
	if err != nil {
		return false
	}
	if byteRange := r.Header.Get("Range"); byteRange != "" {
		request.Header.Set("Range", byteRange)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false
	}
	w.Header().Set("X-Storage-Key", storageKey)
	for key, values := range response.Header {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
	return true
}
