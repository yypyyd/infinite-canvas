package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

var ErrProtocolSubmissionUncertain = errors.New("上游提交结果待确认，系统不会重复创建任务")

func IsManagedMediaChannel(channel model.ModelChannel) bool {
	return IsAutoDLComfyUIChannel(channel) || IsCustomHTTPChannel(channel)
}

func customProtocolOperation(selection ModelChannelSelection, modality, operation string) (model.ProtocolTemplate, model.ProtocolOperation, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return model.ProtocolTemplate{}, model.ProtocolOperation{}, err
	}
	template, err := findProtocolTemplate(normalizeProtocolTemplates(settings.Private.ProtocolTemplates), selection.Channel.TemplateID)
	if err != nil {
		return template, model.ProtocolOperation{}, err
	}
	definition, ok := template.Operations[modality+"."+operation]
	if !ok {
		return template, definition, errors.New("协议模板未配置此操作")
	}
	return template, definition, ValidateProtocolTemplate(template)
}

func ValidateManagedMediaRequest(selection ModelChannelSelection, request PricingRequest) error {
	if !IsCustomHTTPChannel(selection.Channel) {
		return nil
	}
	_, operation, err := customProtocolOperation(selection, request.Modality, request.Operation)
	if err != nil {
		return err
	}
	if request.Modality == "image" && request.Quantity > operation.MaxImages {
		return fmt.Errorf("当前协议每次最多生成 %d 张图片，请调整生成数量", operation.MaxImages)
	}
	encoded, _ := json.Marshal(operation.Create.Body)
	if request.GenerateAudio && !strings.Contains(string(encoded), "${generate_audio}") {
		return errors.New("当前协议没有配置生成音轨参数")
	}
	return nil
}

// SubmitManagedMediaTask is shared by ordinary requests and the batch worker.
func SubmitManagedMediaTask(ctx context.Context, user model.AuthUser, task *model.GenerationTask, selection ModelChannelSelection, input MediaGenerationInput) error {
	if task.ProtocolSnapshot != nil {
		return ErrProtocolSubmissionUncertain
	}
	if IsAutoDLComfyUIChannel(selection.Channel) {
		id, body, err := SubmitAutoDLComfyUITask(ctx, user, *task, selection, input)
		if err != nil {
			return err
		}
		return UpdateGenerationTaskRecovery(task, id, body, nil)
	}
	template, operation, err := customProtocolOperation(selection, task.Modality, task.Operation)
	if err != nil {
		return err
	}
	startedAt, err := time.Parse(time.RFC3339, task.CreatedAt)
	if err != nil {
		return err
	}
	deadline := startedAt.Add(time.Duration(operation.TimeoutSeconds) * time.Second)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if task.BatchJobID == "" {
		claimed, ok, err := repository.ClaimGenerationTaskReconcile(task.ID, newID("submit"), now(), time.Now().UTC().Add(16*time.Minute).Format(timestampLayout))
		if err != nil {
			return err
		}
		if !ok {
			return ErrProtocolSubmissionUncertain
		}
		*task = claimed
		defer func() {
			if err := repository.ReleaseGenerationTaskReconcile(task.ID, task.ReconcileLeaseToken, time.Now().UTC().Add(time.Duration(operation.PollSeconds)*time.Second).Format(timestampLayout)); err != nil {
				logWorkerError("generation_task", "protocol_submission_release_failed", err, "task_id", task.ID)
			}
			task.ReconcileLeaseToken = ""
		}()
		if task.ProtocolSnapshot != nil {
			return ErrProtocolSubmissionUncertain
		}
	}
	files, urls, err := prepareProtocolReferences(ctx, user, *task, input, template.Upload)
	if err != nil {
		return err
	}
	keys := []string{}
	if template.Upload != nil && template.Upload.Mode != "url" {
		keys, err = uploadProtocolFiles(ctx, selection.Channel, template, files)
		if err != nil {
			return err
		}
	}
	if len(files) > 0 && template.Upload == nil && operation.Create.Encoding != "multipart" {
		return errors.New("此协议未配置参考素材上传方式")
	}
	variables := protocolGenerationValues(*task, input)
	variables["reference_keys"], variables["reference_urls"] = keys, urls
	for _, kind := range []string{"image", "video", "audio"} {
		kindURLs := []string{}
		for _, file := range files {
			if file.Kind == kind && file.URL != "" {
				kindURLs = append(kindURLs, file.URL)
			}
		}
		variables["reference_"+kind+"_urls"] = kindURLs
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot := &model.GenerationProtocolSnapshot{ChannelID: selection.Channel.ID, BaseURL: selection.Channel.BaseURL, Template: template, Operation: task.Modality + "." + task.Operation, DeadlineAt: deadline.Format(timestampLayout)}
	if err := repository.SaveGenerationProtocol(*task, snapshot, now()); err != nil {
		return err
	}
	task.ProtocolSnapshot = snapshot
	attachments := files
	if template.Upload != nil {
		attachments = nil
	}
	response, err := protocolRequest(ctx, selection.Channel, template, operation.Create, variables, operation.ValueMaps, attachments)
	if err != nil {
		if errors.Is(err, errProtocolConnection) {
			return ErrProtocolSubmissionUncertain
		}
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_ = response.Body.Close()
		// A server error can occur after accepting a task; never blindly replay it.
		if response.StatusCode >= 500 || response.StatusCode == http.StatusRequestTimeout {
			return ErrProtocolSubmissionUncertain
		}
		return fmt.Errorf("上游拒绝创建任务（HTTP %d）", response.StatusCode)
	}
	defer response.Body.Close()
	err = consumeProtocolTask(ctx, task, selection.Channel, response, operation.Create.Response)
	if err != nil && task.UpstreamTaskID == "" && task.Status != model.GenerationTaskStatusFailed {
		return ErrProtocolSubmissionUncertain
	}
	return err
}

func protocolGenerationValues(task model.GenerationTask, input MediaGenerationInput) map[string]any {
	values := map[string]any{
		"model": task.UpstreamModel, "prompt": input.Prompt, "negative_prompt": input.NegativePrompt,
		"size": input.Size, "ratio": normalizeVideoAspectRatio(firstPricingNonEmpty(input.Ratio, input.Size)),
		"resolution": task.ResolutionTier, "seconds": input.Seconds, "count": max(1, input.Count),
		"seed": input.Seed, "generate_audio": input.GenerateAudio, "request_id": task.RequestID,
	}
	if width, height, ok := parsePricingDimensions(input.Size); ok {
		values["width"], values["height"] = width, height
	}
	return values
}

func prepareProtocolReferences(ctx context.Context, user model.AuthUser, task model.GenerationTask, input MediaGenerationInput, upload *model.ProtocolUpload) ([]protocolFile, []string, error) {
	files := []protocolFile{}
	urls := []string{}
	for _, group := range []struct {
		kind string
		keys []string
		raw  []MediaReferenceFile
	}{
		{"image", input.ReferenceImageStorageKeys, input.ReferenceImages},
		{"video", input.ReferenceVideoStorageKeys, input.ReferenceVideos},
		{"audio", input.ReferenceAudioStorageKeys, input.ReferenceAudios},
	} {
		for _, key := range group.keys {
			var file protocolFile
			var err error
			if upload != nil && upload.Mode == "url" {
				media, resolveErr := ResolveUserWorkspaceMedia(user, key)
				if resolveErr != nil || assetTypeFromMime(media.MimeType) != group.kind || media.Size > protocolReferenceLimit(group.kind) {
					return nil, nil, errors.New("参考素材不存在或类型不符")
				}
				file = protocolFile{Kind: group.kind, Mime: media.MimeType, URL: media.URL, Size: media.Size}
			} else {
				file, err = protocolStorageFile(ctx, user, key, group.kind)
				if err != nil {
					return nil, nil, err
				}
			}
			files = append(files, file)
		}
		for _, raw := range group.raw {
			mimeType, err := mimeParseReference(raw.MimeType, raw.Data, group.kind)
			if err != nil {
				return nil, nil, err
			}
			file := protocolFile{Kind: group.kind, Mime: mimeType, Data: raw.Data}
			if upload != nil && upload.Mode == "url" {
				stored, err := ArchiveGeneratedFile(ctx, user, task.ID+"-reference", group.kind, len(files)+1, mimeType, int64(len(raw.Data)), bytes.NewReader(raw.Data))
				if err != nil {
					return nil, nil, err
				}
				media, err := ResolveUserWorkspaceMedia(user, stored.StorageKey)
				if err != nil {
					return nil, nil, err
				}
				file.URL = media.URL
			}
			files = append(files, file)
		}
	}
	total := int64(0)
	for _, file := range files {
		total += max(file.Size, int64(len(file.Data)))
		if file.URL != "" {
			urls = append(urls, file.URL)
		}
	}
	if total > 320<<20 {
		return nil, nil, errors.New("参考素材合计不能超过 320MB")
	}
	return files, urls, nil
}

func mimeParseReference(mimeType string, data []byte, kind string) (string, error) {
	mimeType = strings.TrimSpace(strings.Split(mimeType, ";")[0])
	if kind == "image" {
		mimeType = http.DetectContentType(data)
	}
	if len(data) == 0 || int64(len(data)) > protocolReferenceLimit(kind) || assetTypeFromMime(mimeType) != kind {
		return "", errors.New("参考素材类型或大小不符合要求")
	}
	return mimeType, nil
}

func protocolTaskChannel(task model.GenerationTask) (model.ModelChannel, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return model.ModelChannel{}, err
	}
	for _, channel := range settings.Private.Channels {
		if channel.ID == task.ProtocolSnapshot.ChannelID && channel.ID != "" && channel.APIKey != "" {
			channel.BaseURL = task.ProtocolSnapshot.BaseURL
			return channel, nil
		}
	}
	return model.ModelChannel{}, errors.New("原渠道密钥不可用，任务暂时无法恢复")
}

func ReconcileManagedMediaTask(ctx context.Context, task *model.GenerationTask, channel model.ModelChannel) (bool, error) {
	if task.ProtocolSnapshot == nil {
		return ReconcileAutoDLGenerationTask(ctx, task, channel)
	}
	if len(task.StorageKeys) > 0 {
		if task.BatchJobID == "" {
			if err := FinishGenerationTask(*task, model.GenerationTaskStatusSuccess, ""); err != nil {
				return false, err
			}
			task.Status = model.GenerationTaskStatusSuccess
		}
		return true, nil
	}
	ctx, cancel := context.WithDeadline(ctx, ManagedMediaDeadline(*task))
	defer cancel()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if task.UpstreamTaskID == "" {
		return false, ErrProtocolSubmissionUncertain
	}
	channel, err := protocolTaskChannel(*task)
	if err != nil {
		return false, err
	}
	snapshot := task.ProtocolSnapshot
	operation := snapshot.Template.Operations[snapshot.Operation]
	if operation.Resume == nil {
		return false, errors.New("原协议没有配置任务恢复接口")
	}
	variables := map[string]any{"task_id": task.UpstreamTaskID, "model": task.UpstreamModel, "request_id": task.RequestID}
	response, err := protocolRequest(ctx, channel, snapshot.Template, *operation.Resume, variables, operation.ValueMaps, nil)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false, fmt.Errorf("上游任务恢复返回 HTTP %d", response.StatusCode)
	}
	err = consumeProtocolTask(ctx, task, channel, response, operation.Resume.Response)
	return err == nil && (task.Status == model.GenerationTaskStatusFailed || len(task.StorageKeys) > 0), err
}

func consumeProtocolTask(ctx context.Context, task *model.GenerationTask, channel model.ModelChannel, response *http.Response, mapping model.ProtocolResponse) error {
	return decodeProtocolResponse(response.Body, mapping, func(result protocolTaskResult) error {
		if result.TaskID != "" && result.TaskID != task.UpstreamTaskID {
			if task.UpstreamTaskID != "" {
				return errors.New("上游返回了不匹配的任务 ID")
			}
			if err := UpdateGenerationTaskRecovery(task, result.TaskID, autoDLStatusBody(task.Modality, result.TaskID, "running"), nil); err != nil {
				return err
			}
		}
		switch result.Status {
		case "failed":
			message := strings.ReplaceAll(result.Error, channel.APIKey, "***")
			if strings.Contains(strings.ToLower(message), "http") {
				message = "上游生成失败"
			}
			message = safeAutoDLError(message)
			if task.BatchJobID == "" {
				if err := FinishGenerationTask(*task, model.GenerationTaskStatusFailed, message); err != nil {
					return err
				}
			}
			task.Status, task.ErrorMessage = model.GenerationTaskStatusFailed, message
			return nil
		case "success":
			return archiveProtocolResult(ctx, task, result)
		}
		return nil
	})
}

func archiveProtocolResult(ctx context.Context, task *model.GenerationTask, result protocolTaskResult) error {
	expected := 1
	if task.Modality == "image" {
		expected = max(1, task.Quantity)
	}
	if len(result.URLs)+len(result.Base64) != expected {
		return errors.New("上游返回的作品数量与请求不符，将继续尝试恢复")
	}
	user, exists, err := repository.GetUserByID(task.UserID)
	if err != nil || !exists {
		return errors.New("生成任务账号不存在")
	}
	auth := model.PublicUser(user)
	auth.OrganizationID = task.OrganizationID
	files := []model.UserFile{}
	for _, rawURL := range result.URLs {
		file, err := archiveAutoDLOutput(ctx, auth, *task, len(files)+1, autoDLTaskOutput{URL: rawURL, Type: task.Modality})
		if err != nil {
			return errors.New("生成结果暂未归档，将继续尝试恢复")
		}
		files = append(files, file)
	}
	for _, encoded := range result.Base64 {
		if base64.StdEncoding.DecodedLen(len(encoded)) > maxUserFileSize {
			return errors.New("生成结果超过文件大小限制")
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || assetTypeFromMime(http.DetectContentType(data)) != task.Modality {
			return errors.New("生成结果内容无效")
		}
		file, err := ArchiveGeneratedFile(ctx, auth, task.ID, task.Modality, len(files)+1, http.DetectContentType(data), int64(len(data)), bytes.NewReader(data))
		if err != nil {
			return err
		}
		files = append(files, file)
	}
	keys := make([]string, 0, len(files))
	for _, file := range files {
		keys = append(keys, file.StorageKey)
	}
	if len(keys) == 0 {
		return errors.New("上游未返回可交付的结果")
	}
	if task.Modality == "video" && task.UpstreamTaskID == "" {
		task.UpstreamTaskID = task.ID
	}
	if err := UpdateGenerationTaskRecovery(task, task.UpstreamTaskID, autoDLArchivedBody(*task, files), keys); err != nil {
		return err
	}
	if task.BatchJobID == "" {
		if err := FinishGenerationTask(*task, model.GenerationTaskStatusSuccess, ""); err != nil {
			return err
		}
		task.Status = model.GenerationTaskStatusSuccess
	}
	return nil
}

func ManagedMediaDeadline(task model.GenerationTask) time.Time {
	if task.ProtocolSnapshot != nil {
		deadline, _ := time.Parse(time.RFC3339, task.ProtocolSnapshot.DeadlineAt)
		return deadline
	}
	created, _ := time.Parse(time.RFC3339, task.CreatedAt)
	return created.Add(generationTaskTimeout)
}

func ManagedMediaResult(ctx context.Context, user model.AuthUser, task model.GenerationTask, format string) ([]byte, error) {
	if task.Modality != "image" {
		return []byte(task.ResultJSON), nil
	}
	items := []map[string]any{}
	for _, key := range task.StorageKeys {
		file, err := readProtocolStoredFile(ctx, user, key, "image", maxUserFileSize)
		if err != nil {
			return nil, err
		}
		item := map[string]any{"storage_key": key, "mime_type": file.Mime, "bytes": len(file.Data)}
		if format == "url" {
			if rawURL, ok := UserWorkspaceFileURL(user, key, ""); ok {
				item["url"] = rawURL
			}
		}
		if item["url"] == nil {
			item["b64_json"] = base64.StdEncoding.EncodeToString(file.Data)
		}
		items = append(items, item)
	}
	return json.Marshal(map[string]any{"created": time.Now().Unix(), "data": items})
}
