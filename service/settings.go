package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

var adminModelHTTPClient = &http.Client{Timeout: 30 * time.Second}

type PricingRequest struct {
	Model          string
	Modality       string
	Operation      string
	Unit           string
	ResolutionTier string
	Quality        string
	Size           string
	Resolution     string
	Quantity       int
}

func PublicSettings() (model.PublicSetting, error) {
	settings, err := repository.GetSettings()
	return normalizeSettings(settings).Public, err
}

func AdminSettings() (model.Settings, error) {
	settings, err := repository.GetSettings()
	return hidePrivateAPIKeys(normalizeSettings(settings)), err
}

func SaveSettings(settings model.Settings) (model.Settings, error) {
	saved, err := repository.GetSettings()
	if err != nil {
		return model.Settings{}, err
	}
	settings = normalizeSettings(settings)
	keepPrivateAPIKeys(&settings, normalizeSettings(saved))
	keepPrivateAuthSecrets(&settings, normalizeSettings(saved))
	result, err := repository.SaveSettings(settings, now())
	if err == nil {
		RefreshPromptSyncScheduler()
	}
	return hidePrivateAPIKeys(result), err
}

func AdminChannelModels(index *int, channel model.ModelChannel) ([]string, error) {
	resolved, err := resolveAdminChannel(index, channel)
	if err != nil {
		return nil, err
	}
	return fetchAdminChannelModels(resolved)
}

func AdminTestChannelModel(index *int, channel model.ModelChannel, modelName string) (string, error) {
	resolved, err := resolveAdminChannel(index, channel)
	if err != nil {
		return "", err
	}
	if isArkAgentPlanChannel(resolved) || isSeedanceModelName(modelName) {
		return testArkSeedanceChannelModel(resolved, modelName)
	}
	return testAdminChannelModel(resolved, modelName)
}

func normalizeSettings(settings model.Settings) model.Settings {
	settings.Private = normalizePrivateSetting(settings.Private)
	settings.Public = normalizePublicSettingWithChannels(settings.Public, settings.Private.Channels)
	return settings
}

func normalizePublicSetting(setting model.PublicSetting) model.PublicSetting {
	return normalizePublicSettingWithChannels(setting, nil)
}

func normalizePublicSettingWithChannels(setting model.PublicSetting, channels []model.ModelChannel) model.PublicSetting {
	if setting.ModelChannel.AvailableModels == nil {
		setting.ModelChannel.AvailableModels = []string{}
	}
	setting.ModelChannel.PricingRules = normalizePricingRules(setting.ModelChannel.PricingRules)
	if setting.ModelChannel.AllowCustomChannel == nil {
		enabled := true
		setting.ModelChannel.AllowCustomChannel = &enabled
	}
	if setting.Auth.AllowRegister == nil {
		enabled := true
		setting.Auth.AllowRegister = &enabled
	}
	enabledModels := enabledChannelModels(channels)
	if len(enabledModels) > 0 {
		setting.ModelChannel.AvailableModels = enabledModels
	} else {
		setting.ModelChannel.AvailableModels = uniqueModelNames(setting.ModelChannel.AvailableModels)
	}
	setting.ModelChannel.DefaultTextModel = repairDefaultModel(setting.ModelChannel.DefaultTextModel, setting.ModelChannel.AvailableModels, isTextModelName)
	setting.ModelChannel.DefaultImageModel = repairDefaultModel(setting.ModelChannel.DefaultImageModel, setting.ModelChannel.AvailableModels, isImageModelName)
	setting.ModelChannel.DefaultVideoModel = repairDefaultModel(setting.ModelChannel.DefaultVideoModel, setting.ModelChannel.AvailableModels, isVideoModelName)
	setting.ModelChannel.DefaultModel = repairDefaultModel(setting.ModelChannel.DefaultModel, setting.ModelChannel.AvailableModels, isTextModelName)
	return setting
}

func CalculateRequestCredits(request PricingRequest) (int, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return 0, err
	}
	request = normalizePricingRequest(request)
	if request.Model == "" || request.Modality == "" || request.Operation == "" || request.Unit == "" {
		return 0, safeMessageError{message: "模型计费参数不完整"}
	}
	rule, ok := selectPricingRule(normalizePublicSetting(settings.Public).ModelChannel.PricingRules, request)
	if !ok {
		return 0, safeMessageError{message: "模型未配置计费规则"}
	}
	quantity := request.Quantity
	if quantity < 1 {
		quantity = 1
	}
	credits := rule.Credits * quantity
	if rule.MinCredits > credits {
		credits = rule.MinCredits
	}
	return credits, nil
}

func normalizePricingRules(items []model.PricingRule) []model.PricingRule {
	if items == nil {
		return []model.PricingRule{}
	}
	result := make([]model.PricingRule, 0, len(items))
	for _, item := range items {
		item.Model = strings.TrimSpace(item.Model)
		item.Modality = normalizePricingToken(item.Modality)
		item.Operation = normalizePricingToken(item.Operation)
		item.Unit = normalizePricingToken(item.Unit)
		item.ResolutionTier = normalizeResolutionTier(item.ResolutionTier)
		item.Quality = normalizeQualityTier(item.Quality)
		item.Remark = strings.TrimSpace(item.Remark)
		if item.Credits < 0 {
			item.Credits = 0
		}
		if item.MinCredits < 0 {
			item.MinCredits = 0
		}
		if item.Model == "" {
			continue
		}
		result = append(result, item)
	}
	return result
}

func normalizePricingRequest(request PricingRequest) PricingRequest {
	request.Model = strings.TrimSpace(request.Model)
	request.Modality = normalizePricingToken(request.Modality)
	request.Operation = normalizePricingToken(request.Operation)
	request.Unit = normalizePricingToken(request.Unit)
	request.Quality = normalizeQualityTier(request.Quality)
	request.ResolutionTier = normalizeResolutionTier(request.ResolutionTier)
	if request.ResolutionTier == "" {
		switch request.Modality {
		case "image":
			request.ResolutionTier = normalizeImageResolutionTier(request.Size, request.Quality)
		case "video":
			request.ResolutionTier = normalizeVideoResolutionTier(firstPricingNonEmpty(request.Resolution, request.Size))
		}
	}
	if request.Quantity < 1 {
		request.Quantity = 1
	}
	return request
}

func selectPricingRule(rules []model.PricingRule, request PricingRequest) (model.PricingRule, bool) {
	var selected model.PricingRule
	bestScore := -1
	for _, rule := range rules {
		if !rule.Enabled || rule.Model != request.Model {
			continue
		}
		score, ok := pricingRuleScore(rule, request)
		if !ok || score <= bestScore {
			continue
		}
		bestScore = score
		selected = rule
	}
	return selected, bestScore >= 0
}

func pricingRuleScore(rule model.PricingRule, request PricingRequest) (int, bool) {
	score := 0
	fields := [][2]string{
		{rule.Modality, request.Modality},
		{rule.Operation, request.Operation},
		{rule.Unit, request.Unit},
		{rule.ResolutionTier, request.ResolutionTier},
		{rule.Quality, request.Quality},
	}
	for _, field := range fields {
		if field[0] == "" {
			continue
		}
		if field[0] != field[1] {
			return 0, false
		}
		score++
	}
	return score, true
}

func normalizePricingToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeQualityTier(value string) string {
	value = normalizePricingToken(value)
	switch value {
	case "1k":
		return "low"
	case "2k":
		return "medium"
	case "4k":
		return "high"
	}
	return value
}

func normalizeResolutionTier(value string) string {
	value = normalizePricingToken(value)
	switch value {
	case "low":
		return "1k"
	case "medium":
		return "2k"
	case "high":
		return "4k"
	case "720":
		return "720p"
	case "1080":
		return "1080p"
	case "2160":
		return "4k"
	}
	if strings.HasSuffix(value, "p") || value == "1k" || value == "2k" || value == "4k" {
		return value
	}
	if strings.Contains(value, "4k") {
		return "4k"
	}
	return value
}

func normalizeImageResolutionTier(size string, quality string) string {
	size = strings.ToLower(strings.TrimSpace(size))
	if width, height, ok := parsePricingDimensions(size); ok {
		longest := width
		if height > longest {
			longest = height
		}
		if longest <= 1024 {
			return "1k"
		}
		if longest <= 2048 {
			return "2k"
		}
		return "4k"
	}
	switch quality {
	case "low":
		return "1k"
	case "medium":
		return "2k"
	case "high":
		return "4k"
	}
	return "1k"
}

func normalizeVideoResolutionTier(value string) string {
	value = normalizePricingToken(value)
	if value == "" || value == "auto" || value == "medium" || value == "high" {
		return "720p"
	}
	if value == "low" {
		return "480p"
	}
	if strings.Contains(value, "4k") || strings.Contains(value, "2160") {
		return "4k"
	}
	if strings.Contains(value, "1080") {
		return "1080p"
	}
	if strings.Contains(value, "720") {
		return "720p"
	}
	if strings.Contains(value, "480") {
		return "480p"
	}
	return normalizeResolutionTier(value)
}

func parsePricingDimensions(value string) (int, int, bool) {
	var width, height int
	if _, err := fmt.Sscanf(value, "%dx%d", &width, &height); err != nil {
		return 0, 0, false
	}
	return width, height, width > 0 && height > 0
}

func firstPricingNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizePrivateSetting(setting model.PrivateSetting) model.PrivateSetting {
	if setting.Channels == nil {
		setting.Channels = []model.ModelChannel{}
	}
	setting.PromptSync = normalizePromptSyncSetting(setting.PromptSync)
	for i := range setting.Channels {
		if setting.Channels[i].Protocol == "" {
			setting.Channels[i].Protocol = "openai"
		}
		if setting.Channels[i].Models == nil {
			setting.Channels[i].Models = []string{}
		}
		if setting.Channels[i].Weight <= 0 {
			setting.Channels[i].Weight = 1
		}
	}
	return setting
}

func hidePrivateAPIKeys(settings model.Settings) model.Settings {
	for i := range settings.Private.Channels {
		settings.Private.Channels[i].APIKey = ""
	}
	settings.Private.Auth.LinuxDo.ClientSecret = ""
	return settings
}

func keepPrivateAPIKeys(settings *model.Settings, saved model.Settings) {
	for i := range settings.Private.Channels {
		if strings.TrimSpace(settings.Private.Channels[i].APIKey) != "" {
			continue
		}
		if channel, ok := findSavedChannel(settings.Private.Channels[i], saved.Private.Channels, i); ok {
			settings.Private.Channels[i].APIKey = channel.APIKey
		}
	}
}

func keepPrivateAuthSecrets(settings *model.Settings, saved model.Settings) {
	if strings.TrimSpace(settings.Private.Auth.LinuxDo.ClientSecret) == "" {
		settings.Private.Auth.LinuxDo.ClientSecret = saved.Private.Auth.LinuxDo.ClientSecret
	}
}

func findSavedChannel(channel model.ModelChannel, saved []model.ModelChannel, index int) (model.ModelChannel, bool) {
	for _, item := range saved {
		if item.Name == channel.Name && item.BaseURL == channel.BaseURL {
			return item, true
		}
	}
	if index < len(saved) {
		return saved[index], true
	}
	return model.ModelChannel{}, false
}

func SelectModelChannel(modelName string) (model.ModelChannel, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return model.ModelChannel{}, err
	}
	channels := modelChannelsForModel(normalizePrivateSetting(settings.Private).Channels, modelName)
	if len(channels) == 0 {
		return model.ModelChannel{}, errors.New("没有可用模型渠道")
	}
	total := 0
	for _, channel := range channels {
		total += channel.Weight
	}
	hit := rand.Intn(total)
	for _, channel := range channels {
		hit -= channel.Weight
		if hit < 0 {
			return channel, nil
		}
	}
	return channels[0], nil
}

func BuildModelChannelURL(channel model.ModelChannel, path string) string {
	baseURL := normalizeModelChannelBaseURL(channel.BaseURL)
	lowerBaseURL := strings.ToLower(baseURL)
	if !strings.HasSuffix(lowerBaseURL, "/v1") && !strings.HasSuffix(lowerBaseURL, "/api/v3") && !strings.HasSuffix(lowerBaseURL, "/api/plan/v3") {
		baseURL += "/v1"
	}
	return baseURL + path
}

func normalizeModelChannelBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		path := strings.TrimRight(parsed.Path, "/")
		lowerPath := strings.ToLower(path)
		if index := strings.Index(lowerPath, "/api/plan/v3"); index >= 0 {
			end := index + len("/api/plan/v3")
			if len(lowerPath) == end || lowerPath[end] == '/' {
				parsed.Path = path[:end]
				parsed.RawPath = ""
				parsed.RawQuery = ""
				parsed.Fragment = ""
				return strings.TrimRight(parsed.String(), "/")
			}
		}
	}
	return baseURL
}

func isArkAgentPlanChannel(channel model.ModelChannel) bool {
	baseURL := strings.ToLower(normalizeModelChannelBaseURL(channel.BaseURL))
	return strings.HasSuffix(baseURL, "/api/plan/v3")
}

func isSeedanceModelName(modelName string) bool {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	return strings.Contains(modelName, "seedance") || strings.Contains(modelName, "doubao-seedance")
}

func enabledChannelModels(channels []model.ModelChannel) []string {
	models := []string{}
	for _, channel := range channels {
		if !channel.Enabled {
			continue
		}
		models = append(models, channel.Models...)
	}
	return uniqueModelNames(models)
}

func uniqueModelNames(models []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, item := range models {
		name := strings.TrimSpace(item)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		result = append(result, name)
	}
	return result
}

func repairDefaultModel(current string, models []string, preferred func(string) bool) string {
	current = strings.TrimSpace(current)
	for _, item := range models {
		if item == current {
			return current
		}
	}
	for _, item := range models {
		if preferred(item) {
			return item
		}
	}
	if len(models) > 0 {
		return models[0]
	}
	return ""
}

func isVideoModelName(modelName string) bool {
	name := strings.ToLower(strings.TrimSpace(modelName))
	return strings.Contains(name, "seedance") || strings.Contains(name, "video")
}

func isImageModelName(modelName string) bool {
	name := strings.ToLower(strings.TrimSpace(modelName))
	return strings.Contains(name, "seedream") || strings.Contains(name, "gpt-image") || strings.Contains(name, "image")
}

func isTextModelName(modelName string) bool {
	return !isImageModelName(modelName) && !isVideoModelName(modelName)
}

func normalizeModelChannel(channel model.ModelChannel) model.ModelChannel {
	if channel.Protocol == "" {
		channel.Protocol = "openai"
	}
	if channel.Models == nil {
		channel.Models = []string{}
	}
	if channel.Weight <= 0 {
		channel.Weight = 1
	}
	return channel
}

func resolveAdminChannel(index *int, channel model.ModelChannel) (model.ModelChannel, error) {
	resolved := normalizeModelChannel(channel)
	if strings.TrimSpace(resolved.APIKey) == "" {
		settings, err := repository.GetSettings()
		if err != nil {
			return model.ModelChannel{}, err
		}
		saved := normalizePrivateSetting(settings.Private).Channels
		if index != nil && *index >= 0 && *index < len(saved) {
			if resolved.APIKey == "" {
				resolved.APIKey = saved[*index].APIKey
			}
			if resolved.BaseURL == "" {
				resolved.BaseURL = saved[*index].BaseURL
			}
			if resolved.Name == "" {
				resolved.Name = saved[*index].Name
			}
		}
		if resolved.APIKey == "" {
			if savedChannel, ok := findSavedChannel(resolved, saved, -1); ok {
				resolved.APIKey = savedChannel.APIKey
			}
		}
	}
	if strings.TrimSpace(resolved.BaseURL) == "" {
		return model.ModelChannel{}, safeMessageError{message: "缺少接口地址"}
	}
	if strings.TrimSpace(resolved.APIKey) == "" {
		return model.ModelChannel{}, safeMessageError{message: "缺少 API Key"}
	}
	return resolved, nil
}

func fetchAdminChannelModels(channel model.ModelChannel) ([]string, error) {
	request, err := http.NewRequest(http.MethodGet, BuildModelChannelURL(channel, "/models"), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	response, err := adminModelHTTPClient.Do(request)
	if err != nil {
		return nil, safeMessageError{message: "读取模型失败：上游接口无响应或网络不可达"}
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode >= http.StatusBadRequest {
		if response.StatusCode == http.StatusNotFound && isArkAgentPlanChannel(channel) {
			return nil, safeMessageError{message: "火山方舟 Agent Plan 未提供 OpenAI /models 模型列表接口，请手动填写模型名称，例如 doubao-seedance-2.0。"}
		}
		return nil, readAdminChannelError(body, response.StatusCode, "读取模型失败")
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &payload)
	result := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		if strings.TrimSpace(item.ID) != "" {
			result = append(result, item.ID)
		}
	}
	sort.Strings(result)
	return result, nil
}

func testAdminChannelModel(channel model.ModelChannel, modelName string) (string, error) {
	if strings.TrimSpace(modelName) == "" {
		return "", errors.New("缺少模型名称")
	}
	body, _ := json.Marshal(map[string]any{
		"model": modelName,
		"messages": []map[string]string{{
			"role":    "user",
			"content": "hi",
		}},
	})
	request, err := http.NewRequest(http.MethodPost, BuildModelChannelURL(channel, "/chat/completions"), strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := adminModelHTTPClient.Do(request)
	if err != nil {
		return "", safeMessageError{message: "测试失败：上游接口无响应或网络不可达"}
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(response.Body)
	if response.StatusCode >= http.StatusBadRequest {
		return "", readAdminChannelError(responseBody, response.StatusCode, "测试失败")
	}
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	_ = json.Unmarshal(responseBody, &payload)
	if len(payload.Choices) > 0 && strings.TrimSpace(payload.Choices[0].Message.Content) != "" {
		return payload.Choices[0].Message.Content, nil
	}
	return "ok", nil
}

func testArkSeedanceChannelModel(channel model.ModelChannel, modelName string) (string, error) {
	if strings.TrimSpace(modelName) == "" {
		return "", errors.New("缺少模型名称")
	}
	if strings.TrimSpace(channel.BaseURL) == "" {
		return "", safeMessageError{message: "缺少接口地址"}
	}
	if strings.TrimSpace(channel.APIKey) == "" {
		return "", safeMessageError{message: "缺少 API Key"}
	}
	if !isArkAgentPlanChannel(channel) {
		return "Seedance 视频模型不会发送 /chat/completions 文本测试。已检查 Base URL、API Key 和模型名非空；未调用视频生成接口，因此未验证套餐额度或模型权限。", nil
	}
	return "Agent Plan / Seedance 视频模型配置格式已通过。后台测试不会调用视频生成接口，因此未验证 API Key、套餐额度或模型权限；请在画布中使用视频生成验证。", nil
}

func readAdminChannelError(body []byte, statusCode int, fallback string) error {
	var payload struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Msg string `json:"msg"`
	}
	if len(body) > 0 && json.Unmarshal(body, &payload) == nil {
		if payload.Error != nil && strings.TrimSpace(payload.Error.Message) != "" {
			return safeMessageError{message: payload.Error.Message}
		}
		if strings.TrimSpace(payload.Msg) != "" {
			return safeMessageError{message: payload.Msg}
		}
	}
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		return safeMessageError{message: fmt.Sprintf("上游接口鉴权失败（%d），请检查 API Key、套餐权限或模型权限", statusCode)}
	}
	if statusCode == http.StatusTooManyRequests {
		return safeMessageError{message: "上游接口限流或额度不足（429），请稍后重试或检查额度"}
	}
	if statusCode > 0 {
		return safeMessageError{message: fmt.Sprintf("%s：%d", fallback, statusCode)}
	}
	return safeMessageError{message: fallback}
}

type safeMessageError struct {
	message string
}

func (err safeMessageError) Error() string {
	return err.message
}

func (err safeMessageError) SafeMessage() string {
	return err.message
}

func modelChannelsForModel(channels []model.ModelChannel, modelName string) []model.ModelChannel {
	result := []model.ModelChannel{}
	for _, channel := range channels {
		if !channel.Enabled || channel.BaseURL == "" || channel.APIKey == "" {
			continue
		}
		for _, item := range channel.Models {
			if strings.TrimSpace(item) == modelName {
				result = append(result, channel)
				break
			}
		}
	}
	return result
}
