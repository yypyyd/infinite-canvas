package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/yypyyd/infinite-canvas/model"
)

const modelChannelProtocolCustomHTTP = "custom_http"

var protocolVariablePattern = regexp.MustCompile(`\$\{([a-z_][a-z_0-9]*)\}`)

func vividAIProtocolTemplate() model.ProtocolTemplate {
	response := model.ProtocolResponse{Format: "ndjson", TaskID: "jobId", Status: "status", Running: []string{"running"}, Success: []string{"succeeded"}, Failure: []string{"failed"}, ResultURL: "result", Error: "error"}
	resume := &model.ProtocolRequest{Method: "POST", Path: "/v1/generate", Headers: map[string]string{"X-Wait-Seconds": "20"}, Body: map[string]any{"jobId": "${task_id}"}, Response: response}
	operation := model.ProtocolOperation{
		Create: model.ProtocolRequest{Method: "POST", Path: "/v1/generate", Headers: map[string]string{"X-Wait-Seconds": "20"}, Body: map[string]any{"model": "${model}", "prompt": "${prompt}", "quality": "${resolution}", "ratio": "${ratio}", "refs": "${reference_keys}"}, Response: response},
		Resume: resume, TimeoutSeconds: 7200, PollSeconds: 5, MaxImages: 1,
		ValueMaps: map[string]map[string]any{"resolution": {"1k": "1K", "2k": "2K", "4k": "4K"}},
	}
	video := operation
	video.Create.Body = map[string]any{"model": "${model}", "prompt": "${prompt}", "quality": "${resolution}", "duration": "${seconds}", "refs": "${reference_keys}"}
	return model.ProtocolTemplate{
		ID: "vividai", Name: "VividAI", Revision: 1, DefaultBaseURL: "https://vividai.run",
		Headers:    map[string]string{"Authorization": "Bearer ${api_key}"},
		Models:     &model.ProtocolModels{Request: model.ProtocolRequest{Method: "GET", Path: "/v1/models", Response: model.ProtocolResponse{Format: "json"}}, Items: "data", ID: "id"},
		Upload:     &model.ProtocolUpload{Mode: "presigned", Request: model.ProtocolRequest{Method: "POST", Path: "/v1/uploads", Body: map[string]any{"kind": "${kind}", "mime": "${mime}"}, Response: model.ProtocolResponse{Format: "json"}}, URL: "url", Key: "key"},
		Operations: map[string]model.ProtocolOperation{"image.generation": operation, "image.edit": operation, "video.generation": video},
	}
}

func normalizeProtocolTemplates(items []model.ProtocolTemplate) []model.ProtocolTemplate {
	for _, item := range items {
		if item.ID == "vividai" {
			return items
		}
	}
	return append(items, vividAIProtocolTemplate())
}

func IsCustomHTTPChannel(channel model.ModelChannel) bool {
	return channel.Protocol == modelChannelProtocolCustomHTTP
}

func findProtocolTemplate(items []model.ProtocolTemplate, id string) (model.ProtocolTemplate, error) {
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return model.ProtocolTemplate{}, safeMessageError{message: "渠道引用的协议模板不存在"}
}

func validateProtocolTemplates(settings model.PrivateSetting) error {
	if len(settings.ProtocolTemplates) > 64 {
		return errors.New("协议模板最多 64 个")
	}
	ids := map[string]bool{}
	for _, item := range settings.ProtocolTemplates {
		if ids[item.ID] {
			return fmt.Errorf("协议模板 ID 重复：%s", item.ID)
		}
		ids[item.ID] = true
		if err := ValidateProtocolTemplate(item); err != nil {
			return fmt.Errorf("协议模板 %s：%w", item.Name, err)
		}
	}
	channelIDs := map[string]bool{}
	for _, channel := range settings.Channels {
		if channel.ID == "" || channelIDs[channel.ID] {
			return errors.New("渠道 ID 不能为空或重复")
		}
		channelIDs[channel.ID] = true
		if !IsCustomHTTPChannel(channel) {
			continue
		}
		if !validProtocolBaseURL(channel.BaseURL) {
			return errors.New("自定义协议渠道必须使用有效的 HTTPS 根地址，不含查询参数")
		}
		template, err := findProtocolTemplate(settings.ProtocolTemplates, channel.TemplateID)
		if err != nil {
			return err
		}
		for _, item := range channel.Models {
			for _, operation := range item.Operations {
				if _, ok := template.Operations[item.Modality+"."+operation]; !ok {
					return fmt.Errorf("模型 %s 的 %s.%s 没有对应协议操作", item.Model, item.Modality, operation)
				}
			}
		}
	}
	return nil
}

func validProtocolBaseURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

func ValidateProtocolTemplate(template model.ProtocolTemplate) error {
	encoded, err := json.Marshal(template)
	if err != nil || len(encoded) > 64<<10 {
		return errors.New("模板必须是有效 JSON，且不能超过 64KB")
	}
	if !comfyUIWorkflowIDPattern.MatchString(template.ID) || strings.TrimSpace(template.Name) == "" || template.Revision < 1 {
		return errors.New("请填写有效 ID、名称和正整数版本号")
	}
	if template.DefaultBaseURL != "" && !validProtocolBaseURL(template.DefaultBaseURL) {
		return errors.New("默认地址必须使用 HTTPS")
	}
	if err := validateProtocolHeaders(template.Headers); err != nil {
		return err
	}
	if len(template.Operations) == 0 || len(template.Operations) > 3 {
		return errors.New("请配置图片生成、图片编辑或视频生成操作")
	}
	for key, operation := range template.Operations {
		if key != "image.generation" && key != "image.edit" && key != "video.generation" {
			return fmt.Errorf("不支持的操作 %s", key)
		}
		if operation.TimeoutSeconds < 60 || operation.TimeoutSeconds > 7200 || operation.PollSeconds < 2 || operation.PollSeconds > 60 || operation.MaxImages < 1 || operation.MaxImages > 10 {
			return errors.New("总时限须为 60–7200 秒，查询间隔为 2–60 秒，单次图片数为 1–10")
		}
		if err := validateProtocolRequest(operation.Create, true); err != nil {
			return err
		}
		if operation.Resume != nil {
			if operation.Create.Response.TaskID == "" || operation.Create.Response.Status == "" {
				return errors.New("可恢复任务必须声明创建响应的任务 ID 和状态字段")
			}
			if err := validateProtocolRequest(*operation.Resume, true); err != nil {
				return err
			}
			resumeJSON, _ := json.Marshal(operation.Resume)
			if !strings.Contains(string(resumeJSON), "${task_id}") {
				return errors.New("恢复请求必须使用 ${task_id}")
			}
			for _, variable := range protocolVariablePattern.FindAllStringSubmatch(string(resumeJSON), -1) {
				if variable[1] != "task_id" && variable[1] != "model" && variable[1] != "request_id" && variable[1] != "api_key" {
					return errors.New("恢复请求只能引用 task_id、model、request_id 和请求头中的 api_key")
				}
			}
		}
	}
	if template.Models != nil {
		if template.Models.ID == "" || template.Models.Items == "" || template.Models.Request.Method != "GET" {
			return errors.New("模型列表使用 GET，并填写列表路径及模型 ID 路径")
		}
		if err := validateProtocolRequest(template.Models.Request, false); err != nil {
			return err
		}
	}
	if upload := template.Upload; upload != nil {
		switch upload.Mode {
		case "url":
		case "presigned", "multipart":
			if upload.Key == "" || upload.Mode == "presigned" && upload.URL == "" {
				return errors.New("上传配置缺少 key 或预签名 URL 路径")
			}
			if err := validateProtocolRequest(upload.Request, false); err != nil {
				return err
			}
			if upload.Request.Method != "POST" || upload.Request.Response.Format != "json" || (upload.Mode == "multipart") != (upload.Request.Encoding == "multipart") {
				return errors.New("上传须使用 POST / JSON 响应，multipart 模式须使用 multipart 编码")
			}
		default:
			return errors.New("上传模式仅支持 url、multipart、presigned")
		}
	}
	return nil
}

func validateProtocolRequest(request model.ProtocolRequest, generation bool) error {
	if request.Method != http.MethodGet && request.Method != http.MethodPost {
		return errors.New("接口请求方法仅支持 GET 和 POST")
	}
	if !strings.HasPrefix(request.Path, "/") || strings.HasPrefix(request.Path, "//") || strings.ContainsAny(request.Path, "\r\n#") || strings.Contains(request.Path, "${api_key}") {
		return errors.New("接口路径必须是以 / 开头的相对路径，不能包含密钥")
	}
	if request.Encoding != "" && request.Encoding != "json" && request.Encoding != "multipart" {
		return errors.New("请求编码仅支持 json 和 multipart")
	}
	if request.Encoding == "multipart" && strings.TrimSpace(request.FileField) == "" {
		return errors.New("multipart 请求必须配置文件字段 fileField")
	}
	if err := validateProtocolText(request.Path, false); err != nil {
		return err
	}
	if request.Response.Format != "json" && request.Response.Format != "ndjson" {
		return errors.New("响应格式仅支持 json 和 ndjson")
	}
	if err := validateProtocolHeaders(request.Headers); err != nil {
		return err
	}
	if err := validateProtocolValue(request.Body); err != nil {
		return err
	}
	if generation {
		response := request.Response
		if response.ResultURL == "" && response.ResultBase64 == "" {
			return errors.New("生成响应必须配置结果 URL 或 Base64 路径")
		}
		if response.Status != "" && (len(response.Success) == 0 || len(response.Failure) == 0 || len(response.Running) == 0) {
			return errors.New("声明状态字段时必须填写运行中、成功和失败值")
		}
		states := map[string]bool{}
		for _, list := range [][]string{response.Running, response.Success, response.Failure} {
			for _, state := range list {
				if state == "" || states[state] {
					return errors.New("任务状态值不能为空或重复")
				}
				states[state] = true
			}
		}
	}
	return nil
}

func validateProtocolHeaders(headers map[string]string) error {
	for key, value := range headers {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key+value, "\r\n") {
			return errors.New("请求头名称或内容无效")
		}
		if err := validateProtocolText(value, true); err != nil {
			return err
		}
		lower := strings.ToLower(key)
		if lower != "idempotency-key" && (strings.Contains(lower, "authorization") || strings.Contains(lower, "key") || strings.Contains(lower, "token")) && !strings.Contains(value, "${api_key}") {
			return errors.New("鉴权请求头请使用 ${api_key}，密钥单独保存在渠道中")
		}
	}
	return nil
}

func validateProtocolText(value string, allowKey bool) error {
	for _, match := range protocolVariablePattern.FindAllStringSubmatch(value, -1) {
		if !protocolVariableAllowed(match[1]) && !(allowKey && match[1] == "api_key") {
			return fmt.Errorf("不支持的占位符 %s", match[0])
		}
	}
	if strings.Contains(protocolVariablePattern.ReplaceAllString(value, ""), "${") {
		return errors.New("占位符格式无效")
	}
	return nil
}

func validateProtocolValue(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			if err := validateProtocolValue(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := validateProtocolValue(child); err != nil {
				return err
			}
		}
	case string:
		if !strings.Contains(typed, "${") {
			return nil
		}
		name := strings.TrimSuffix(strings.TrimPrefix(typed, "${"), "}")
		if typed != "${"+name+"}" || !protocolVariableAllowed(name) {
			return fmt.Errorf("不支持的占位符 %s；占位符须独占整个 JSON 值", typed)
		}
	}
	return nil
}

func protocolVariableAllowed(name string) bool {
	switch name {
	case "model", "prompt", "negative_prompt", "size", "width", "height", "ratio", "resolution", "seconds", "count", "seed", "generate_audio", "task_id", "request_id", "reference_keys", "reference_urls", "reference_image_urls", "reference_video_urls", "reference_audio_urls", "mime", "kind":
		return true
	}
	return false
}

func protocolText(value string, variables map[string]any, escape bool) (string, error) {
	var renderErr error
	result := protocolVariablePattern.ReplaceAllStringFunc(value, func(placeholder string) string {
		name := placeholder[2 : len(placeholder)-1]
		resolved, ok := variables[name]
		if !ok {
			renderErr = fmt.Errorf("缺少变量 %s", name)
			return ""
		}
		text := fmt.Sprint(resolved)
		if escape {
			text = strings.ReplaceAll(url.QueryEscape(text), "+", "%20")
		}
		return text
	})
	return result, renderErr
}

func protocolField(raw []byte, path string) gjson.Result {
	if path == "" {
		return gjson.Result{}
	}
	return gjson.GetBytes(raw, path)
}

type ProtocolPreviewInput struct {
	Template  model.ProtocolTemplate `json:"template"`
	Operation string                 `json:"operation"`
	Phase     string                 `json:"phase"`
	Variables map[string]any         `json:"variables"`
	Response  string                 `json:"response"`
}

// PreviewProtocol only renders supplied examples; it never makes an HTTP request.
func PreviewProtocol(input ProtocolPreviewInput) (map[string]any, error) {
	if err := ValidateProtocolTemplate(input.Template); err != nil {
		return nil, err
	}
	operation, ok := input.Template.Operations[input.Operation]
	if !ok {
		return nil, errors.New("请选择模板中的操作")
	}
	request := operation.Create
	if input.Phase == "resume" {
		if operation.Resume == nil {
			return nil, errors.New("此操作没有恢复请求")
		}
		request = *operation.Resume
	}
	variables := input.Variables
	if variables == nil {
		variables = map[string]any{}
	}
	variables["api_key"] = "***"
	body, _, err := renderMediaTemplateValue(request.Body, variables, operation.ValueMaps)
	if err != nil {
		return nil, err
	}
	path, err := protocolText(request.Path, variables, true)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{}
	for _, group := range []map[string]string{input.Template.Headers, request.Headers} {
		for name, value := range group {
			headers[name], err = protocolText(value, variables, false)
			if err != nil {
				return nil, err
			}
		}
	}
	result := map[string]any{"method": request.Method, "path": path, "headers": headers, "body": body}
	if strings.TrimSpace(input.Response) != "" {
		frames := []protocolTaskResult{}
		err := decodeProtocolResponse(strings.NewReader(input.Response), request.Response, func(frame protocolTaskResult) error { frames = append(frames, frame); return nil })
		if err != nil {
			return nil, err
		}
		result["frames"] = frames
	}
	return result, nil
}
