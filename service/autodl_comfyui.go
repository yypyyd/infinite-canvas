package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

const autoDLRequestTimeout = 60 * time.Second

var autoDLHTTPClient = &http.Client{Timeout: autoDLRequestTimeout, CheckRedirect: func(request *http.Request, via []*http.Request) error {
	if len(via) >= 3 || request.URL.Scheme != "https" || len(via) == 0 || !strings.EqualFold(request.URL.Host, via[0].URL.Host) {
		return errors.New("AutoDL API redirect is not allowed")
	}
	return nil
}}

type AutoDLReferenceFile struct {
	MimeType string
	Data     []byte
}

type AutoDLGenerationInput struct {
	Prompt                    string
	NegativePrompt            string
	Size                      string
	Ratio                     string
	Resolution                string
	ResolutionTier            string
	Seconds                   int
	Count                     int
	Seed                      string
	GenerateAudio             bool
	ReferenceImageStorageKeys []string
	ReferenceVideoStorageKeys []string
	ReferenceAudioStorageKeys []string
	ReferenceImages           []AutoDLReferenceFile
}

type autoDLTaskOutput struct {
	URL        string `json:"url"`
	Type       string `json:"type"`
	FileType   string `json:"file_type"`
	OutputType string `json:"output_type"`
}

type autoDLTaskResult struct {
	Status  string
	TaskID  string
	Outputs []autoDLTaskOutput
	Error   string
}

func IsAutoDLComfyUIChannel(channel model.ModelChannel) bool {
	return channel.Protocol == modelChannelProtocolAutoDLComfyUI
}

func testAutoDLComfyUICredentials(ctx context.Context, channel model.ModelChannel) error {
	endpoint := normalizeModelChannelBaseURL(channel.BaseURL) + "/api/v1/comfyui/workflow/tasks?page_size=1&page_index=1"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", channel.APIKey)
	response, err := autoDLHTTPClient.Do(request)
	if err != nil {
		return safeMessageError{message: "AutoDL 权限校验失败：上游无响应或网络不可达"}
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxGenerationTaskStatusSize+1))
	if len(body) > maxGenerationTaskStatusSize {
		return safeMessageError{message: "AutoDL 权限校验响应过大"}
	}
	var payload struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return safeMessageError{message: "AutoDL 权限校验响应无效"}
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden || strings.EqualFold(payload.Code, "AuthorizeFailed") {
		return safeMessageError{message: "AutoDL Token 无效或没有 ComfyUI API 权限"}
	}
	if response.StatusCode >= http.StatusBadRequest || !strings.EqualFold(payload.Code, "Success") {
		return autoDLResponseError(response.StatusCode, payload.Msg, "AutoDL 权限校验失败")
	}
	return nil
}

func SubmitAutoDLComfyUITask(ctx context.Context, user model.AuthUser, task model.GenerationTask, selection ModelChannelSelection, input AutoDLGenerationInput) (string, []byte, error) {
	workflow := selection.Model.Workflow
	if err := validateComfyUIWorkflow(workflow); err != nil {
		return "", nil, safeMessageError{message: "AutoDL 工作流配置无效：" + err.Error()}
	}
	values, err := autoDLTemplateValues(ctx, user, task, input)
	if err != nil {
		return "", nil, err
	}
	payload, err := renderAutoDLRequest(*workflow, values)
	if err != nil {
		return "", nil, safeMessageError{message: "AutoDL 请求模板渲染失败：" + err.Error()}
	}
	endpoint := normalizeModelChannelBaseURL(selection.Channel.BaseURL) + "/api/v1/comfyui/comfyui_workflow/" + url.PathEscape(workflow.WorkflowID)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", nil, err
	}
	request.Header.Set("Authorization", selection.Channel.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := autoDLHTTPClient.Do(request)
	if err != nil {
		return "", nil, safeMessageError{message: "AutoDL 任务提交失败：上游无响应或网络不可达"}
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxGenerationTaskStatusSize+1))
	if len(body) > maxGenerationTaskStatusSize {
		return "", nil, safeMessageError{message: "AutoDL 任务提交响应过大"}
	}
	result, err := parseAutoDLTaskResponse(body)
	if response.StatusCode >= http.StatusBadRequest || err != nil || result.TaskID == "" {
		return "", nil, autoDLResponseError(response.StatusCode, result.Error, "AutoDL 任务提交失败")
	}
	return result.TaskID, autoDLStatusBody(task.Modality, result.TaskID, result.Status), nil
}

// ReconcileAutoDLGenerationTask queries, archives, and settles one task without exposing temporary URLs.
func ReconcileAutoDLGenerationTask(ctx context.Context, task *model.GenerationTask, channel model.ModelChannel) (bool, error) {
	if task == nil || task.UpstreamTaskID == "" {
		return false, errors.New("AutoDL task ID is missing")
	}
	if len(task.StorageKeys) > 0 {
		task.Status = model.GenerationTaskStatusSuccess
		return true, FinishGenerationTask(*task, model.GenerationTaskStatusSuccess, "")
	}
	result, err := queryAutoDLComfyUITask(ctx, channel, task.UpstreamTaskID)
	if err != nil {
		return false, err
	}
	switch result.Status {
	case "failed":
		message := strings.TrimSpace(result.Error)
		if message == "" {
			message = "AutoDL 生成失败"
		}
		task.Status, task.ErrorMessage = model.GenerationTaskStatusFailed, message
		return true, FinishGenerationTask(*task, model.GenerationTaskStatusFailed, message)
	case "success":
		return archiveAutoDLTaskOutputs(ctx, task, result.Outputs)
	default:
		body := autoDLStatusBody(task.Modality, task.UpstreamTaskID, result.Status)
		if string(body) != task.ResultJSON {
			if err := UpdateGenerationTaskRecovery(task, task.UpstreamTaskID, body, nil); err != nil {
				return false, err
			}
		}
		return false, nil
	}
}

func queryAutoDLComfyUITask(ctx context.Context, channel model.ModelChannel, taskID string) (autoDLTaskResult, error) {
	endpoint := normalizeModelChannelBaseURL(channel.BaseURL) + "/api/v1/comfyui/comfyui_workflow/result/" + url.PathEscape(taskID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return autoDLTaskResult{}, err
	}
	request.Header.Set("Authorization", channel.APIKey)
	response, err := autoDLHTTPClient.Do(request)
	if err != nil {
		return autoDLTaskResult{}, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxGenerationTaskStatusSize+1))
	if len(body) > maxGenerationTaskStatusSize {
		return autoDLTaskResult{}, errors.New("AutoDL task status response is too large")
	}
	result, parseErr := parseAutoDLTaskResponse(body)
	if response.StatusCode >= http.StatusBadRequest || parseErr != nil {
		return result, autoDLResponseError(response.StatusCode, result.Error, "AutoDL 任务状态查询失败")
	}
	return result, nil
}

func parseAutoDLTaskResponse(body []byte) (autoDLTaskResult, error) {
	var payload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Msg     string `json:"msg"`
		Data    struct {
			TaskID  string             `json:"task_id"`
			Status  string             `json:"status"`
			Results []autoDLTaskOutput `json:"results"`
			Error   any                `json:"error"`
			Message string             `json:"message"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return autoDLTaskResult{}, errors.New("invalid AutoDL response")
	}
	errorMessage := firstPricingNonEmpty(payload.Data.Message, autoDLErrorText(payload.Data.Error), payload.Message, payload.Msg)
	if errorMessage == "<nil>" {
		errorMessage = ""
	}
	errorMessage = safeAutoDLError(errorMessage)
	result := autoDLTaskResult{TaskID: strings.TrimSpace(payload.Data.TaskID), Status: normalizeAutoDLStatus(payload.Data.Status), Outputs: payload.Data.Results, Error: errorMessage}
	if payload.Code != "" && !strings.EqualFold(payload.Code, "success") {
		return result, errors.New("AutoDL response code indicates failure")
	}
	if result.Status == "success" && len(result.Outputs) == 0 {
		result.Error = "AutoDL 任务成功但未返回生成结果"
		result.Status = "failed"
	}
	return result, nil
}

func normalizeAutoDLStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "succeeded", "completed", "done":
		return "success"
	case "failed", "failure", "cancelled", "canceled", "error":
		return "failed"
	case "running", "processing", "in_progress":
		return "running"
	default:
		return "queued"
	}
}

func autoDLTemplateValues(ctx context.Context, user model.AuthUser, task model.GenerationTask, input AutoDLGenerationInput) (map[string]any, error) {
	imageURLs, err := autoDLStorageURLs(user, input.ReferenceImageStorageKeys)
	if err != nil {
		return nil, err
	}
	for index, reference := range input.ReferenceImages {
		if len(reference.Data) == 0 || len(reference.Data) > 20<<20 {
			return nil, safeMessageError{message: "AutoDL 参考图必须在 20MB 以内"}
		}
		mimeType := strings.TrimSpace(strings.Split(reference.MimeType, ";")[0])
		if detected := http.DetectContentType(reference.Data); strings.HasPrefix(detected, "image/") {
			mimeType = detected
		}
		if !strings.HasPrefix(mimeType, "image/") {
			return nil, safeMessageError{message: "AutoDL 参考素材必须是图片"}
		}
		file, archiveErr := ArchiveGeneratedFile(ctx, user, task.ID+"-reference", "image", index+1, mimeType, int64(len(reference.Data)), bytes.NewReader(reference.Data))
		if archiveErr != nil {
			return nil, archiveErr
		}
		media, resolveErr := ResolveUserWorkspaceMedia(user, file.StorageKey)
		if resolveErr != nil {
			return nil, resolveErr
		}
		imageURLs = append(imageURLs, media.URL)
	}
	videoURLs, err := autoDLStorageURLs(user, input.ReferenceVideoStorageKeys)
	if err != nil {
		return nil, err
	}
	audioURLs, err := autoDLStorageURLs(user, input.ReferenceAudioStorageKeys)
	if err != nil {
		return nil, err
	}
	width, height, dimensions := parsePricingDimensions(input.Size)
	ratio := strings.TrimSpace(input.Ratio)
	if !strings.Contains(ratio, ":") {
		ratio = normalizeVideoAspectRatio(input.Size)
	}
	values := map[string]any{
		"prompt": input.Prompt, "negative_prompt": input.NegativePrompt, "size": input.Size, "ratio": ratio,
		"resolution": firstPricingNonEmpty(input.Resolution, input.ResolutionTier), "seconds": input.Seconds,
		"count": input.Count, "seed": input.Seed, "generate_audio": input.GenerateAudio,
		"reference_image_urls": imageURLs, "reference_video_urls": videoURLs, "reference_audio_urls": audioURLs,
	}
	if dimensions {
		values["width"], values["height"] = width, height
	}
	if len(imageURLs) > 0 {
		values["reference_image_url"] = imageURLs[0]
		for index, value := range imageURLs {
			if index >= 16 {
				break
			}
			values["reference_image_"+strconv.Itoa(index)] = value
		}
	}
	for index, value := range audioURLs {
		if index >= 3 {
			break
		}
		values["reference_audio_"+strconv.Itoa(index)] = value
	}
	return values, nil
}

func autoDLStorageURLs(user model.AuthUser, keys []string) ([]string, error) {
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		media, err := ResolveUserWorkspaceMedia(user, key)
		if err != nil {
			return nil, err
		}
		result = append(result, media.URL)
	}
	return result, nil
}

func renderAutoDLRequest(workflow model.ComfyUIWorkflowConfig, values map[string]any) ([]byte, error) {
	var template map[string]any
	if err := json.Unmarshal([]byte(workflow.RequestTemplate), &template); err != nil {
		return nil, err
	}
	var valueMaps map[string]map[string]any
	if err := json.Unmarshal([]byte(workflow.ValueMaps), &valueMaps); err != nil {
		return nil, err
	}
	rendered, _, err := renderAutoDLValue(template, values, valueMaps)
	if err != nil {
		return nil, err
	}
	return json.Marshal(rendered)
}

func renderAutoDLValue(value any, values map[string]any, valueMaps map[string]map[string]any) (any, bool, error) {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			rendered, exists, err := renderAutoDLValue(child, values, valueMaps)
			if err != nil {
				return nil, false, err
			}
			if exists {
				result[key] = rendered
			}
		}
		return result, true, nil
	case []any:
		result := make([]any, 0, len(typed))
		for _, child := range typed {
			rendered, exists, err := renderAutoDLValue(child, values, valueMaps)
			if err != nil {
				return nil, false, err
			}
			if exists {
				result = append(result, rendered)
			}
		}
		return result, true, nil
	case string:
		if !isComfyUIPlaceholder(typed) {
			return typed, true, nil
		}
		name := strings.TrimSuffix(strings.TrimPrefix(typed, "${"), "}")
		rendered, exists := values[name]
		if !exists || rendered == "" || isEmptyAutoDLSlice(rendered) {
			return nil, false, nil
		}
		if mapped, ok := valueMaps[name][fmt.Sprint(rendered)]; ok {
			rendered = mapped
		}
		return rendered, true, nil
	default:
		return value, true, nil
	}
}

func isEmptyAutoDLSlice(value any) bool {
	switch typed := value.(type) {
	case []string:
		return len(typed) == 0
	case []any:
		return len(typed) == 0
	}
	return false
}

func archiveAutoDLTaskOutputs(ctx context.Context, task *model.GenerationTask, outputs []autoDLTaskOutput) (bool, error) {
	user, exists, err := repository.GetUserByID(task.UserID)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, errors.New("AutoDL task user does not exist")
	}
	authUser := model.PublicUser(user)
	authUser.OrganizationID = task.OrganizationID
	files := make([]model.UserFile, 0, len(outputs))
	for _, output := range outputs {
		outputType := strings.ToLower(strings.TrimSpace(output.Type))
		if outputType != "" && outputType != task.Modality {
			continue
		}
		file, downloadErr := archiveAutoDLOutput(ctx, authUser, *task, len(files)+1, output)
		if downloadErr != nil {
			return false, downloadErr
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		return false, errors.New("AutoDL task returned no matching media output")
	}
	storageKeys := make([]string, 0, len(files))
	for _, file := range files {
		storageKeys = append(storageKeys, file.StorageKey)
	}
	body := autoDLArchivedBody(*task, files)
	if err := UpdateGenerationTaskRecovery(task, task.UpstreamTaskID, body, storageKeys); err != nil {
		return false, err
	}
	if err := FinishGenerationTask(*task, model.GenerationTaskStatusSuccess, ""); err != nil {
		return false, err
	}
	task.Status = model.GenerationTaskStatusSuccess
	return true, nil
}

func archiveAutoDLOutput(ctx context.Context, user model.AuthUser, task model.GenerationTask, index int, output autoDLTaskOutput) (model.UserFile, error) {
	if !validHTTPSURL(output.URL) {
		return model.UserFile{}, errors.New("AutoDL output URL is invalid")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, output.URL, nil)
	if err != nil {
		return model.UserFile{}, err
	}
	transport := batchResultTransport()
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Minute, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !validHTTPSURL(request.URL.String()) {
			return errors.New("AutoDL output redirect is not allowed")
		}
		return nil
	}}
	response, err := client.Do(request)
	if err != nil {
		return model.UserFile{}, errors.New("AutoDL output download failed")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return model.UserFile{}, fmt.Errorf("AutoDL output returned HTTP %d", response.StatusCode)
	}
	mimeType := strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = autoDLOutputMIME(output)
	}
	if assetTypeFromMime(mimeType) != task.Modality {
		return model.UserFile{}, errors.New("AutoDL output MIME does not match task modality")
	}
	return ArchiveGeneratedStream(ctx, user, task.ID, task.Modality, index, mimeType, response.ContentLength, response.Body)
}

func autoDLOutputMIME(output autoDLTaskOutput) string {
	extension := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(output.FileType), "."))
	switch extension {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	case "gif":
		return "image/gif"
	case "mp4":
		return "video/mp4"
	case "webm":
		return "video/webm"
	default:
		if strings.EqualFold(output.Type, "video") {
			return "video/mp4"
		}
		return "image/png"
	}
}

func autoDLStatusBody(modality, taskID, status string) []byte {
	normalized := normalizeAutoDLStatus(status)
	if normalized == "success" {
		normalized = "completed"
	}
	if modality == "image" {
		body, _ := json.Marshal(map[string]any{"id": taskID, "status": normalized, "data": []any{}})
		return body
	}
	body, _ := json.Marshal(map[string]any{"id": taskID, "status": normalized})
	return body
}

func autoDLArchivedBody(task model.GenerationTask, files []model.UserFile) []byte {
	if task.Modality == "image" {
		items := make([]map[string]any, 0, len(files))
		for _, file := range files {
			items = append(items, map[string]any{"storage_key": file.StorageKey, "mime_type": file.MimeType, "bytes": file.Size})
		}
		body, _ := json.Marshal(map[string]any{"id": task.UpstreamTaskID, "status": "completed", "data": items})
		return body
	}
	file := files[0]
	body, _ := json.Marshal(map[string]any{"id": task.UpstreamTaskID, "status": "completed", "storage_key": file.StorageKey, "mime_type": file.MimeType, "bytes": file.Size})
	return body
}

func autoDLResponseError(statusCode int, message, fallback string) error {
	message = safeAutoDLError(message)
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		message = "AutoDL Token 无效、没有 ComfyUI API 权限或没有该工作流权限"
	} else if statusCode == http.StatusTooManyRequests {
		message = "AutoDL 接口限流或额度不足"
	} else if message == "" {
		message = fallback
		if statusCode > 0 {
			message += fmt.Sprintf("（HTTP %d）", statusCode)
		}
	}
	return safeMessageError{message: message}
}

func autoDLErrorText(value any) string {
	if item, ok := value.(map[string]any); ok {
		return firstPricingNonEmpty(strings.TrimSpace(fmt.Sprint(item["message"])), strings.TrimSpace(fmt.Sprint(item["msg"])), strings.TrimSpace(fmt.Sprint(item["error"])))
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func safeAutoDLError(message string) string {
	message = strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	if runes := []rune(message); len(runes) > 300 {
		message = string(runes[:300]) + "..."
	}
	lower := strings.ToLower(message)
	if strings.Contains(lower, "http://") || strings.Contains(lower, "https://") {
		return "AutoDL 上游返回失败"
	}
	return message
}
