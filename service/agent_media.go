package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

// 助手的生成工具由服务端执行。这里按运行时的画布上下文解析规格并组装请求，口径与画布页面手动生成保持一致，
// 这样选路、计费和上游收到的内容都和页面发起的请求一样。

const (
	agentReferenceImageBytes = 20 << 20
	agentReferenceVideoBytes = 200 << 20
	agentReferenceAudioBytes = 50 << 20
	agentReferenceTotalBytes = 320 << 20
	agentImageCountLimit     = 15
)

var (
	agentImageDimensionPattern  = regexp.MustCompile(`^(\d+)[xX](\d+)$`)
	agentVideoRatioPattern      = regexp.MustCompile(`^\d+:\d+$`)
	agentVideoResolutionPattern = regexp.MustCompile(`^\d+(p|k)$`)
	agentStandardImageSizes     = map[string]string{"1:1": "1024x1024", "3:2": "1200x800", "2:3": "800x1200", "4:3": "1024x768", "3:4": "768x1024", "16:9": "1280x720", "9:16": "720x1280", "21:9": "1680x720"}
	agentImageQualityAliases    = map[string]string{"1k": "low", "2k": "medium", "4k": "high"}
	agentImageRequestQualities  = []string{"low", "medium", "high", "standard", "hd"}
	agentGPTImageQualityModels  = []string{"gpt-image-2.6", "gpt-image-2", "gpt-image-1.5", "gpt-image-1", "gpt-image-1-mini"}
	agentDefaultVideoRatios     = []string{"16:9", "9:16"}
	agentDefaultVideoTiers      = []string{"480p", "720p"}
	agentDefaultVideoDurations  = []int{6, 10}
)

// agentMediaPresentation 随 tool.call 下发给页面，用于生成占位节点、写生成记录并把结果落到画布。
type agentMediaPresentation struct {
	Type          string   `json:"type"`
	NodeIDs       []string `json:"nodeIds"`
	Prompt        string   `json:"prompt"`
	SourceNodeIDs []string `json:"sourceNodeIds"`
	Model         string   `json:"model"`
	Size          string   `json:"size,omitempty"`
	Quality       string   `json:"quality,omitempty"`
	Resolution    string   `json:"resolution,omitempty"`
	Seconds       int      `json:"seconds,omitempty"`
}

type agentMediaFile struct {
	Field      string
	StorageKey string
	Limit      int64
}

// agentMediaSpec 是一次服务端生成的完整规格：请求路径与字段、随请求上传的参考素材，以及页面落到画布所需的信息。
type agentMediaSpec struct {
	Path   string
	Fields map[string]string
	Count  int
	Files  []agentMediaFile
	Media  agentMediaPresentation
}

type agentMediaConfig struct {
	Model, Size, Quality, Resolution string
	GenerateAudio                    bool
	Node                             *AgentCanvasNode
	Sources                          []AgentCanvasNode
	Prompt, Text                     string
}

// agentRunCanvas 返回运行上下文里的画布，并补上本轮已经生成、页面可能还没落到画布的图片和视频节点。
func agentRunCanvas(run model.AgentRun, steps []model.AgentStep) (AgentCanvasContext, map[string]AgentCanvasNode, error) {
	var canvas AgentCanvasContext
	if err := json.Unmarshal([]byte(run.Context), &canvas); err != nil {
		return canvas, nil, errors.New("agent canvas context invalid")
	}
	nodes := make(map[string]AgentCanvasNode, len(canvas.Nodes))
	for _, node := range canvas.Nodes {
		nodes[node.ID] = node
	}
	generated := func(id, nodeType, storageKey string) {
		if node := nodes[id]; node.StorageKey == "" {
			node.ID, node.Type, node.StorageKey = id, nodeType, storageKey
			nodes[id] = node
		}
	}
	for _, step := range steps {
		var result SubmitAgentToolResultRequest
		if json.Unmarshal([]byte(step.Output), &result) != nil || result.Status != "success" {
			continue
		}
		for _, image := range result.Images {
			generated(image.NodeID, "image", image.StorageKey)
		}
		if result.Video != nil {
			generated(result.Video.NodeID, "video", result.Video.StorageKey)
		}
	}
	return canvas, nodes, nil
}

// resolveAgentMediaConfig 按“工具参数 > 配置节点 > 用户默认设置”的顺序解析规格，并取出配置节点的上游输入。
func resolveAgentMediaConfig(canvas AgentCanvasContext, nodes map[string]AgentCanvasNode, options AgentMediaOptions, mode string) (agentMediaConfig, error) {
	var config agentMediaConfig
	settings := AgentMediaOptions{}
	if options.ConfigNodeID != "" {
		node, ok := nodes[options.ConfigNodeID]
		if !ok || node.Type != "config" {
			return config, safeMessageError{message: "指定的配置节点已不存在"}
		}
		if node.Settings != nil {
			settings = *node.Settings
		}
		if settings.Mode != "" && settings.Mode != mode {
			return config, safeMessageError{message: "配置节点的生成类型与本次任务不一致"}
		}
		config.Node, config.Prompt = &node, node.Prompt
		texts := []string{}
		for _, connection := range canvas.Connections {
			source, ok := nodes[connection.From]
			if connection.To != node.ID || !ok {
				continue
			}
			config.Sources = append(config.Sources, source)
			if source.Type == "text" && source.Content != "" {
				texts = append(texts, source.Content)
			}
		}
		config.Text = strings.Join(texts, "\n\n")
	}
	if mode == "image" && options.Resolution != "" {
		return config, safeMessageError{message: "图片分辨率请通过 size 指定像素尺寸，例如 2048x2048"}
	}
	config.Model = strings.TrimSpace(firstPricingNonEmpty(options.Model, settings.Model, canvas.Defaults[mode+"Model"]))
	config.Size = strings.TrimSpace(firstPricingNonEmpty(options.Size, settings.Size, canvas.Defaults["size"]))
	config.Quality = strings.TrimSpace(firstPricingNonEmpty(options.Quality, settings.Quality, canvas.Defaults["quality"]))
	config.Resolution = strings.TrimSpace(firstPricingNonEmpty(options.Resolution, settings.Resolution, canvas.Defaults["resolution"]))
	config.GenerateAudio = canvas.Defaults["generateAudio"] == "true"
	if settings.GenerateAudio != nil {
		config.GenerateAudio = *settings.GenerateAudio
	}
	if options.GenerateAudio != nil {
		config.GenerateAudio = *options.GenerateAudio
	}
	return config, nil
}

func (config agentMediaConfig) sourceIDs(nodeType string) []string {
	ids := []string{}
	for _, source := range config.Sources {
		if source.Type == nodeType {
			ids = append(ids, source.ID)
		}
	}
	return ids
}

// prompt 合并提示词与配置节点上游的文本输入。
func (config agentMediaConfig) prompt(argument string) (string, error) {
	parts := []string{}
	for _, part := range []string{firstPricingNonEmpty(argument, config.Prompt), config.Text} {
		if strings.TrimSpace(part) != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return "", safeMessageError{message: "配置节点没有可用的提示词或文本输入"}
	}
	return strings.Join(parts, "\n\n"), nil
}

func resolveAgentImageSpec(run model.AgentRun, steps []model.AgentStep, callID, name string, arguments any) (agentMediaSpec, error) {
	var options AgentMediaOptions
	var promptArgument string
	var referenceIDs []string
	count := 1
	switch input := arguments.(type) {
	case imageGenerateArguments:
		options, promptArgument, count, referenceIDs = input.AgentMediaOptions, input.Prompt, input.Count, append([]string{}, input.ReferenceNodeIDs...)
	case imageEditArguments:
		options, promptArgument, count, referenceIDs = input.AgentMediaOptions, input.Prompt, input.Count, []string{input.NodeID}
	default:
		return agentMediaSpec{}, safeMessageError{message: "图片工具参数无效"}
	}
	canvas, nodes, err := agentRunCanvas(run, steps)
	if err != nil {
		return agentMediaSpec{}, err
	}
	settings, err := PublicSettings()
	if err != nil {
		return agentMediaSpec{}, err
	}
	config, err := resolveAgentMediaConfig(canvas, nodes, options, "image")
	if err != nil {
		return agentMediaSpec{}, err
	}
	if config.Model == "" {
		return agentMediaSpec{}, safeMessageError{message: "请先配置可用的图片模型"}
	}
	definition, known := agentModelDefinition(settings.ModelChannel.Models, config.Model)
	qualitySupported := agentImageQualitySupported(definition, config.Model)
	quality := strings.ToLower(firstPricingNonEmpty(config.Quality, "auto"))
	if quality != "auto" && agentImageRequestQuality(quality) == "" {
		return agentMediaSpec{}, safeMessageError{message: "图片质量参数不受支持，请调整规格"}
	}
	if options.Quality != "" && quality != "auto" && !qualitySupported {
		return agentMediaSpec{}, safeMessageError{message: "当前模型不支持指定的图片质量，请调整模型或规格"}
	}
	if config.Node != nil && quality != "auto" && !qualitySupported {
		return agentMediaSpec{}, safeMessageError{message: "当前模型不支持配置节点指定的图片质量"}
	}
	if len(config.sourceIDs("video"))+len(config.sourceIDs("audio")) > 0 {
		return agentMediaSpec{}, safeMessageError{message: "图片工具不支持配置节点中的视频或音频输入"}
	}
	if !qualitySupported {
		quality = "auto"
	}
	for _, id := range config.sourceIDs("image") {
		if !slices.Contains(referenceIDs, id) {
			referenceIDs = append(referenceIDs, id)
		}
	}
	if len(referenceIDs) > 0 && (!known || !slices.Contains(definition.Operations, "edit")) {
		if name == "image.edit" {
			return agentMediaSpec{}, safeMessageError{message: "当前图片模型不支持参考图编辑"}
		}
		return agentMediaSpec{}, safeMessageError{message: "当前图片模型不支持参考图，请选择支持参考图的模型"}
	}
	files := make([]agentMediaFile, 0, len(referenceIDs))
	for _, id := range referenceIDs {
		node := nodes[id]
		if node.Type != "image" || node.StorageKey == "" {
			if name == "image.edit" {
				return agentMediaSpec{}, safeMessageError{message: "未找到可用的源图片节点，源图片需要先保存到账号工作区"}
			}
			return agentMediaSpec{}, safeMessageError{message: "指定的参考图片不可用，参考图片需要先保存到账号工作区"}
		}
		files = append(files, agentMediaFile{Field: "image", StorageKey: node.StorageKey, Limit: agentReferenceImageBytes})
	}
	if known && len(files) > definition.MaxReferenceImages {
		return agentMediaSpec{}, safeMessageError{message: fmt.Sprintf("当前模型最多支持 %d 张参考图", definition.MaxReferenceImages)}
	}
	prompt, err := config.prompt(promptArgument)
	if err != nil {
		return agentMediaSpec{}, err
	}
	size, err := agentImageRequestSize(config.Size)
	if err != nil {
		return agentMediaSpec{}, err
	}
	limit := agentImageCountLimit
	if known && definition.MaxImageCount > 0 {
		limit = min(limit, definition.MaxImageCount)
	}
	count = min(max(count, 1), limit)
	requestPrompt, path := prompt, "/images/generations"
	if len(files) > 0 {
		labels := make([]string, len(files))
		for i := range files {
			labels[i] = fmt.Sprintf("图片%d", i+1)
		}
		requestPrompt, path = "参考图片编号："+strings.Join(labels, "、")+"。请按这些编号理解提示词中的图片引用。\n\n"+prompt, "/images/edits"
	}
	if systemPrompt := strings.TrimSpace(settings.ModelChannel.SystemPrompt); systemPrompt != "" {
		requestPrompt = systemPrompt + "\n\n" + requestPrompt
	}
	fields := map[string]string{"model": config.Model, "prompt": requestPrompt, "response_format": "b64_json", "output_format": "png"}
	if value := agentImageRequestQuality(quality); value != "" {
		fields["quality"] = value
	}
	if size != "" {
		fields["size"] = size
	}
	sourceIDs := referenceIDs
	if config.Node != nil {
		sourceIDs = []string{config.Node.ID}
	}
	return agentMediaSpec{Path: path, Fields: fields, Count: count, Files: files, Media: agentMediaPresentation{
		Type: "image", NodeIDs: agentGeneratedNodeIDs("image", callID, count), Prompt: prompt, SourceNodeIDs: sourceIDs,
		Model: config.Model, Size: firstPricingNonEmpty(config.Size, "auto"), Quality: quality,
	}}, nil
}

func resolveAgentVideoSpec(run model.AgentRun, steps []model.AgentStep, callID string, input videoGenerateArguments) (agentMediaSpec, error) {
	canvas, nodes, err := agentRunCanvas(run, steps)
	if err != nil {
		return agentMediaSpec{}, err
	}
	settings, err := PublicSettings()
	if err != nil {
		return agentMediaSpec{}, err
	}
	config, err := resolveAgentMediaConfig(canvas, nodes, input.AgentMediaOptions, "video")
	if err != nil {
		return agentMediaSpec{}, err
	}
	if config.Model == "" {
		return agentMediaSpec{}, safeMessageError{message: "请先配置可用的视频模型"}
	}
	definition, _ := agentModelDefinition(settings.ModelChannel.Models, config.Model)
	references := map[string][]string{"image": input.ImageNodeIDs, "video": input.VideoNodeIDs, "audio": input.AudioNodeIDs}
	labels := map[string]string{"image": "图片", "video": "视频", "audio": "音频"}
	sourceIDs := []string{}
	for _, nodeType := range []string{"image", "video", "audio"} {
		ids := append([]string{}, references[nodeType]...)
		for _, id := range config.sourceIDs(nodeType) {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
		for _, id := range ids {
			if node := nodes[id]; node.Type != nodeType || node.StorageKey == "" {
				return agentMediaSpec{}, safeMessageError{message: fmt.Sprintf("未找到指定的参考%s节点，参考素材需要先保存到账号工作区", labels[nodeType])}
			}
		}
		references[nodeType], sourceIDs = ids, append(sourceIDs, ids...)
	}
	if config.Node != nil {
		sourceIDs = []string{config.Node.ID}
	}
	prompt, err := config.prompt(input.Prompt)
	if err != nil {
		return agentMediaSpec{}, err
	}
	images, videos, audios := len(references["image"]), len(references["video"]), len(references["audio"])
	if images > max(definition.MaxReferenceImages, 0) {
		return agentMediaSpec{}, safeMessageError{message: fmt.Sprintf("当前视频模型最多支持 %d 张参考图", max(definition.MaxReferenceImages, 0))}
	}
	if videos > max(definition.MaxReferenceVideos, 0) {
		return agentMediaSpec{}, safeMessageError{message: fmt.Sprintf("当前视频模型最多支持 %d 个参考视频", max(definition.MaxReferenceVideos, 0))}
	}
	if audios > max(definition.MaxReferenceAudios, 0) {
		return agentMediaSpec{}, safeMessageError{message: fmt.Sprintf("当前视频模型最多支持 %d 个参考音频", max(definition.MaxReferenceAudios, 0))}
	}
	if definition.MaxReferenceMedia > 0 && images+videos+audios > definition.MaxReferenceMedia {
		return agentMediaSpec{}, safeMessageError{message: fmt.Sprintf("当前视频模型最多支持 %d 个参考素材", definition.MaxReferenceMedia)}
	}
	ratios, tiers, durations := agentDefaultVideoRatios, agentDefaultVideoTiers, agentDefaultVideoDurations
	ratio, resolution, seconds := ratios[0], tiers[0], durations[0]
	if len(definition.AspectRatios) > 0 {
		ratios, ratio = definition.AspectRatios, agentSupportedValue(definition.AspectRatios, agentVideoRatio(config.Size))
	}
	if len(definition.ResolutionTiers) > 0 {
		tiers, resolution = definition.ResolutionTiers, agentSupportedValue(definition.ResolutionTiers, agentVideoResolution(config.Resolution))
	}
	if len(definition.Durations) > 0 {
		durations, seconds = definition.Durations, agentSupportedValue(definition.Durations, input.Duration)
	}
	// 助手指定的规格必须是模型支持的，不替用户悄悄换成别的规格。
	if seconds != input.Duration {
		supported := make([]string, len(durations))
		for i, value := range durations {
			supported[i] = strconv.Itoa(value)
		}
		return agentMediaSpec{}, safeMessageError{message: fmt.Sprintf("当前视频模型不支持 %d 秒，支持：%s 秒", input.Duration, strings.Join(supported, "、"))}
	}
	if ratio != agentVideoRatio(config.Size) || resolution != agentVideoResolution(config.Resolution) {
		return agentMediaSpec{}, safeMessageError{message: fmt.Sprintf("当前视频模型不支持指定规格，请从 %s / %s 中选择", strings.Join(ratios, "、"), strings.Join(tiers, "、"))}
	}
	if config.GenerateAudio && !definition.SupportsAudioOutput {
		return agentMediaSpec{}, safeMessageError{message: "当前视频模型不支持生成音轨"}
	}
	// 配置节点指定的关键帧单独提交（首尾帧顺序以配置为准），其余图片作为参考图。
	keyframeIDs := []string{}
	if definition.ReferenceMode == "mixed" && config.Node != nil {
		for _, id := range config.Node.KeyframeIDs {
			if slices.Contains(references["image"], id) && !slices.Contains(keyframeIDs, id) {
				keyframeIDs = append(keyframeIDs, id)
			}
		}
	}
	files := []agentMediaFile{}
	for _, id := range references["image"] {
		if !slices.Contains(keyframeIDs, id) {
			files = append(files, agentMediaFile{Field: "input_reference", StorageKey: nodes[id].StorageKey, Limit: agentReferenceImageBytes})
		}
	}
	for _, id := range keyframeIDs {
		files = append(files, agentMediaFile{Field: "keyframes", StorageKey: nodes[id].StorageKey, Limit: agentReferenceImageBytes})
	}
	for _, id := range references["video"] {
		files = append(files, agentMediaFile{Field: "reference_videos", StorageKey: nodes[id].StorageKey, Limit: agentReferenceVideoBytes})
	}
	for _, id := range references["audio"] {
		files = append(files, agentMediaFile{Field: "reference_audios", StorageKey: nodes[id].StorageKey, Limit: agentReferenceAudioBytes})
	}
	fields := map[string]string{
		"model": config.Model, "prompt": prompt, "seconds": strconv.Itoa(seconds), "resolution": resolution,
		"size": agentVideoOutputSize(resolution, ratio), "generate_audio": strconv.FormatBool(config.GenerateAudio),
	}
	return agentMediaSpec{Path: "/videos", Fields: fields, Count: 1, Files: files, Media: agentMediaPresentation{
		Type: "video", NodeIDs: agentGeneratedNodeIDs("video", callID, 1), Prompt: prompt, SourceNodeIDs: sourceIDs,
		Model: config.Model, Size: ratio, Resolution: resolution, Seconds: seconds,
	}}, nil
}

// body 组装请求体：没有参考图的图片生成用 JSON，其余与页面一样用 multipart 连同素材文件一起提交。
func (spec agentMediaSpec) body(ctx context.Context, organizationID string) ([]byte, string, error) {
	if spec.Path == "/images/generations" {
		payload := map[string]any{"n": spec.Count}
		for key, value := range spec.Fields {
			payload[key] = value
		}
		encoded, err := json.Marshal(payload)
		return encoded, "application/json", err
	}
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	for key, value := range spec.Fields {
		_ = writer.WriteField(key, value)
	}
	if spec.Media.Type == "image" {
		_ = writer.WriteField("n", strconv.Itoa(spec.Count))
	}
	total := 0
	for index, file := range spec.Files {
		data, mimeType, err := readOrganizationFile(ctx, organizationID, file.StorageKey, "", file.Limit)
		if err != nil {
			return nil, "", err
		}
		if total += len(data); total > agentReferenceTotalBytes {
			return nil, "", safeMessageError{message: fmt.Sprintf("参考素材总大小不能超过 %d MB", agentReferenceTotalBytes>>20)}
		}
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": file.Field, "filename": fmt.Sprintf("%s-%d%s", file.Field, index+1, assetFileExt(mimeType))}))
		header.Set("Content-Type", mimeType)
		part, err := writer.CreatePart(header)
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

// readOrganizationFile 读取当前企业工作区里的一个文件：本地存储直接读盘，云存储走限时签名地址（可带图片规格）。
func readOrganizationFile(ctx context.Context, organizationID, storageKey, variant string, limit int64) ([]byte, string, error) {
	item, ok, err := repository.GetUserFile(organizationID, strings.TrimSpace(storageKey))
	if err != nil {
		return nil, "", err
	}
	if !ok || strings.TrimSpace(item.ObjectKey) == "" {
		return nil, "", safeMessageError{message: "参考素材不存在或已失效"}
	}
	mimeType := item.MimeType
	var reader io.ReadCloser
	if effectiveStorageDriver(item.StorageDriver) == "local" {
		setting, err := currentUserStorageSetting()
		if err != nil {
			return nil, "", err
		}
		path, err := localStorageObjectPath(setting, item.ObjectKey)
		if err != nil {
			return nil, "", err
		}
		if reader, err = os.Open(path); err != nil {
			return nil, "", safeMessageError{message: "参考素材读取失败"}
		}
	} else {
		fileURL, ok := signedOrganizationFileURL(item, variant, 10*time.Minute)
		if !ok {
			return nil, "", safeMessageError{message: "参考素材地址无效"}
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
		if err != nil {
			return nil, "", err
		}
		response, err := generationTaskArchiveClient.Do(request)
		if err != nil {
			return nil, "", safeMessageError{message: "参考素材读取失败"}
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			_ = response.Body.Close()
			return nil, "", safeMessageError{message: fmt.Sprintf("参考素材下载失败（HTTP %d）", response.StatusCode)}
		}
		reader = response.Body
		if variant != "" {
			mimeType = strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])
		}
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil || len(data) == 0 {
		return nil, "", safeMessageError{message: "参考素材读取失败"}
	}
	if int64(len(data)) > limit {
		return nil, "", safeMessageError{message: fmt.Sprintf("参考素材超过大小限制（最大 %d MB）", limit>>20)}
	}
	if assetTypeFromMime(mimeType) == "" {
		mimeType = strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0])
	}
	return data, mimeType, nil
}

// agentGeneratedNodeIDs 为一次生成预先分配画布节点编号：由调用编号推出，页面离线时也能在之后按同样的编号落地。
func agentGeneratedNodeIDs(nodeType, callID string, count int) []string {
	sum := sha256.Sum256([]byte(callID))
	ids := make([]string, count)
	for i := range ids {
		ids[i] = fmt.Sprintf("%s-%s-%d", nodeType, hex.EncodeToString(sum[:8]), i+1)
	}
	return ids
}

func agentModelDefinition(models []model.ModelDefinition, id string) (model.ModelDefinition, bool) {
	for _, item := range models {
		if item.ID == id {
			return item, true
		}
	}
	return model.ModelDefinition{}, false
}

// agentImageQualitySupported：模型声明了质量档位时以声明为准，未声明时只有 GPT Image 系列支持。
func agentImageQualitySupported(definition model.ModelDefinition, modelName string) bool {
	if definition.QualityLevels != nil {
		return len(definition.QualityLevels) > 0
	}
	return slices.Contains(agentGPTImageQualityModels, strings.ToLower(strings.TrimSpace(modelName)))
}

// agentImageRequestQuality 归一质量档位；auto 等不随请求发送的值返回空。
func agentImageRequestQuality(quality string) string {
	value := strings.ToLower(strings.TrimSpace(quality))
	if alias, ok := agentImageQualityAliases[value]; ok {
		value = alias
	}
	if slices.Contains(agentImageRequestQualities, value) {
		return value
	}
	return ""
}

// agentImageRequestSize 把 auto、比例或像素尺寸换算成请求里的像素尺寸；auto 不随请求发送。
func agentImageRequestSize(size string) (string, error) {
	value := strings.TrimSpace(size)
	if value == "" || strings.EqualFold(value, "auto") {
		return "", nil
	}
	width, height := 0, 0
	if match := agentImageDimensionPattern.FindStringSubmatch(value); match != nil {
		width, _ = strconv.Atoi(match[1])
		height, _ = strconv.Atoi(match[2])
	} else if standard, ok := agentStandardImageSizes[value]; ok {
		return standard, nil
	} else if left, right, found := strings.Cut(value, ":"); found {
		ratioWidth, widthErr := strconv.ParseFloat(strings.TrimSpace(left), 64)
		ratioHeight, heightErr := strconv.ParseFloat(strings.TrimSpace(right), 64)
		if widthErr != nil || heightErr != nil || !finite(ratioWidth) || !finite(ratioHeight) || ratioWidth <= 0 || ratioHeight <= 0 {
			return "", safeMessageError{message: "图像比例必须是正数，例如 9:16"}
		}
		long := int(math.Round(1024*max(ratioWidth, ratioHeight)/min(ratioWidth, ratioHeight)/16)) * 16
		width, height = long, 1024
		if ratioWidth < ratioHeight {
			width, height = 1024, long
		}
	} else {
		return "", safeMessageError{message: "图像尺寸格式不支持，请使用 auto、9:16 或 1024x1024"}
	}
	if width <= 0 || height <= 0 {
		return "", safeMessageError{message: "图像尺寸必须是正整数，例如 1024x1024"}
	}
	if max(width, height) > 3*min(width, height) {
		return "", safeMessageError{message: "图像宽高比不能超过 3:1，请调整尺寸"}
	}
	if width%16 != 0 || height%16 != 0 {
		return "", safeMessageError{message: "图像尺寸的宽高必须是 16 的倍数，请调整尺寸"}
	}
	if max(width, height) > 3840 {
		return "", safeMessageError{message: "图像尺寸最长边不能超过 3840px，请调整尺寸"}
	}
	if pixels := width * height; pixels < 655360 || pixels > 8294400 {
		return "", safeMessageError{message: "图像总像素需在 655360 到 8294400 之间，请调整尺寸"}
	}
	return fmt.Sprintf("%dx%d", width, height), nil
}

// agentVideoRatio 把比例或像素尺寸归一成比例；无法识别时用默认比例。
func agentVideoRatio(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "auto") {
		return "auto"
	}
	if agentVideoRatioPattern.MatchString(value) {
		return value
	}
	match := agentImageDimensionPattern.FindStringSubmatch(value)
	if match == nil {
		return agentDefaultVideoRatios[0]
	}
	width, _ := strconv.Atoi(match[1])
	height, _ := strconv.Atoi(match[2])
	divisor, rest := width, height
	for rest != 0 {
		divisor, rest = rest, divisor%rest
	}
	divisor = max(divisor, 1)
	return fmt.Sprintf("%d:%d", width/divisor, height/divisor)
}

func agentVideoResolution(value string) string {
	value = strings.ToLower(value)
	switch value {
	case "low":
		return "480p"
	case "", "auto", "medium", "high":
		return "720p"
	}
	if agentVideoResolutionPattern.MatchString(value) {
		return value
	}
	return value + "p"
}

// agentVideoOutputSize 按清晰度和比例算出输出像素尺寸，短边取清晰度，宽高取偶数。
func agentVideoOutputSize(resolution, ratio string) string {
	ratio = agentVideoRatio(ratio)
	if ratio == "auto" {
		return "auto"
	}
	short := 2160.0
	if token := agentVideoResolution(resolution); token != "4k" {
		parsed, err := strconv.ParseFloat(strings.TrimSuffix(token, "p"), 64)
		if err != nil || parsed <= 0 {
			parsed = 720
		}
		short = parsed
	}
	left, right, _ := strings.Cut(ratio, ":")
	ratioWidth, _ := strconv.ParseFloat(left, 64)
	ratioHeight, _ := strconv.ParseFloat(right, 64)
	even := func(value float64) int {
		rounded := max(2, int(math.Round(value)))
		return rounded + rounded%2
	}
	if ratioWidth == ratioHeight || ratioWidth <= 0 || ratioHeight <= 0 {
		return fmt.Sprintf("%dx%d", int(short), int(short))
	}
	if ratioWidth > ratioHeight {
		return fmt.Sprintf("%dx%d", even(short*ratioWidth/ratioHeight), even(short))
	}
	return fmt.Sprintf("%dx%d", even(short), even(short*ratioHeight/ratioWidth))
}

// agentSupportedValue 取模型支持的规格：请求值受支持时原样使用，否则回到第一个支持的值。
func agentSupportedValue[T comparable](supported []T, value T) T {
	if slices.Contains(supported, value) {
		return value
	}
	return supported[0]
}
