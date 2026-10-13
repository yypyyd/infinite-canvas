package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

var protocolHTTPClient = &http.Client{Transport: batchResultTransport(), Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

var errProtocolConnection = errors.New("上游连接中断或等待超时")

type protocolFile struct {
	Kind string
	Mime string
	Data []byte
	URL  string
	Size int64
}

type protocolTaskResult struct {
	TaskID string   `json:"taskId,omitempty"`
	Status string   `json:"status"`
	URLs   []string `json:"urls,omitempty"`
	Base64 []string `json:"base64,omitempty"`
	Error  string   `json:"error,omitempty"`
}

func protocolRequest(ctx context.Context, channel model.ModelChannel, template model.ProtocolTemplate, request model.ProtocolRequest, variables map[string]any, maps map[string]map[string]any, files []protocolFile) (*http.Response, error) {
	if !validProtocolBaseURL(channel.BaseURL) {
		return nil, errors.New("自定义协议地址必须使用 HTTPS")
	}
	path, err := protocolText(request.Path, variables, true)
	if err != nil {
		return nil, err
	}
	rendered, _, err := renderMediaTemplateValue(request.Body, variables, maps)
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	contentType := "application/json"
	if request.Encoding == "multipart" {
		writer := multipart.NewWriter(&body)
		contentType = writer.FormDataContentType()
		if fields, ok := rendered.(map[string]any); ok {
			for key, value := range fields {
				text := fmt.Sprint(value)
				switch value.(type) {
				case []any, []string, map[string]any:
					encoded, _ := json.Marshal(value)
					text = string(encoded)
				}
				if err := writer.WriteField(key, text); err != nil {
					return nil, err
				}
			}
		}
		if len(files) > 0 && request.FileField == "" {
			return nil, errors.New("multipart 请求未配置文件字段")
		}
		for i, file := range files {
			header := textproto.MIMEHeader{}
			header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": request.FileField, "filename": fmt.Sprintf("reference-%d%s", i+1, assetFileExt(file.Mime))}))
			header.Set("Content-Type", file.Mime)
			part, err := writer.CreatePart(header)
			if err != nil {
				return nil, err
			}
			if _, err := part.Write(file.Data); err != nil {
				return nil, err
			}
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
	} else if request.Body != nil {
		if err := json.NewEncoder(&body).Encode(rendered); err != nil {
			return nil, err
		}
	}
	upstream, err := http.NewRequestWithContext(ctx, request.Method, normalizeModelChannelBaseURL(channel.BaseURL)+path, &body)
	if err != nil {
		return nil, errors.New("协议请求地址无效")
	}
	variables["api_key"] = channel.APIKey
	defer delete(variables, "api_key")
	upstream.Header.Set("Content-Type", contentType)
	for _, headers := range []map[string]string{template.Headers, request.Headers} {
		for key, value := range headers {
			text, err := protocolText(value, variables, false)
			if err != nil {
				return nil, err
			}
			upstream.Header.Set(key, text)
		}
	}
	response, err := protocolHTTPClient.Do(upstream)
	if err != nil {
		return nil, errProtocolConnection
	}
	return response, nil
}

func readProtocolJSON(response *http.Response) ([]byte, error) {
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxGenerationTaskStatusSize+1))
	if err != nil || len(body) > maxGenerationTaskStatusSize || !json.Valid(body) {
		return nil, errors.New("上游响应不是有效的 JSON 或响应过大")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("上游返回 HTTP %d", response.StatusCode)
	}
	return body, nil
}

func decodeProtocolResponse(reader io.Reader, mapping model.ProtocolResponse, consume func(protocolTaskResult) error) error {
	decoder := json.NewDecoder(io.LimitReader(reader, maxStandardBatchResponseBytes+1))
	frames := 0
	for frames < 256 {
		var raw json.RawMessage
		err := decoder.Decode(&raw)
		if errors.Is(err, io.EOF) && frames > 0 {
			return nil
		}
		if err != nil {
			return errors.New("上游结果流中断或 JSON 无效")
		}
		if decoder.InputOffset() > maxStandardBatchResponseBytes {
			return errors.New("上游结果响应过大")
		}
		frames++
		result, err := parseProtocolResult(raw, mapping)
		if err != nil {
			if result.TaskID != "" {
				if receiptErr := consume(protocolTaskResult{TaskID: result.TaskID, Status: "running"}); receiptErr != nil {
					return receiptErr
				}
			}
			return err
		}
		if err := consume(result); err != nil {
			return err
		}
		if mapping.Format == "json" || result.Status == "success" || result.Status == "failed" {
			return nil
		}
	}
	return errors.New("上游结果帧数量过多")
}

func parseProtocolResult(raw []byte, mapping model.ProtocolResponse) (protocolTaskResult, error) {
	result := protocolTaskResult{TaskID: protocolField(raw, mapping.TaskID).String()}
	stringsAt := func(path string) []string {
		field := protocolField(raw, path)
		result := []string{}
		if field.IsArray() {
			for _, item := range field.Array() {
				if item.Str != "" {
					result = append(result, item.Str)
				}
			}
		} else if field.Str != "" {
			result = append(result, field.Str)
		}
		return result
	}
	status := protocolField(raw, mapping.Status).String()
	switch {
	case mapping.Status == "" || containsPricingValue(mapping.Success, status):
		result.Status = "success"
		result.URLs, result.Base64 = stringsAt(mapping.ResultURL), stringsAt(mapping.ResultBase64)
		if len(result.URLs)+len(result.Base64) == 0 || len(result.URLs)+len(result.Base64) > 10 {
			return result, errors.New("上游成功响应缺少结果，或结果数量超过限制")
		}
	case containsPricingValue(mapping.Failure, status):
		result.Status = "failed"
		result.Error = protocolField(raw, mapping.Error).String()
		if result.Error == "" {
			result.Error = "上游生成失败"
		}
	case containsPricingValue(mapping.Running, status):
		result.Status = "running"
	default:
		return result, errors.New("上游返回了模板未声明的任务状态")
	}
	return result, nil
}

func fetchProtocolModels(channel model.ModelChannel) ([]model.DiscoveredModel, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return nil, err
	}
	template, err := findProtocolTemplate(normalizeProtocolTemplates(settings.Private.ProtocolTemplates), channel.TemplateID)
	if err != nil {
		return nil, err
	}
	if template.Models == nil {
		return nil, errors.New("模板没有配置模型发现接口，请手动添加模型")
	}
	response, err := protocolRequest(context.Background(), channel, template, template.Models.Request, map[string]any{}, nil, nil)
	if err != nil {
		return nil, err
	}
	body, err := readProtocolJSON(response)
	if err != nil {
		return nil, err
	}
	items := protocolField(body, template.Models.Items)
	if !items.IsArray() {
		return nil, errors.New("模型列表字段不是数组，请检查模板中的字段路径")
	}
	result := []model.DiscoveredModel{}
	for _, item := range items.Array() {
		if id := strings.TrimSpace(item.Get(template.Models.ID).String()); id != "" {
			result = append(result, model.DiscoveredModel{ID: id})
		}
	}
	return result, nil
}

func protocolStorageFile(ctx context.Context, user model.AuthUser, key, kind string) (protocolFile, error) {
	return readProtocolStoredFile(ctx, user, key, kind, protocolReferenceLimit(kind))
}

func protocolReferenceLimit(kind string) int64 {
	limit := int64(20 << 20)
	if kind == "video" {
		limit = 200 << 20
	} else if kind == "audio" {
		limit = 50 << 20
	}
	return limit
}

func readProtocolStoredFile(ctx context.Context, user model.AuthUser, key, kind string, limit int64) (protocolFile, error) {
	if file, item, ok := OpenLocalUserWorkspaceFile(user, key); ok {
		defer file.Close()
		if assetTypeFromMime(item.MimeType) != kind || item.Size > limit {
			return protocolFile{}, errors.New("参考素材类型或大小不符合要求")
		}
		data, err := io.ReadAll(io.LimitReader(file, limit+1))
		if err != nil || int64(len(data)) > limit || len(data) == 0 {
			return protocolFile{}, errors.New("参考素材读取失败或文件过大")
		}
		return protocolFile{Kind: kind, Mime: item.MimeType, Data: data}, nil
	}
	media, err := ResolveUserWorkspaceMedia(user, key)
	if err != nil {
		return protocolFile{}, err
	}
	if assetTypeFromMime(media.MimeType) != kind || media.Size > limit {
		return protocolFile{}, errors.New("参考素材类型或大小不符合要求")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, media.URL, nil)
	if err != nil {
		return protocolFile{}, errors.New("参考素材地址无效")
	}
	response, err := protocolHTTPClient.Do(request)
	if err != nil {
		return protocolFile{}, errors.New("参考素材下载失败")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || response.StatusCode != http.StatusOK || len(data) == 0 || int64(len(data)) > limit {
		return protocolFile{}, errors.New("参考素材下载失败或文件过大")
	}
	return protocolFile{Kind: kind, Mime: media.MimeType, Data: data}, nil
}

func uploadProtocolFiles(ctx context.Context, channel model.ModelChannel, template model.ProtocolTemplate, files []protocolFile) ([]string, error) {
	keys := []string{}
	if template.Upload == nil {
		return keys, nil
	}
	for _, file := range files {
		variables := map[string]any{"kind": file.Kind, "mime": file.Mime}
		response, err := protocolRequest(ctx, channel, template, template.Upload.Request, variables, nil, []protocolFile{file})
		if err != nil {
			return nil, err
		}
		body, err := readProtocolJSON(response)
		if err != nil {
			return nil, err
		}
		key := protocolField(body, template.Upload.Key).String()
		if key == "" {
			return nil, errors.New("素材上传未返回 key，请检查模板")
		}
		if template.Upload.Mode == "presigned" {
			uploadURL := protocolField(body, template.Upload.URL).String()
			if !validHTTPSURL(uploadURL) {
				return nil, errors.New("素材上传地址无效")
			}
			request, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(file.Data))
			if err != nil {
				return nil, errors.New("素材上传地址无效")
			}
			// The signed upload is a different trust boundary: never forward API credentials.
			request.Header.Set("Content-Type", file.Mime)
			response, err := protocolHTTPClient.Do(request)
			if err != nil {
				return nil, errors.New("素材直传失败")
			}
			_ = response.Body.Close()
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				return nil, fmt.Errorf("素材直传返回 HTTP %d", response.StatusCode)
			}
		}
		keys = append(keys, key)
	}
	return keys, nil
}
