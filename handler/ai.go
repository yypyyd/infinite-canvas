package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/basketikun/infinite-canvas/service"
)

type cachedVideoGeneration struct {
	Body      []byte
	MimeType  string
	CreatedAt time.Time
}

type aiRetryPolicy struct {
	MaxRetries int
	Delay      time.Duration
}

var videoGenerationCache = struct {
	sync.Mutex
	items map[string]cachedVideoGeneration
}{items: map[string]cachedVideoGeneration{}}

func AIImagesGenerations(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/images/generations")
}

func AIImagesEdits(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/images/edits")
}

func AIChatCompletions(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/chat/completions")
}

func AIAudioSpeech(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/audio/speech")
}

func AIVideos(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/videos")
}

func AIVideo(w http.ResponseWriter, r *http.Request, id string) {
	proxyAIGetRequest(w, r, "/videos/"+id)
}

func AIVideoContent(w http.ResponseWriter, r *http.Request, id string) {
	proxyAIGetRequest(w, r, "/videos/"+id+"/content")
}

func proxyAIGetRequest(w http.ResponseWriter, r *http.Request, path string) {
	originalPath := path
	modelName := r.URL.Query().Get("model")
	if strings.TrimSpace(modelName) == "" {
		modelName = "grok-imagine-video"
	}
	if isCachedVideoPath(originalPath) {
		if serveCachedVideoGeneration(w, originalPath) {
			return
		}
	}
	selection, err := service.SelectModelChannel(service.PricingRequest{Model: modelName})
	if err != nil {
		log.Printf("AI proxy select channel failed: model=%s err=%v", modelName, err)
		FailError(w, err)
		return
	}
	channel := selection.Channel
	upstreamModel := selection.Model.UpstreamModel
	path = resolveAIProxyPath(channel.BaseURL, upstreamModel, path)
	request, err := http.NewRequest(http.MethodGet, service.BuildModelChannelURL(channel, path), nil)
	if err != nil {
		Fail(w, "AI 接口请求失败")
		return
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	if isVideoGenerationsAPI(channel.BaseURL, upstreamModel) && strings.HasSuffix(originalPath, "/content") {
		copyVideoGenerationsContent(w, request)
		return
	}
	copyAIResponse(w, request, nil, aiResponseAdapter(channel.BaseURL, upstreamModel, originalPath), nil)
}

func proxyAIRequest(w http.ResponseWriter, r *http.Request, path string) {
	originalPath := path
	body, contentType, requestMeta, err := readAIRequest(r)
	if err != nil {
		log.Printf("AI proxy request read failed: %v", err)
		Fail(w, "AI 接口请求失败")
		return
	}
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	pricingRequest := pricingRequestForAIPath(path, requestMeta)
	selection, err := service.SelectModelChannel(pricingRequest)
	if err != nil {
		log.Printf("AI proxy select channel failed: model=%s operation=%s resolution=%s err=%v", requestMeta.ModelName, pricingRequest.Operation, pricingRequest.ResolutionTier, err)
		FailError(w, err)
		return
	}
	credits, err := service.CalculateRequestCreditsForGroup(pricingRequest, user.Group)
	if err != nil {
		log.Printf("AI proxy calculate credits failed: model=%s path=%s err=%v", requestMeta.ModelName, path, err)
		FailError(w, err)
		return
	}
	channel := selection.Channel
	upstreamModel := selection.Model.UpstreamModel
	body, contentType, err = replaceAIRequestModel(body, contentType, upstreamModel)
	if err != nil {
		log.Printf("AI proxy replace upstream model failed: model=%s upstream=%s err=%v", requestMeta.ModelName, upstreamModel, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	path = resolveAIProxyPath(channel.BaseURL, upstreamModel, path)
	body, contentType = adaptAIRequestBody(channel.BaseURL, upstreamModel, originalPath, body, contentType)
	request, err := http.NewRequest(http.MethodPost, service.BuildModelChannelURL(channel, path), bytes.NewReader(body))
	if err != nil {
		log.Printf("AI proxy build request failed: url=%s err=%v", service.BuildModelChannelURL(channel, path), err)
		Fail(w, "AI 接口请求失败")
		return
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if err := service.ConsumeUserCredits(user.ID, requestMeta.ModelName, credits, path); err != nil {
		FailError(w, err)
		return
	}
	task, taskErr := service.BeginGenerationTask(service.GenerationTaskInput{
		UserID:         user.ID,
		Model:          requestMeta.ModelName,
		UpstreamModel:  upstreamModel,
		ChannelName:    channel.Name,
		Path:           path,
		Modality:       pricingRequest.Modality,
		Operation:      pricingRequest.Operation,
		ResolutionTier: pricingRequest.ResolutionTier,
		Quantity:       pricingRequest.Quantity,
		Credits:        credits,
	})
	if taskErr != nil {
		log.Printf("AI proxy create generation task failed: user=%s model=%s err=%v", user.ID, requestMeta.ModelName, taskErr)
	}
	failed := false
	onFailure := func(message string) {
		failed = true
		if err := service.RefundUserCredits(user.ID, requestMeta.ModelName, credits, path); err != nil {
			log.Printf("AI proxy refund credits failed: user=%s model=%s credits=%d err=%v", user.ID, requestMeta.ModelName, credits, err)
		}
		if taskErr == nil {
			service.FinishGenerationTask(task, "failed", message)
		}
	}
	if isChatGPT2APIImageTaskChannel(channel.BaseURL, originalPath) {
		copyChatGPT2APIImageTaskResponse(w, channel.BaseURL, channel.APIKey, body, onFailure)
		if !failed && taskErr == nil {
			service.FinishGenerationTask(task, "success", "")
		}
		return
	}
	copyAIResponse(w, request, onFailure, aiResponseAdapter(channel.BaseURL, upstreamModel, originalPath), aiRetryPolicyForRequest(channel.BaseURL, upstreamModel, originalPath))
	if !failed && taskErr == nil {
		service.FinishGenerationTask(task, "success", "")
	}
}

type chatGPT2APIImageTask struct {
	ID     string           `json:"id"`
	Status string           `json:"status"`
	Data   []map[string]any `json:"data"`
	Error  string           `json:"error"`
}

func isChatGPT2APIImageTaskChannel(baseURL string, path string) bool {
	if path != "/images/generations" {
		return false
	}
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	return err == nil && strings.EqualFold(parsed.Hostname(), "chat2.pluskk.xyz")
}

func copyChatGPT2APIImageTaskResponse(w http.ResponseWriter, baseURL string, apiKey string, body []byte, onFailure func(string)) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		message := "AI 接口请求失败：请求参数无效"
		onFailure(message)
		Fail(w, message)
		return
	}
	taskPayload := map[string]any{
		"client_task_id": fmt.Sprintf("huantu-%d", time.Now().UnixNano()),
		"prompt":         payload["prompt"],
		"model":          payload["model"],
	}
	for _, key := range []string{"size", "quality"} {
		if value, ok := payload[key]; ok {
			taskPayload[key] = value
		}
	}
	taskBody, _ := json.Marshal(taskPayload)
	upstreamBaseURL := strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(baseURL), "/"), "/v1")
	task, responseBody, statusCode, err := requestChatGPT2APIImageTask(http.MethodPost, upstreamBaseURL+"/api/image-tasks/generations", apiKey, taskBody)
	if err != nil || statusCode >= http.StatusBadRequest || task.ID == "" {
		log.Printf("AI async image task submit failed: status=%d err=%v", statusCode, err)
		if statusCode >= http.StatusBadRequest {
			message := aiUpstreamStatusMessage(statusCode, responseBody)
			onFailure(message)
			Fail(w, message)
		} else {
			message := "AI 接口请求失败：上游任务提交失败"
			onFailure(message)
			Fail(w, message)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	deadline := time.Now().Add(6 * time.Minute)
	for {
		switch strings.ToLower(task.Status) {
		case "success":
			data := normalizeChatGPT2APIImageTaskData(upstreamBaseURL, apiKey, task.Data)
			if len(data) == 0 {
				message := "AI 接口请求失败：上游任务没有返回图片"
				onFailure(message)
				Fail(w, message)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"created": time.Now().Unix(), "data": data})
			return
		case "error", "failed":
			message := "AI 接口请求失败：" + safeUpstreamText(task.Error)
			onFailure(message)
			Fail(w, message)
			return
		}
		if time.Now().After(deadline) {
			message := "AI 接口请求失败：上游生图任务等待超时"
			onFailure(message)
			Fail(w, message)
			return
		}
		writeAIKeepAlive(w)
		time.Sleep(10 * time.Second)
		pollURL := upstreamBaseURL + "/api/image-tasks?ids=" + url.QueryEscape(task.ID)
		nextTask, _, pollStatus, pollErr := requestChatGPT2APIImageTask(http.MethodGet, pollURL, apiKey, nil)
		if pollErr != nil || pollStatus >= http.StatusInternalServerError {
			log.Printf("AI async image task poll failed: id=%s status=%d err=%v", task.ID, pollStatus, pollErr)
			continue
		}
		if pollStatus >= http.StatusBadRequest {
			message := "AI 接口请求失败：上游任务查询失败"
			onFailure(message)
			Fail(w, message)
			return
		}
		task = nextTask
	}
}

func requestChatGPT2APIImageTask(method string, requestURL string, apiKey string, body []byte) (chatGPT2APIImageTask, []byte, int, error) {
	request, err := http.NewRequest(method, requestURL, bytes.NewReader(body))
	if err != nil {
		return chatGPT2APIImageTask{}, nil, 0, err
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, responseBody, err := doAIRequestWithRetry(request, &aiRetryPolicy{MaxRetries: 2, Delay: 2 * time.Second})
	if err != nil {
		return chatGPT2APIImageTask{}, nil, 0, err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return chatGPT2APIImageTask{}, responseBody, response.StatusCode, nil
	}
	var task chatGPT2APIImageTask
	if method == http.MethodGet {
		var list struct {
			Items []chatGPT2APIImageTask `json:"items"`
		}
		if err := json.Unmarshal(responseBody, &list); err != nil {
			return task, responseBody, response.StatusCode, err
		}
		if len(list.Items) == 0 {
			return task, responseBody, response.StatusCode, fmt.Errorf("image task not found")
		}
		task = list.Items[0]
	} else if err := json.Unmarshal(responseBody, &task); err != nil {
		return task, responseBody, response.StatusCode, err
	}
	return task, responseBody, response.StatusCode, nil
}

func normalizeChatGPT2APIImageTaskData(baseURL string, apiKey string, data []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(data))
	for _, item := range data {
		urlValue := firstStringValue(item, "url")
		encoded := firstStringValue(item, "b64_json")
		if encoded == "" && urlValue != "" {
			encoded = downloadChatGPT2APIImage(baseURL, apiKey, urlValue)
		}
		if encoded == "" && urlValue == "" {
			continue
		}
		next := map[string]any{}
		if encoded != "" {
			next["b64_json"] = encoded
		}
		if urlValue != "" {
			next["url"] = urlValue
		}
		if revisedPrompt := firstStringValue(item, "revised_prompt"); revisedPrompt != "" {
			next["revised_prompt"] = revisedPrompt
		}
		result = append(result, next)
	}
	return result
}

func downloadChatGPT2APIImage(baseURL string, apiKey string, imageURL string) string {
	parsed, err := url.Parse(imageURL)
	if err != nil {
		return ""
	}
	if !parsed.IsAbs() {
		base, baseErr := url.Parse(baseURL + "/")
		if baseErr != nil {
			return ""
		}
		parsed = base.ResolveReference(parsed)
	}
	request, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return ""
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return ""
	}
	imageBody, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil || len(imageBody) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(imageBody)
}

func writeAIKeepAlive(w http.ResponseWriter) {
	_, _ = io.WriteString(w, strings.Repeat(" ", 2048)+"\n")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func copyAIResponse(w http.ResponseWriter, request *http.Request, onFailure func(string), adapter func([]byte) ([]byte, string, bool), retryPolicy *aiRetryPolicy) {
	response, body, err := doAIRequestWithRetry(request, retryPolicy)
	if err != nil {
		log.Printf("AI proxy request failed: url=%s err=%v", request.URL.String(), err)
		message := "AI 接口请求失败：上游连接中断，请稍后重试"
		if onFailure != nil {
			onFailure(message)
		}
		Fail(w, message)
		return
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		log.Printf("AI upstream error: url=%s status=%d", request.URL.String(), response.StatusCode)
		message := aiUpstreamStatusMessage(response.StatusCode, body)
		if onFailure != nil {
			onFailure(message)
		}
		Fail(w, message)
		return
	}

	for key, values := range response.Header {
		if strings.EqualFold(key, "Content-Length") || adapter != nil && strings.EqualFold(key, "Content-Type") {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	if adapter != nil {
		if transformed, contentType, ok := adapter(body); ok {
			if contentType != "" {
				w.Header().Set("Content-Type", contentType)
			}
			w.WriteHeader(response.StatusCode)
			_, _ = w.Write(transformed)
			return
		}
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(body)
		return
	}
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(body)
}

func doAIRequestWithRetry(request *http.Request, retryPolicy *aiRetryPolicy) (*http.Response, []byte, error) {
	attempts := 1
	delay := time.Duration(0)
	if retryPolicy != nil && retryPolicy.MaxRetries > 0 {
		attempts += retryPolicy.MaxRetries
		delay = retryPolicy.Delay
	}
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 && delay > 0 {
			time.Sleep(delay)
		}
		if request.GetBody != nil {
			body, err := request.GetBody()
			if err != nil {
				return nil, nil, err
			}
			request.Body = body
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			if attempt+1 < attempts {
				log.Printf("AI proxy retry request error: url=%s attempt=%d err=%v", request.URL.String(), attempt+1, err)
				continue
			}
			return nil, nil, err
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		response.Body = io.NopCloser(bytes.NewReader(body))
		if readErr != nil {
			if attempt+1 < attempts {
				log.Printf("AI proxy retry read error: url=%s attempt=%d err=%v", request.URL.String(), attempt+1, readErr)
				continue
			}
			return nil, nil, readErr
		}
		if attempt+1 < attempts && isRetryableAIUpstreamError(response.StatusCode, body) {
			log.Printf("AI proxy retry upstream temporary error: url=%s status=%d attempt=%d", request.URL.String(), response.StatusCode, attempt+1)
			continue
		}
		return response, body, nil
	}
	return nil, nil, fmt.Errorf("AI request retry exhausted")
}

func aiRetryPolicyForRequest(baseURL string, modelName string, path string) *aiRetryPolicy {
	if isVideoGenerationsAPI(baseURL, modelName) && path == "/videos" {
		return &aiRetryPolicy{MaxRetries: 2, Delay: 4 * time.Second}
	}
	return nil
}

func isRetryableAIUpstreamError(statusCode int, body []byte) bool {
	if statusCode == http.StatusRequestTimeout || statusCode == http.StatusBadGateway || statusCode == http.StatusServiceUnavailable || statusCode == http.StatusGatewayTimeout {
		return true
	}
	text := strings.ToLower(string(body))
	return strings.Contains(text, "timeout_error") ||
		strings.Contains(text, "system under load") ||
		strings.Contains(text, "temporary unavailable") ||
		strings.Contains(text, "temporarily unavailable")
}

func copyVideoGenerationsContent(w http.ResponseWriter, request *http.Request) {
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		log.Printf("AI proxy request failed: url=%s err=%v", request.URL.String(), err)
		Fail(w, "AI 接口请求失败：上游连接中断，请稍后重试")
		return
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
	if response.StatusCode >= http.StatusBadRequest {
		log.Printf("AI upstream error: url=%s status=%d", request.URL.String(), response.StatusCode)
		Fail(w, aiUpstreamStatusMessage(response.StatusCode, body))
		return
	}
	videoURL := videoGenerationsContentURL(body)
	if videoURL == "" {
		Fail(w, "Video is not ready")
		return
	}
	parsed, err := url.Parse(videoURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		Fail(w, "AI video URL is invalid")
		return
	}
	videoRequest, err := http.NewRequest(http.MethodGet, videoURL, nil)
	if err != nil {
		Fail(w, "AI video URL is invalid")
		return
	}
	videoResponse, err := http.DefaultClient.Do(videoRequest)
	if err != nil {
		log.Printf("AI proxy video download failed: url=%s err=%v", videoURL, err)
		Fail(w, "Video download failed")
		return
	}
	defer videoResponse.Body.Close()
	if videoResponse.StatusCode >= http.StatusBadRequest {
		log.Printf("AI proxy video download failed: url=%s status=%d", videoURL, videoResponse.StatusCode)
		Fail(w, "Video download failed")
		return
	}
	for key, values := range videoResponse.Header {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(videoResponse.StatusCode)
	_, _ = io.Copy(w, videoResponse.Body)
}

func isCachedVideoPath(path string) bool {
	return strings.HasPrefix(path, "/videos/local-")
}

func serveCachedVideoGeneration(w http.ResponseWriter, path string) bool {
	id := strings.TrimPrefix(path, "/videos/")
	isContent := strings.HasSuffix(id, "/content")
	id = strings.TrimSuffix(id, "/content")
	videoGenerationCache.Lock()
	item, ok := videoGenerationCache.items[id]
	videoGenerationCache.Unlock()
	if !ok {
		return false
	}
	if isContent {
		if item.MimeType != "" {
			w.Header().Set("Content-Type", item.MimeType)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(item.Body)
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "status": "completed"})
	return true
}

func cacheVideoGeneration(body []byte, mimeType string) string {
	id := fmt.Sprintf("local-%d", time.Now().UnixNano())
	videoGenerationCache.Lock()
	defer videoGenerationCache.Unlock()
	videoGenerationCache.items[id] = cachedVideoGeneration{Body: body, MimeType: mimeType, CreatedAt: time.Now()}
	cutoff := time.Now().Add(-30 * time.Minute)
	for key, item := range videoGenerationCache.items {
		if item.CreatedAt.Before(cutoff) {
			delete(videoGenerationCache.items, key)
		}
	}
	return id
}

type aiRequestMeta struct {
	ModelName      string
	Count          int
	Size           string
	Quality        string
	Resolution     string
	ResolutionTier string
	Duration       int
}

func readAIRequest(r *http.Request) ([]byte, string, aiRequestMeta, error) {
	contentType := r.Header.Get("Content-Type")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, "", aiRequestMeta{}, err
	}
	requestMeta := aiRequestMeta{Count: 1, Duration: 1}
	if strings.HasPrefix(contentType, "multipart/form-data") {
		requestMeta = readMultipartAIRequest(body, contentType)
	} else {
		requestMeta = readJSONAIRequest(body)
	}
	requestMeta.ModelName = strings.TrimSpace(requestMeta.ModelName)
	if requestMeta.Count < 1 {
		requestMeta.Count = 1
	}
	if requestMeta.Duration < 1 {
		requestMeta.Duration = 1
	}
	if requestMeta.ModelName == "" {
		return nil, "", aiRequestMeta{}, errMissingModel
	}
	return body, contentType, requestMeta, nil
}

func replaceAIRequestModel(body []byte, contentType string, modelName string) ([]byte, string, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil, "", errors.New("缺少上游模型名称")
	}
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return replaceMultipartAIRequestModel(body, contentType, modelName)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, contentType, nil
	}
	payload["model"] = modelName
	encoded, err := json.Marshal(payload)
	return encoded, contentType, err
}

func replaceMultipartAIRequestModel(body []byte, contentType string, modelName string) ([]byte, string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil || params["boundary"] == "" {
		return nil, "", errors.New("multipart 请求格式无效")
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	foundModel := false
	for {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return nil, "", nextErr
		}
		target, createErr := writer.CreatePart(part.Header)
		if createErr != nil {
			_ = part.Close()
			return nil, "", createErr
		}
		if part.FormName() == "model" {
			foundModel = true
			_, err = io.WriteString(target, modelName)
		} else {
			_, err = io.Copy(target, part)
		}
		_ = part.Close()
		if err != nil {
			return nil, "", err
		}
	}
	if !foundModel {
		field, createErr := writer.CreateFormField("model")
		if createErr != nil {
			return nil, "", createErr
		}
		if _, err = io.WriteString(field, modelName); err != nil {
			return nil, "", err
		}
	}
	if err = writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func readMultipartAIRequest(body []byte, contentType string) aiRequestMeta {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return aiRequestMeta{Count: 1, Duration: 1}
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	form, err := reader.ReadForm(32 << 20)
	if err != nil {
		return aiRequestMeta{Count: 1, Duration: 1}
	}
	defer form.RemoveAll()
	return aiRequestMeta{
		ModelName:      firstFormValue(form, "model"),
		Count:          readIntValue(firstFormValue(form, "n"), 1),
		Size:           firstFormValue(form, "size", "ratio"),
		Quality:        firstFormValue(form, "quality"),
		Resolution:     firstFormValue(form, "resolution_name", "resolution", "vquality"),
		ResolutionTier: firstFormValue(form, "resolutionTier"),
		Duration:       readIntValue(firstFormValue(form, "seconds", "duration", "videoSeconds"), 1),
	}
}

func readJSONAIRequest(body []byte) aiRequestMeta {
	var payload map[string]any
	_ = json.Unmarshal(body, &payload)
	return aiRequestMeta{
		ModelName:      readStringField(payload, "model"),
		Count:          readIntField(payload, 1, "n"),
		Size:           readStringField(payload, "size", "ratio"),
		Quality:        readStringField(payload, "quality"),
		Resolution:     readStringField(payload, "resolution", "resolution_name", "vquality"),
		ResolutionTier: readStringField(payload, "resolutionTier"),
		Duration:       readIntField(payload, 1, "duration", "seconds", "videoSeconds"),
	}
}

func firstFormValue(form *multipart.Form, keys ...string) string {
	for _, key := range keys {
		if values := form.Value[key]; len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func readStringField(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		if text, ok := value.(string); ok {
			return text
		}
		return fmt.Sprint(value)
	}
	return ""
}

func readIntField(payload map[string]any, defaultValue int, keys ...string) int {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int(typed)
		case string:
			return readIntValue(typed, defaultValue)
		}
	}
	return defaultValue
}

func readIntValue(value string, defaultValue int) int {
	result := defaultValue
	if strings.TrimSpace(value) == "" {
		return result
	}
	_, _ = fmt.Sscan(value, &result)
	return result
}

func pricingRequestForAIPath(path string, request aiRequestMeta) service.PricingRequest {
	pricing := service.PricingRequest{
		Model:          request.ModelName,
		ResolutionTier: request.ResolutionTier,
		Size:           request.Size,
		Resolution:     request.Resolution,
		Quantity:       1,
	}
	switch path {
	case "/images/generations":
		pricing.Modality = "image"
		pricing.Operation = "generation"
		pricing.Unit = "image"
		pricing.Quantity = request.Count
	case "/images/edits":
		pricing.Modality = "image"
		pricing.Operation = "edit"
		pricing.Unit = "image"
		pricing.Quantity = request.Count
	case "/videos":
		pricing.Modality = "video"
		pricing.Operation = "generation"
		pricing.Unit = "second"
		pricing.Quantity = request.Duration
	case "/chat/completions":
		pricing.Modality = "text"
		pricing.Operation = "completion"
		pricing.Unit = "request"
	case "/audio/speech":
		pricing.Modality = "audio"
		pricing.Operation = "speech"
		pricing.Unit = "request"
	}
	return pricing
}

var errMissingModel = &aiError{"缺少模型名称"}

func resolveAIProxyPath(baseURL string, modelName string, path string) string {
	if isVideoGenerationsAPI(baseURL, modelName) {
		if path == "/videos" {
			return "/videos/generations"
		}
		if strings.HasPrefix(path, "/videos/") {
			id := strings.TrimPrefix(path, "/videos/")
			id = strings.TrimSuffix(id, "/content")
			return "/videos/" + id
		}
	}
	if !isArkSeedanceVideo(baseURL, modelName) {
		return path
	}
	if path == "/videos" {
		return "/contents/generations/tasks"
	}
	if strings.HasPrefix(path, "/videos/") && !strings.HasSuffix(path, "/content") {
		return "/contents/generations/tasks/" + strings.TrimPrefix(path, "/videos/")
	}
	return path
}

func isArkSeedanceVideo(baseURL string, modelName string) bool {
	base := strings.ToLower(baseURL)
	model := strings.ToLower(modelName)
	return strings.Contains(model, "seedance") || strings.Contains(model, "doubao-seedance") || strings.Contains(base, "/api/plan/v3")
}

func isVideoGenerationsAPI(baseURL string, modelName string) bool {
	base := strings.ToLower(baseURL)
	model := strings.ToLower(modelName)
	return strings.Contains(base, "vividai.run") || strings.Contains(base, "api.x.ai") || strings.Contains(model, "grok-imagine")
}

func adaptAIRequestBody(baseURL string, modelName string, path string, body []byte, contentType string) ([]byte, string) {
	if isVividAI(baseURL) && strings.HasPrefix(path, "/images/") {
		return adaptVividAIImageRequestBody(modelName, body, contentType)
	}
	if !isVideoGenerationsAPI(baseURL, modelName) || path != "/videos" {
		return body, contentType
	}
	payload, ok := videoGenerationsPayload(body, contentType)
	if !ok {
		return body, contentType
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return body, contentType
	}
	return encoded, "application/json"
}

func isVividAI(baseURL string) bool {
	return strings.Contains(strings.ToLower(baseURL), "vividai.run")
}

func adaptVividAIImageRequestBody(modelName string, body []byte, contentType string) ([]byte, string) {
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return body, contentType
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, contentType
	}
	if size := normalizeVividAIImageSize(modelName, payloadString(payload, "size", "ratio")); size != "" {
		payload["size"] = size
	}
	if tier := normalizeVividAIImageResolution(modelName, payloadString(payload, "resolution", "resolution_name", "resolutionTier")); tier != "" {
		payload["resolution"] = tier
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return body, contentType
	}
	return encoded, "application/json"
}

func normalizeVividAIImageSize(modelName string, value string) string {
	ratio := normalizeRatioName(value)
	model := strings.ToLower(strings.TrimSpace(modelName))
	allowed := []string{"1:1", "16:9", "9:16", "4:3", "3:4"}
	if strings.Contains(model, "firefly-gpt-image") {
		allowed = []string{"1:1", "5:4", "9:16", "21:9", "16:9", "4:3", "3:2", "4:5", "3:4", "2:3"}
	}
	if stringIn(ratio, allowed) {
		return ratio
	}
	return "1:1"
}

func normalizeVividAIImageResolution(modelName string, value string) string {
	tier := strings.ToLower(strings.TrimSpace(value))
	if strings.HasSuffix(tier, "p") {
		tier = ""
	}
	model := strings.ToLower(strings.TrimSpace(modelName))
	allowed := []string{"1k"}
	if strings.Contains(model, "firefly-image-5") {
		allowed = []string{"1k", "2k"}
	}
	if strings.Contains(model, "firefly-gpt-image") {
		allowed = []string{"1k", "2k", "4k"}
	}
	if stringIn(tier, allowed) {
		return tier
	}
	return allowed[0]
}

func videoGenerationsPayload(body []byte, contentType string) (map[string]any, bool) {
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return multipartVideoGenerationsPayload(body, contentType)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false
	}
	return normalizeVideoGenerationsPayload(payload), true
}

func multipartVideoGenerationsPayload(body []byte, contentType string) (map[string]any, bool) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, false
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	form, err := reader.ReadForm(32 << 20)
	if err != nil {
		return nil, false
	}
	defer form.RemoveAll()
	payload := map[string]any{}
	for key, values := range form.Value {
		if len(values) > 0 {
			payload[key] = values[0]
		}
	}
	if references := multipartVideoGenerationReferences(form); len(references) > 0 {
		payload["input_reference"] = references
	}
	return normalizeVideoGenerationsPayload(payload), true
}

func multipartVideoGenerationReferences(form *multipart.Form) []string {
	files := form.File["input_reference[]"]
	if len(files) == 0 {
		files = form.File["input_reference"]
	}
	references := []string{}
	for _, header := range files {
		file, err := header.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(file, 16<<20))
		_ = file.Close()
		if err != nil || len(data) == 0 {
			continue
		}
		references = append(references, base64.StdEncoding.EncodeToString(data))
	}
	return references
}

func normalizeVideoGenerationsPayload(payload map[string]any) map[string]any {
	result := map[string]any{}
	modelName := payloadString(payload, "model")
	if modelName != "" {
		result["model"] = modelName
	}
	copyStringField(result, payload, "prompt", "prompt")
	if duration := normalizeVideoGenerationDuration(modelName, payloadString(payload, "duration", "seconds", "videoSeconds")); duration != "" {
		result["duration"] = duration
	}
	if resolution := normalizeVideoGenerationResolution(modelName, payloadString(payload, "resolution", "resolution_name", "vquality")); resolution != "" {
		result["resolution"] = resolution
	}
	if ratio := normalizeVideoGenerationRatio(modelName, payloadString(payload, "ratio", "aspect_ratio", "size")); ratio != "" {
		result["aspect_ratio"] = ratio
	}
	if references := normalizeVideoReferenceImages(firstAnyValue(payload, "reference_images", "input_reference", "input_reference[]")); references != nil {
		result["reference_images"] = references
	}
	return result
}

func normalizeVideoReferenceImages(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case []string:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if encoded := normalizeBase64Reference(item); encoded != "" {
				result = append(result, encoded)
			}
		}
		return result
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if encoded := normalizeBase64Reference(fmt.Sprint(item)); encoded != "" {
				result = append(result, encoded)
			}
		}
		return result
	case string:
		if encoded := normalizeBase64Reference(typed); encoded != "" {
			return []string{encoded}
		}
	}
	return nil
}

func normalizeBase64Reference(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if comma := strings.Index(value, ","); comma >= 0 && strings.Contains(strings.ToLower(value[:comma]), "base64") {
		value = value[comma+1:]
	}
	return strings.TrimSpace(value)
}

func normalizeVideoGenerationDuration(modelName string, value string) string {
	duration := readIntValue(value, 0)
	model := strings.ToLower(strings.TrimSpace(modelName))
	if strings.Contains(model, "gemini") || strings.Contains(model, "veo") {
		if duration > 6 {
			return "8s"
		}
		return "6s"
	}
	if strings.Contains(model, "ray") {
		if duration > 5 {
			return "10s"
		}
		return "5s"
	}
	if strings.Contains(model, "firefly") {
		return "5s"
	}
	if duration <= 0 {
		return "5"
	}
	return fmt.Sprint(duration)
}

func normalizeVideoGenerationResolution(modelName string, value string) string {
	resolution := normalizeResolutionName(value)
	model := strings.ToLower(strings.TrimSpace(modelName))
	if strings.Contains(model, "ray") {
		return "720p"
	}
	if strings.Contains(model, "firefly-video") {
		return "720p"
	}
	if strings.Contains(model, "gemini") || strings.Contains(model, "veo") {
		if resolution == "1080p" {
			return "1080p"
		}
		return "720p"
	}
	switch resolution {
	case "540p", "720p", "1080p":
		return resolution
	case "":
		return "720p"
	default:
		return "720p"
	}
}

func normalizeResolutionName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "auto", "medium", "high":
		return "720p"
	case "low", "480p", "480":
		return "540p"
	case "540", "720", "1080":
		return value + "p"
	default:
		return value
	}
}

func normalizeVideoGenerationRatio(modelName string, value string) string {
	ratio := normalizeRatioName(value)
	model := strings.ToLower(strings.TrimSpace(modelName))
	if strings.Contains(model, "gemini") || strings.Contains(model, "veo") {
		if ratio == "9:16" {
			return "9:16"
		}
		return "16:9"
	}
	if strings.Contains(model, "firefly-video") {
		return "16:9"
	}
	if strings.Contains(model, "ray") && !stringIn(ratio, []string{"21:9", "16:9", "4:3", "1:1", "3:4", "9:16", "9:21"}) {
		return "16:9"
	}
	if ratio == "" {
		return "16:9"
	}
	return ratio
}

func normalizeRatioName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "x", ":")
	switch value {
	case "1280:720", "1920:1080", "16:9":
		return "16:9"
	case "720:1280", "1080:1920", "9:16":
		return "9:16"
	case "1024:1024", "1:1":
		return "1:1"
	case "21:9", "4:3", "3:4", "9:21":
		return value
	default:
		return value
	}
}

func payloadString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		return strings.TrimSpace(fmt.Sprint(value))
	}
	return ""
}

func firstAnyValue(payload map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			return value
		}
	}
	return nil
}

func stringIn(value string, items []string) bool {
	for _, item := range items {
		if value == item {
			return true
		}
	}
	return false
}

func copyStringField(result map[string]any, payload map[string]any, target string, keys ...string) {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" {
			result[target] = text
		}
		return
	}
}

func copyAnyField(result map[string]any, payload map[string]any, target string, keys ...string) {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			result[target] = value
			return
		}
	}
}

func aiResponseAdapter(baseURL string, modelName string, path string) func([]byte) ([]byte, string, bool) {
	if !isVideoGenerationsAPI(baseURL, modelName) || !strings.HasPrefix(path, "/videos") {
		return nil
	}
	return adaptVideoGenerationsResponse
}

func adaptVideoGenerationsResponse(body []byte) ([]byte, string, bool) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "", false
	}
	if id := cacheVideoGenerationsB64JSON(payload); id != "" {
		encoded, err := json.Marshal(map[string]any{"id": id, "status": "completed"})
		return encoded, "application/json", err == nil
	}
	result := map[string]any{}
	id := firstStringValue(payload, "id", "request_id")
	if id != "" {
		result["id"] = id
	}
	if status := videoGenerationsStatus(payload); status != "" {
		result["status"] = status
	}
	if message := nestedStringValue(payload, "error", "message"); message != "" {
		result["error"] = map[string]string{"message": message}
	}
	if len(result) == 0 {
		return nil, "", false
	}
	encoded, err := json.Marshal(result)
	return encoded, "application/json", err == nil
}

func cacheVideoGenerationsB64JSON(payload map[string]any) string {
	data, ok := payload["data"].([]any)
	if !ok || len(data) == 0 {
		return ""
	}
	item, ok := data[0].(map[string]any)
	if !ok {
		return ""
	}
	text := firstStringValue(item, "b64_json")
	if text == "" {
		return ""
	}
	body, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return ""
	}
	mimeType := "video/mp4"
	if strings.HasPrefix(http.DetectContentType(body), "video/") {
		mimeType = http.DetectContentType(body)
	}
	return cacheVideoGeneration(body, mimeType)
}

func videoGenerationsStatus(payload map[string]any) string {
	status := strings.ToLower(firstStringValue(payload, "status"))
	if done, ok := payload["done"].(bool); ok {
		if done {
			return "completed"
		}
		if status == "" {
			return "running"
		}
	}
	switch status {
	case "succeeded", "success", "completed", "done":
		return "completed"
	case "failed", "cancelled", "canceled":
		return status
	case "queued", "pending", "running", "processing", "in_progress":
		return "running"
	default:
		return status
	}
}

func videoGenerationsContentURL(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if url := firstStringValue(payload, "url", "video_url"); url != "" {
		return url
	}
	if url := nestedStringValue(payload, "content", "video_url"); url != "" {
		return url
	}
	if url := nestedStringValue(payload, "video", "url"); url != "" {
		return url
	}
	if data, ok := payload["data"].([]any); ok {
		for _, item := range data {
			if object, ok := item.(map[string]any); ok {
				if url := firstStringValue(object, "url", "video_url"); url != "" {
					return url
				}
			}
		}
	}
	return ""
}

func firstStringValue(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func nestedStringValue(payload map[string]any, key string, nestedKey string) string {
	object, ok := payload[key].(map[string]any)
	if !ok {
		return ""
	}
	return firstStringValue(object, nestedKey)
}

func aiStatusMessage(statusCode int) string {
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "AI 接口鉴权失败，请检查 API Key、套餐权限或模型权限"
	case http.StatusTooManyRequests:
		return "AI 接口限流或额度不足，请稍后重试或检查额度"
	default:
		return "AI 接口请求失败"
	}
}

func aiUpstreamStatusMessage(statusCode int, body []byte) string {
	base := aiStatusMessage(statusCode)
	detail := aiUpstreamErrorDetail(body)
	if detail == "" {
		return base
	}
	return base + "：" + detail
}

func aiUpstreamErrorDetail(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}
	var payload struct {
		Msg     string `json:"msg"`
		Message string `json:"message"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if payload.Error.Message != "" {
			if detail := friendlyUpstreamError(payload.Error.Code, payload.Error.Message); detail != "" {
				return safeUpstreamText(detail)
			}
			if payload.Error.Code != "" {
				return safeUpstreamText(payload.Error.Code + " " + payload.Error.Message)
			}
			return safeUpstreamText(payload.Error.Message)
		}
		if payload.Msg != "" {
			return safeUpstreamText(payload.Msg)
		}
		if payload.Message != "" {
			return safeUpstreamText(payload.Message)
		}
	}
	return safeUpstreamText(text)
}

func friendlyUpstreamError(code string, message string) string {
	lowerCode := strings.ToLower(strings.TrimSpace(code))
	if strings.Contains(lowerCode, "inputvideosensitivecontentdetected") || strings.Contains(lowerCode, "privacyinformation") {
		return strings.TrimSpace(code + " 参考视频疑似包含真人或隐私信息，火山方舟拒绝使用普通 URL 作为真人视频参考；请改用不含真人的视频、官方允许的模型产物，或已授权的 asset:// 素材。原始错误：" + message)
	}
	return ""
}

func safeUpstreamText(text string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	runes := []rune(text)
	if len(runes) > 300 {
		return string(runes[:300]) + "..."
	}
	return text
}

type aiError struct {
	message string
}

func (err *aiError) Error() string {
	return err.message
}
