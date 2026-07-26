package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

const (
	standardBatchImageSize         = "1024x1024"
	maxStandardBatchReferences     = 4
	maxStandardBatchReferenceBytes = 20 << 20
	maxStandardBatchResponseBytes  = 112 << 20
)

type StandardBatchProductionExecutor struct {
	Client       *http.Client
	ResultClient *http.Client
}

func standardBatchTypedError(err error, category model.BatchProductionErrorCategory, retryable bool) error {
	var typedError *batchProductionTypedError
	if errors.As(err, &typedError) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) { return err }
	if retryable { return transientBatchProductionError(category, err) }
	return permanentBatchProductionError(category, err)
}

func validateStandardBatchGenerationTask(task model.GenerationTask, item model.BatchProductionItem, job model.BatchProductionJob) error {
	if task.OrganizationID != item.OrganizationID || item.OrganizationID != job.OrganizationID ||
		task.BatchItemID != item.ID || task.BatchJobID != item.JobID || item.JobID != job.ID ||
		task.RequestID != standardBatchRequestID(item) {
		return errors.New("batch generation task does not match the current production item")
	}
	return nil
}

func (executor StandardBatchProductionExecutor) Execute(ctx context.Context, input BatchProductionExecution) (BatchProductionResult, error) {
	requestID := standardBatchRequestID(input.Item)
	existingTask, exists, err := repository.GetGenerationTaskByRequest(input.Job.OrganizationID, input.Job.CreatedBy, requestID)
	if err != nil {
		return BatchProductionResult{}, standardBatchTypedError(err, model.BatchProductionErrorInternal, true)
	}
	pendingResult := BatchProductionResult{}
	if exists {
		if err := validateStandardBatchGenerationTask(existingTask, input.Item, input.Job); err != nil {
			return BatchProductionResult{}, permanentBatchProductionError(model.BatchProductionErrorUpstreamPermanent, err)
		}
		pendingResult.GenerationTask = &existingTask
		if existingTask.Status != model.GenerationTaskStatusRunning {
			return pendingResult, permanentBatchProductionError(model.BatchProductionErrorUpstreamPermanent, errors.New("batch generation request is already settled"))
		}
	}
	prompt, err := batchProductionPrompt(input)
	if err != nil {
		return pendingResult, standardBatchTypedError(err, model.BatchProductionErrorValidationInput, false)
	}
	references, err := standardBatchReferenceURLs(input.MediaURLs)
	if err != nil {
		return pendingResult, standardBatchTypedError(err, model.BatchProductionErrorValidationInput, false)
	}
	operation, path := "generation", "/images/generations"
	if len(references) > 0 {
		operation, path = "edit", "/images/edits"
	}
	modelName := ""
	var resumeTask *model.GenerationTask
	if exists {
		modelName, resumeTask = existingTask.Model, &existingTask
	} else {
		settings, err := PublicSettings()
		if err != nil {
			return pendingResult, standardBatchTypedError(err, model.BatchProductionErrorInternal, true)
		}
		modelName = strings.TrimSpace(settings.ModelChannel.DefaultImageModel)
		if modelName == "" {
			return pendingResult, permanentBatchProductionError(model.BatchProductionErrorValidationInput, errors.New("default image model is not configured"))
		}
	}
	deliverySpec := input.Job.DeliverySpec
	if input.Selection != nil { deliverySpec = input.Selection.DeliverySpec }
	generationSize := productionDeliveryGenerationSize(deliverySpec)
	pricing := standardBatchPricingRequest(modelName, operation, deliverySpec)
	selection, err := selectStandardBatchModelChannel(pricing, resumeTask, requestID)
	if err != nil {
		if err.Error() == "original batch model channel is unavailable" || err.Error() == "batch image model has no compatible channel" {
			return pendingResult, permanentBatchProductionError(model.BatchProductionErrorValidationInput, err)
		}
		return pendingResult, standardBatchTypedError(err, model.BatchProductionErrorInternal, true)
	}
	user, ok, err := repository.GetUserByID(input.Job.CreatedBy)
	if err != nil {
		return pendingResult, standardBatchTypedError(err, model.BatchProductionErrorInternal, true)
	}
	if !ok || user.Status != model.UserStatusActive {
		return pendingResult, permanentBatchProductionError(model.BatchProductionErrorValidationInput, errors.New("batch production creator is unavailable"))
	}
	credits := existingTask.Credits
	if !exists {
		credits, err = CalculateRequestCreditsForGroup(pricing, user.Group)
		if err != nil {
			return pendingResult, permanentBatchProductionError(model.BatchProductionErrorPricingCredit, err)
		}
	}
	if credits != input.Item.EstimatedCredits {
		return pendingResult, permanentBatchProductionError(model.BatchProductionErrorPricingCredit, errors.New("batch production item pricing changed after reservation"))
	}
	resultClient, cleanup := executor.standardResultClient()
	defer cleanup()
	body, contentType, err := buildStandardBatchRequest(ctx, resultClient, selection, prompt, references, generationSize)
	if err != nil {
		return pendingResult, standardBatchTypedError(err, model.BatchProductionErrorUpstreamTransient, true)
	}
	task, err := beginOrResumeBatchGeneration(GenerationTaskInput{
		UserID: input.Job.CreatedBy, OrganizationID: input.Job.OrganizationID,
		RequestID: requestID, BatchJobID: input.Job.ID, BatchItemID: input.Item.ID, Model: modelName,
		UpstreamModel: selection.Model.UpstreamModel, ChannelName: selection.Channel.Name,
		Path: path, Modality: "image", Operation: operation, ResolutionTier: "1k", Quantity: 1, Credits: credits,
	})
	if err != nil {
		var typedError *batchProductionTypedError
		if errors.As(err, &typedError) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) { return pendingResult, err }
		if errors.Is(err, repository.ErrGenerationTaskRequestConflict) || strings.Contains(err.Error(), "batch generation request is already settled") {
			return pendingResult, permanentBatchProductionError(model.BatchProductionErrorUpstreamPermanent, err)
		}
		return pendingResult, transientBatchProductionError(model.BatchProductionErrorInternal, err)
	}
	if err := validateStandardBatchGenerationTask(task, input.Item, input.Job); err != nil {
		return BatchProductionResult{}, permanentBatchProductionError(model.BatchProductionErrorUpstreamPermanent, err)
	}
	pendingResult.GenerationTask = &task
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, BuildModelChannelURL(selection.Channel, path), bytes.NewReader(body))
	if err != nil {
		return pendingResult, standardBatchTypedError(err, model.BatchProductionErrorUpstreamPermanent, false)
	}
	request.Header.Set("Authorization", "Bearer "+selection.Channel.APIKey)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Idempotency-Key", task.RequestID)
	response, err := executor.standardUpstreamClient().Do(request)
	if err != nil {
		return pendingResult, standardBatchTypedError(err, model.BatchProductionErrorUpstreamTransient, true)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		statusErr := fmt.Errorf("model channel returned HTTP %d", response.StatusCode)
		if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 { return pendingResult, transientBatchProductionError(model.BatchProductionErrorUpstreamTransient, statusErr) }
		return pendingResult, permanentBatchProductionError(model.BatchProductionErrorUpstreamPermanent, statusErr)
	}
	result, err := parseStandardBatchResponse(ctx, resultClient, response.Body)
	result.GenerationTask = &task
	if err != nil { return result, standardBatchTypedError(err, model.BatchProductionErrorUpstreamPermanent, false) }
	return result, nil
}

func (executor StandardBatchProductionExecutor) standardUpstreamClient() *http.Client {
	client := &http.Client{}
	if executor.Client != nil {
		*client = *executor.Client
	}
	if client.Timeout <= 0 || client.Timeout > 15*time.Minute {
		client.Timeout = 15 * time.Minute
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client
}

func (executor StandardBatchProductionExecutor) standardResultClient() (*http.Client, func()) {
	if executor.ResultClient != nil {
		return executor.ResultClient, func() {}
	}
	transport := batchResultTransport()
	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Minute,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 || !validHTTPSURL(request.URL.String()) {
				return errors.New("batch image redirect is not allowed")
			}
			return nil
		},
	}
	return client, transport.CloseIdleConnections
}

func standardBatchRequestID(item model.BatchProductionItem) string {
	return fmt.Sprintf("batch:%s:%d", item.ID, item.RunNumber)
}

func standardBatchPricingRequest(modelName string, operation string, deliverySpec model.ProductionDeliverySpec) PricingRequest {
	return PricingRequest{Model: modelName, Modality: "image", Operation: operation, Unit: "image", ResolutionTier: "1k", Size: productionDeliveryGenerationSize(deliverySpec), Quantity: 1}
}

func selectStandardBatchModelChannel(request PricingRequest, task *model.GenerationTask, key string) (ModelChannelSelection, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return ModelChannelSelection{}, err
	}
	request = normalizePricingRequest(request)
	selections := modelChannelSelectionsForRequest(normalizePrivateSetting(settings.Private).Channels, request)
	if task != nil {
		for _, selection := range selections {
			if selection.Channel.Name == task.ChannelName && selection.Model.UpstreamModel == task.UpstreamModel {
				return selection, nil
			}
		}
		return ModelChannelSelection{}, errors.New("original batch model channel is unavailable")
	}
	if len(selections) == 0 {
		return ModelChannelSelection{}, errors.New("batch image model has no compatible channel")
	}
	total := 0
	for _, selection := range selections {
		total += selection.Channel.Weight
	}
	sum := sha256.Sum256([]byte(key))
	hit := int(binary.BigEndian.Uint64(sum[:8]) % uint64(total))
	for _, selection := range selections {
		hit -= selection.Channel.Weight
		if hit < 0 {
			return selection, nil
		}
	}
	return selections[0], nil
}

func beginOrResumeBatchGeneration(input GenerationTaskInput) (model.GenerationTask, error) {
	if task, ok, err := repository.GetGenerationTaskByRequest(input.OrganizationID, input.UserID, input.RequestID); err != nil {
		return model.GenerationTask{}, err
	} else if ok {
		if task.Status == model.GenerationTaskStatusRunning {
			return task, nil
		}
		return model.GenerationTask{}, errors.New("batch generation request is already settled")
	}
	task, err := BeginGenerationTask(input)
	if err == nil {
		return task, nil
	}
	if saved, ok, lookupErr := repository.GetGenerationTaskByRequest(input.OrganizationID, input.UserID, input.RequestID); lookupErr == nil && ok && saved.Status == model.GenerationTaskStatusRunning {
		return saved, nil
	}
	return task, err
}

func buildStandardBatchRequest(ctx context.Context, client *http.Client, selection ModelChannelSelection, prompt string, references []string, size string) ([]byte, string, error) {
	if strings.TrimSpace(size) == "" { size = standardBatchImageSize }
	if len(references) == 0 {
		body, err := json.Marshal(map[string]any{"model": selection.Model.UpstreamModel, "prompt": prompt, "n": 1, "size": size})
		return body, "application/json", err
	}
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	fields := map[string]string{"model": selection.Model.UpstreamModel, "prompt": prompt, "n": "1", "size": size}
	for _, name := range []string{"model", "prompt", "n", "size"} {
		if err := writer.WriteField(name, fields[name]); err != nil {
			return nil, "", err
		}
	}
	fieldName := "image[]"
	if strings.Contains(strings.ToLower(selection.Channel.BaseURL), "vividai.run") {
		fieldName = "image"
	}
	for index, referenceURL := range references {
		data, mimeType, err := downloadStandardBatchImage(ctx, client, referenceURL, maxStandardBatchReferenceBytes)
		if err != nil {
			return nil, "", err
		}
		part, err := writer.CreateFormFile(fieldName, fmt.Sprintf("reference-%d%s", index+1, assetFileExt(mimeType)))
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(data); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func standardBatchReferenceURLs(items map[string]string) ([]string, error) {
	keys := make([]string, 0, len(items))
	for key, value := range items {
		if !validHTTPSURL(value) {
			return nil, errors.New("batch reference URL is invalid")
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > maxStandardBatchReferences {
		keys = keys[:maxStandardBatchReferences]
	}
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, items[key])
	}
	return result, nil
}

func parseStandardBatchResponse(ctx context.Context, client *http.Client, reader io.Reader) (BatchProductionResult, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxStandardBatchResponseBytes+1))
	if err != nil {
		return BatchProductionResult{}, err
	}
	if len(data) > maxStandardBatchResponseBytes {
		return BatchProductionResult{}, errors.New("model channel response is too large")
	}
	var payload struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || len(payload.Data) == 0 {
		return BatchProductionResult{}, errors.New("model channel returned invalid image response")
	}
	item := payload.Data[0]
	if item.B64JSON != "" {
		if base64.StdEncoding.DecodedLen(len(item.B64JSON)) > maxUserFileSize {
			return BatchProductionResult{}, errors.New("generated image is too large")
		}
		image, err := base64.StdEncoding.DecodeString(item.B64JSON)
		if err != nil {
			return BatchProductionResult{}, errors.New("generated image base64 is invalid")
		}
		return standardBatchImageResult(image)
	}
	if !validHTTPSURL(item.URL) {
		return BatchProductionResult{}, errors.New("model channel returned invalid image URL")
	}
	image, _, err := downloadStandardBatchImage(ctx, client, item.URL, maxUserFileSize)
	if err != nil {
		return BatchProductionResult{}, err
	}
	return standardBatchImageResult(image)
}

func downloadStandardBatchImage(ctx context.Context, client *http.Client, value string, limit int64) ([]byte, string, error) {
	if !validHTTPSURL(value) {
		return nil, "", errors.New("batch image URL must use HTTPS")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, value, nil)
	if err != nil {
		return nil, "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", fmt.Errorf("batch image returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > limit {
		return nil, "", errors.New("batch image is too large")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > limit {
		return nil, "", errors.New("batch image is too large")
	}
	mimeType := strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0])
	if len(data) == 0 || assetTypeFromMime(mimeType) != "image" {
		return nil, "", errors.New("batch image content is invalid")
	}
	return data, mimeType, nil
}

func standardBatchImageResult(data []byte) (BatchProductionResult, error) {
	mimeType := strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0])
	if len(data) == 0 || len(data) > maxUserFileSize || assetTypeFromMime(mimeType) != "image" {
		return BatchProductionResult{}, errors.New("generated image content is invalid")
	}
	return BatchProductionResult{MimeType: mimeType, Size: int64(len(data)), Data: data}, nil
}

func batchProductionPrompt(input BatchProductionExecution) (string, error) {
	preset := strings.TrimSpace(input.Job.PresetPrompt)
	if preset == "" {
		return "", errors.New("batch production preset is invalid")
	}
	renderedPreset, err := renderProductionTemplatePrompt(preset, input.Product, input.SKU, input.Brand)
	if err != nil {
		return "", err
	}
	sections := []string{renderedPreset, "商品名称：" + input.Product.Name}
	appendSection := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			sections = append(sections, label+"："+value)
		}
	}
	appendSection("商品分类", input.Product.Category)
	appendSection("商品描述", input.Product.Description)
	appendSection("目标人群", input.Product.TargetAudience)
	if len(input.Product.SellingPoints) > 0 {
		appendSection("核心卖点", strings.Join(input.Product.SellingPoints, "；"))
	}
	if input.Brand != nil {
		appendSection("品牌", input.Brand.Name)
		appendSection("品牌语气", input.Brand.Tone)
		appendSection("品牌规范", input.Brand.Guidelines)
		if len(input.Brand.ProhibitedTerms) > 0 {
			appendSection("禁止在画面中出现的文字或概念", strings.Join(input.Brand.ProhibitedTerms, "、"))
		}
	}
	if input.SKU != nil {
		appendSection("SKU", input.SKU.Name)
		keys := make([]string, 0, len(input.SKU.Attributes))
		for key := range input.SKU.Attributes {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		attributes := make([]string, 0, len(keys))
		for _, key := range keys {
			attributes = append(attributes, key+"="+input.SKU.Attributes[key])
		}
		appendSection("SKU 属性", strings.Join(attributes, "；"))
	}
	if input.Job.DeliverySpec.Width > 0 && input.Job.DeliverySpec.Height > 0 {
		appendSection("目标渠道", input.Job.DeliverySpec.Platform+" "+input.Job.DeliverySpec.Name)
		appendSection("交付画幅", fmt.Sprintf("%d×%d，保持主体完整并适配该比例", input.Job.DeliverySpec.Width, input.Job.DeliverySpec.Height))
	}
	return truncateBatchPrompt(strings.Join(sections, "\n"), 12000), nil
}

func truncateBatchPrompt(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
