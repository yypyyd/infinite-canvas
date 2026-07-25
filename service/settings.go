package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
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
	Size           string
	Resolution     string
	Quantity       int
}

type ModelChannelSelection struct {
	Channel model.ModelChannel
	Model   model.ChannelModel
}

func PublicSettings() (model.PublicSetting, error) {
	settings, err := repository.GetSettings()
	public := normalizeSettings(settings).Public
	public.Announcements.Items = publishedAnnouncements(public.Announcements)
	return public, err
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
	emailDomainRestriction := settings.Public.Auth.EmailDomainRestriction
	settings = normalizeSettings(settings)
	if emailDomainRestriction && len(settings.Public.Auth.EmailDomains) == 0 {
		return model.Settings{}, safeMessageError{message: "启用邮箱域名限制前请至少填写一个有效域名"}
	}
	keepPrivateAPIKeys(&settings, normalizeSettings(saved))
	if err := validateEmailSetting(settings.Private.Email); err != nil {
		return model.Settings{}, err
	}
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
	if channelModel, ok := findChannelModel(resolved.Models, modelName); ok {
		modelName = channelModel.UpstreamModel
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
	enabledModels := enabledChannelModels(channels)
	setting.ModelChannel.Models = normalizeModelDefinitions(setting.ModelChannel.Models, setting.ModelChannel.AvailableModels, setting.ModelChannel.ModelAspectRatios, enabledModels)
	setting.ModelChannel.PricingRules = appendDefaultPricingRulesForModels(normalizePricingRules(setting.ModelChannel.PricingRules), setting.ModelChannel.Models)
	setting.ModelChannel.GroupRatios = normalizeGroupRatios(setting.ModelChannel.GroupRatios)
	setting.ModelChannel.ModelAspectRatios = normalizeModelAspectRatios(modelAspectRatiosFromDefinitions(setting.ModelChannel.Models, setting.ModelChannel.ModelAspectRatios))
	if setting.Auth.AllowRegister == nil {
		enabled := true
		setting.Auth.AllowRegister = &enabled
	}
	setting.Auth.EmailVerification = true
	setting.Auth.EmailDomains = normalizeStringList(setting.Auth.EmailDomains, normalizeEmailDomain)
	if len(setting.Auth.EmailDomains) == 0 {
		setting.Auth.EmailDomainRestriction = false
	}
	if setting.Auth.NewUserRewardCredits < 0 {
		setting.Auth.NewUserRewardCredits = 0
	}
	setting.Announcements = normalizeAnnouncementSetting(setting.Announcements)
	if setting.CheckIn.RewardCredits < 0 {
		setting.CheckIn.RewardCredits = 0
	}
	managedModels := enabledManagedModelIDs(setting.ModelChannel.Models)
	if len(managedModels) > 0 {
		setting.ModelChannel.AvailableModels = managedModels
	} else if len(enabledModels) > 0 {
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

func normalizeAnnouncementSetting(setting model.AnnouncementSetting) model.AnnouncementSetting {
	if setting.Items == nil {
		setting.Items = []model.Announcement{}
	}
	result := make([]model.Announcement, 0, len(setting.Items))
	seen := map[int]bool{}
	for index, item := range setting.Items {
		item.Title = strings.TrimSpace(item.Title)
		item.Content = strings.TrimSpace(item.Content)
		item.Type = normalizePricingToken(item.Type)
		item.PublishAt = strings.TrimSpace(item.PublishAt)
		if item.ID <= 0 {
			item.ID = index + 1
		}
		if seen[item.ID] || item.Title == "" || item.Content == "" {
			continue
		}
		seen[item.ID] = true
		switch item.Type {
		case "success", "warning", "error":
		default:
			item.Type = "info"
		}
		result = append(result, item)
	}
	setting.Items = result
	return setting
}

func publishedAnnouncements(setting model.AnnouncementSetting) []model.Announcement {
	if !setting.Enabled {
		return []model.Announcement{}
	}
	current := time.Now()
	result := make([]model.Announcement, 0, len(setting.Items))
	for _, item := range setting.Items {
		if !item.Enabled {
			continue
		}
		if publishAt, err := time.Parse(time.RFC3339, item.PublishAt); err == nil && publishAt.After(current) {
			continue
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].PublishAt > result[j].PublishAt })
	return result
}

func CalculateRequestCredits(request PricingRequest) (int, error) {
	return CalculateRequestCreditsForGroup(request, "default")
}

func CalculateRequestCreditsForGroup(request PricingRequest, userGroup string) (int, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return 0, err
	}
	request = normalizePricingRequest(request)
	if request.Model == "" || request.Modality == "" || request.Operation == "" || request.Unit == "" {
		return 0, safeMessageError{message: "模型计费参数不完整"}
	}
	public := normalizePublicSetting(settings.Public)
	rule, ok := selectPricingRule(public.ModelChannel.PricingRules, request)
	if !ok {
		return 0, safeMessageError{message: "模型未配置计费规则"}
	}
	quantity := request.Quantity
	if quantity < 1 {
		quantity = 1
	}
	credits := calculateRuleCredits(rule, quantity, groupRatio(public.ModelChannel.GroupRatios, userGroup))
	if rule.MinCredits > credits {
		credits = rule.MinCredits
	}
	return credits, nil
}

func calculateRuleCredits(rule model.PricingRule, quantity int, ratio float64) int {
	if quantity < 1 {
		quantity = 1
	}
	if ratio <= 0 {
		ratio = 1
	}
	if rule.BillingMode == "ratio" {
		modelRatio := rule.ModelRatio
		if modelRatio <= 0 {
			modelRatio = 1
		}
		return int(math.Ceil(float64(quantity) * modelRatio * ratio))
	}
	return int(math.Ceil(float64(rule.Credits*quantity) * ratio))
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
		item.BillingMode = normalizePricingToken(item.BillingMode)
		if item.BillingMode != "ratio" {
			item.BillingMode = "fixed"
		}
		item.Remark = strings.TrimSpace(item.Remark)
		if item.Credits < 0 {
			item.Credits = 0
		}
		if item.MinCredits < 0 {
			item.MinCredits = 0
		}
		if item.ModelRatio <= 0 {
			item.ModelRatio = 1
		}
		if item.CompletionRatio <= 0 {
			item.CompletionRatio = 1
		}
		if item.Model == "" {
			continue
		}
		result = append(result, item)
	}
	return result
}

func appendDefaultPricingRulesForModels(rules []model.PricingRule, models []model.ModelDefinition) []model.PricingRule {
	result := append([]model.PricingRule{}, rules...)
	for _, item := range models {
		if !item.Enabled || strings.TrimSpace(item.ID) == "" {
			continue
		}
		for _, rule := range defaultPricingRulesForModel(item) {
			if hasPricingFallbackRule(result, rule) {
				continue
			}
			result = append(result, rule)
		}
	}
	return result
}

func defaultPricingRulesForModel(item model.ModelDefinition) []model.PricingRule {
	modelID := strings.TrimSpace(item.ID)
	modality := normalizePricingToken(item.Modality)
	base := model.PricingRule{
		Model:           modelID,
		BillingMode:     "fixed",
		Credits:         1,
		MinCredits:      0,
		ModelRatio:      1,
		CompletionRatio: 1,
		Enabled:         true,
		Remark:          "auto default",
	}
	unit := "request"
	if modality == "image" {
		unit = "image"
	} else if modality == "video" {
		unit = "second"
	}
	operations := normalizeModelOperations(item.Operations, modelID, modality)
	rules := make([]model.PricingRule, 0, len(operations))
	for _, operation := range operations {
		rules = append(rules, defaultPricingRule(base, modality, operation, unit))
	}
	return rules
}

func defaultPricingRule(base model.PricingRule, modality string, operation string, unit string) model.PricingRule {
	base.Modality = modality
	base.Operation = operation
	base.Unit = unit
	return base
}

func hasPricingFallbackRule(rules []model.PricingRule, target model.PricingRule) bool {
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if rule.Model == target.Model &&
			rule.Modality == target.Modality &&
			rule.Operation == target.Operation &&
			rule.Unit == target.Unit &&
			rule.ResolutionTier == "" {
			return true
		}
	}
	return false
}

func normalizeModelDefinitions(items []model.ModelDefinition, availableModels []string, aspectRatios map[string][]string, channelModels []string) []model.ModelDefinition {
	seedModels := uniqueModelNames(channelModels)
	if len(seedModels) == 0 {
		seedModels = uniqueModelNames(availableModels)
	}
	if len(items) == 0 {
		items = make([]model.ModelDefinition, 0, len(seedModels))
		for index, modelName := range seedModels {
			modality := defaultModelModality(modelName)
			items = append(items, model.ModelDefinition{ID: modelName, Name: modelName, Modality: modality, Enabled: true, Sort: index, AspectRatios: aspectRatios[modelName], ResolutionTiers: defaultModelResolutionTiers(modality)})
		}
	}
	result := make([]model.ModelDefinition, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" || seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		item.Name = strings.TrimSpace(item.Name)
		if item.Name == "" {
			item.Name = item.ID
		}
		item.Modality = normalizePricingToken(item.Modality)
		if item.Modality == "" {
			item.Modality = defaultModelModality(item.ID)
		}
		item.Operations = normalizeModelOperations(item.Operations, item.ID, item.Modality)
		item.AspectRatios = normalizeStringList(item.AspectRatios, normalizePricingToken)
		item.ResolutionTiers = normalizeStringList(item.ResolutionTiers, normalizeResolutionTier)
		if item.Modality != "image" && item.Modality != "video" {
			item.AspectRatios = []string{}
			item.ResolutionTiers = []string{}
		}
		item.Remark = strings.TrimSpace(item.Remark)
		result = append(result, item)
	}
	for _, modelName := range seedModels {
		if seen[modelName] {
			continue
		}
		seen[modelName] = true
		modality := defaultModelModality(modelName)
		result = append(result, model.ModelDefinition{
			ID:              modelName,
			Name:            modelName,
			Modality:        modality,
			Operations:      defaultModelOperations(modelName, modality),
			Enabled:         true,
			Sort:            len(result),
			AspectRatios:    normalizeStringList(aspectRatios[modelName], normalizePricingToken),
			ResolutionTiers: defaultModelResolutionTiers(modality),
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Sort == result[j].Sort {
			return result[i].ID < result[j].ID
		}
		return result[i].Sort < result[j].Sort
	})
	return result
}

func defaultModelResolutionTiers(modality string) []string {
	switch normalizePricingToken(modality) {
	case "image":
		return []string{"1k"}
	case "video":
		return []string{"720p"}
	}
	return []string{}
}

func normalizeModelOperations(items []string, modelName string, modality string) []string {
	allowed := map[string]bool{}
	switch normalizePricingToken(modality) {
	case "image":
		allowed["generation"] = true
		allowed["edit"] = true
	case "video":
		allowed["generation"] = true
	case "audio":
		allowed["speech"] = true
	default:
		allowed["completion"] = true
	}
	result := []string{}
	seen := map[string]bool{}
	for _, operation := range items {
		operation = normalizePricingToken(operation)
		if allowed[operation] && !seen[operation] {
			seen[operation] = true
			result = append(result, operation)
		}
	}
	if len(result) == 0 {
		return defaultModelOperations(modelName, modality)
	}
	return result
}

func defaultModelOperations(modelName string, modality string) []string {
	modality = normalizePricingToken(modality)
	if modality == "video" {
		return []string{"generation"}
	}
	if modality == "audio" {
		return []string{"speech"}
	}
	if modality != "image" {
		return []string{"completion"}
	}
	name := strings.ToLower(strings.TrimSpace(modelName))
	editOnly := []string{"qwen-image-edit", "image-edit", "image_edit", "inpaint", "outpaint", "remove-background", "flux-pro-1.0-fill", "flux-pro-1.0-expand"}
	for _, pattern := range editOnly {
		if strings.Contains(name, pattern) {
			return []string{"edit"}
		}
	}
	editable := []string{"gpt-image", "dall-e-2", "flux-kontext", "flux-klein", "seedream", "nano-banana", "firefly-image-5"}
	for _, pattern := range editable {
		if strings.Contains(name, pattern) {
			return []string{"generation", "edit"}
		}
	}
	return []string{"generation"}
}

func normalizeGroupRatios(items map[string]float64) map[string]float64 {
	result := map[string]float64{"default": 1}
	for key, value := range items {
		key = normalizePricingToken(key)
		if key == "" || value <= 0 {
			continue
		}
		result[key] = value
	}
	return result
}

func groupRatio(items map[string]float64, userGroup string) float64 {
	userGroup = normalizePricingToken(userGroup)
	if userGroup == "" {
		userGroup = "default"
	}
	if value := items[userGroup]; value > 0 {
		return value
	}
	if value := items["default"]; value > 0 {
		return value
	}
	return 1
}

func enabledManagedModelIDs(items []model.ModelDefinition) []string {
	result := []string{}
	for _, item := range items {
		if item.Enabled && strings.TrimSpace(item.ID) != "" {
			result = append(result, item.ID)
		}
	}
	return uniqueModelNames(result)
}

func modelAspectRatiosFromDefinitions(items []model.ModelDefinition, fallback map[string][]string) map[string][]string {
	result := map[string][]string{}
	for key, value := range fallback {
		result[key] = value
	}
	for _, item := range items {
		if item.ID != "" && len(item.AspectRatios) > 0 {
			result[item.ID] = item.AspectRatios
		}
	}
	return result
}

func normalizeStringList(items []string, normalize func(string) string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, item := range items {
		item = normalize(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

func normalizePricingRequest(request PricingRequest) PricingRequest {
	request.Model = strings.TrimSpace(request.Model)
	request.Modality = normalizePricingToken(request.Modality)
	request.Operation = normalizePricingToken(request.Operation)
	request.Unit = normalizePricingToken(request.Unit)
	request.ResolutionTier = normalizeResolutionTier(request.ResolutionTier)
	if request.ResolutionTier == "" {
		switch request.Modality {
		case "image":
			request.ResolutionTier = normalizeImageResolutionTier(request.Size)
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

func normalizeModelAspectRatios(items map[string][]string) map[string][]string {
	if items == nil {
		return map[string][]string{}
	}
	result := map[string][]string{}
	for modelName, ratios := range items {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		seen := map[string]bool{}
		for _, ratio := range ratios {
			ratio = normalizePricingToken(ratio)
			if ratio == "" || seen[ratio] {
				continue
			}
			seen[ratio] = true
			result[modelName] = append(result[modelName], ratio)
		}
	}
	return result
}

func normalizePricingToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
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

func normalizeImageResolutionTier(size string) string {
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
	setting.Email = normalizeEmailSetting(setting.Email)
	setting.OperationsAlerts = normalizeOperationsAlertSetting(setting.OperationsAlerts)
	for i := range setting.Channels {
		setting.Channels[i] = normalizeModelChannel(setting.Channels[i])
	}
	return setting
}

func normalizeEmailSetting(setting model.EmailSetting) model.EmailSetting {
	setting.SMTPHost = strings.TrimSpace(setting.SMTPHost)
	setting.SMTPUsername = strings.TrimSpace(setting.SMTPUsername)
	setting.SMTPFromEmail = strings.ToLower(strings.TrimSpace(setting.SMTPFromEmail))
	setting.SMTPFromName = strings.TrimSpace(setting.SMTPFromName)
	setting.SMTPSecurity = strings.ToLower(strings.TrimSpace(setting.SMTPSecurity))
	if setting.SMTPPort <= 0 || setting.SMTPPort > 65535 {
		setting.SMTPPort = 587
	}
	if setting.SMTPSecurity == "" && setting.SMTPPort == 465 {
		setting.SMTPSecurity = "ssl"
	}
	if setting.SMTPSecurity != "ssl" && setting.SMTPSecurity != "none" {
		setting.SMTPSecurity = "starttls"
	}
	return setting
}

func validateEmailSetting(setting model.EmailSetting) error {
	if setting.SMTPHost == "" && setting.SMTPUsername == "" && setting.SMTPPassword == "" && setting.SMTPFromEmail == "" {
		return nil
	}
	if setting.SMTPHost == "" || setting.SMTPFromEmail == "" {
		return safeMessageError{message: "请完整填写 SMTP 服务器和发件邮箱"}
	}
	if _, err := normalizeEmailAddress(setting.SMTPFromEmail); err != nil {
		return safeMessageError{message: "请输入有效的 SMTP 发件邮箱"}
	}
	if (setting.SMTPUsername == "") != (setting.SMTPPassword == "") {
		return safeMessageError{message: "SMTP 账号和密码必须同时配置"}
	}
	if setting.SMTPSecurity == "none" && setting.SMTPUsername != "" {
		return safeMessageError{message: "SMTP 账号密码必须使用 STARTTLS 或 SSL/TLS 加密传输"}
	}
	return nil
}

func normalizeEmailDomain(value string) string {
	value = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "@")))
	if len(value) == 0 || len(value) > 253 || strings.ContainsAny(value, "@ /\\") {
		return ""
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return ""
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return ""
			}
		}
	}
	return value
}

func hidePrivateAPIKeys(settings model.Settings) model.Settings {
	for i := range settings.Private.Channels {
		settings.Private.Channels[i].APIKey = ""
	}
	settings.Private.Email.PasswordConfigured = settings.Private.Email.SMTPPassword != ""
	settings.Private.Email.SMTPPassword = ""
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
	if settings.Private.Email.SMTPPassword == "" {
		settings.Private.Email.SMTPPassword = saved.Private.Email.SMTPPassword
	}
	settings.Private.Email.PasswordConfigured = false
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

func SelectModelChannel(request PricingRequest) (ModelChannelSelection, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return ModelChannelSelection{}, err
	}
	request = normalizePricingRequest(request)
	selections := modelChannelSelectionsForRequest(normalizePrivateSetting(settings.Private).Channels, request)
	if len(selections) == 0 {
		return ModelChannelSelection{}, safeMessageError{message: fmt.Sprintf("模型 %s 没有支持当前操作或分辨率的可用渠道", request.Model)}
	}
	total := 0
	for _, selection := range selections {
		total += selection.Channel.Weight
	}
	hit := rand.Intn(total)
	for _, selection := range selections {
		hit -= selection.Channel.Weight
		if hit < 0 {
			return selection, nil
		}
	}
	return selections[0], nil
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
		for _, item := range channel.Models {
			models = append(models, item.Model)
		}
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
	return strings.Contains(name, "seedance") || strings.Contains(name, "video") || strings.Contains(name, "sora") || strings.Contains(name, "veo") || strings.Contains(name, "kling") || strings.Contains(name, "wan") || strings.Contains(name, "firefly-ray")
}

func normalizeOperationsAlertSetting(setting model.OperationsAlertSetting) model.OperationsAlertSetting {
	if setting.Enabled == nil {
		enabled := true
		setting.Enabled = &enabled
	}
	setting.BatchQueuedThreshold = normalizeOperationsAlertThreshold(setting.BatchQueuedThreshold, 100)
	setting.BatchExpiredLeasesThreshold = normalizeOperationsAlertThreshold(setting.BatchExpiredLeasesThreshold, 1)
	setting.EmailPendingThreshold = normalizeOperationsAlertThreshold(setting.EmailPendingThreshold, 50)
	setting.EmailFailedThreshold = normalizeOperationsAlertThreshold(setting.EmailFailedThreshold, 1)
	setting.EmailExpiredLeasesThreshold = normalizeOperationsAlertThreshold(setting.EmailExpiredLeasesThreshold, 1)
	setting.ObjectDeletionPendingThreshold = normalizeOperationsAlertThreshold(setting.ObjectDeletionPendingThreshold, 100)
	setting.ObjectDeletionFailedThreshold = normalizeOperationsAlertThreshold(setting.ObjectDeletionFailedThreshold, 1)
	setting.ObjectDeletionExpiredLeasesThreshold = normalizeOperationsAlertThreshold(setting.ObjectDeletionExpiredLeasesThreshold, 1)
	return setting
}

func normalizeOperationsAlertThreshold(value *int64, fallback int64) *int64 {
	if value == nil {
		value = &fallback
	}
	if *value < 0 {
		zero := int64(0)
		return &zero
	}
	return value
}

func isImageModelName(modelName string) bool {
	name := strings.ToLower(strings.TrimSpace(modelName))
	return strings.Contains(name, "seedream") || strings.Contains(name, "gpt-image") || strings.Contains(name, "image") || strings.Contains(name, "dall-e") || strings.Contains(name, "imagen") || strings.Contains(name, "imagine-") || strings.Contains(name, "flux") || strings.Contains(name, "nano-banana")
}

func isAudioModelName(modelName string) bool {
	name := strings.ToLower(strings.TrimSpace(modelName))
	return strings.Contains(name, "audio") || strings.Contains(name, "speech") || strings.Contains(name, "tts")
}

func isTextModelName(modelName string) bool {
	return !isImageModelName(modelName) && !isVideoModelName(modelName) && !isAudioModelName(modelName)
}

func defaultModelModality(modelName string) string {
	if isVideoModelName(modelName) {
		return "video"
	}
	if isImageModelName(modelName) {
		return "image"
	}
	if isAudioModelName(modelName) {
		return "audio"
	}
	return "text"
}

func normalizeModelChannel(channel model.ModelChannel) model.ModelChannel {
	if channel.Protocol == "" {
		channel.Protocol = "openai"
	}
	if channel.Models == nil {
		channel.Models = []model.ChannelModel{}
	}
	models := make([]model.ChannelModel, 0, len(channel.Models))
	seen := map[string]bool{}
	for _, item := range channel.Models {
		item.Model = strings.TrimSpace(item.Model)
		if item.Model == "" || seen[item.Model] {
			continue
		}
		seen[item.Model] = true
		item.UpstreamModel = strings.TrimSpace(item.UpstreamModel)
		if item.UpstreamModel == "" {
			item.UpstreamModel = item.Model
		}
		item.Operations = normalizeStringList(item.Operations, normalizePricingToken)
		item.ResolutionTiers = normalizeStringList(item.ResolutionTiers, normalizeResolutionTier)
		models = append(models, item)
	}
	channel.Models = models
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

func modelChannelSelectionsForRequest(channels []model.ModelChannel, request PricingRequest) []ModelChannelSelection {
	result := []ModelChannelSelection{}
	for _, channel := range channels {
		if !channel.Enabled || channel.BaseURL == "" || channel.APIKey == "" {
			continue
		}
		for _, item := range channel.Models {
			if channelModelSupportsRequest(item, request) {
				result = append(result, ModelChannelSelection{Channel: channel, Model: item})
				break
			}
		}
	}
	return result
}

func channelModelSupportsRequest(item model.ChannelModel, request PricingRequest) bool {
	if item.Model != request.Model {
		return false
	}
	if request.Operation != "" && !containsPricingValue(item.Operations, request.Operation) {
		return false
	}
	return request.ResolutionTier == "" || containsPricingValue(item.ResolutionTiers, request.ResolutionTier)
}

func containsPricingValue(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func findChannelModel(items []model.ChannelModel, modelName string) (model.ChannelModel, bool) {
	modelName = strings.TrimSpace(modelName)
	for _, item := range items {
		if item.Model == modelName {
			return item, true
		}
	}
	return model.ChannelModel{}, false
}
