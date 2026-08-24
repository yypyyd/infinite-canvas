package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

var adminModelHTTPClient = &http.Client{Timeout: 30 * time.Second}

const modelChannelProtocolAutoDLComfyUI = "autodl_comfyui"

var comfyUIWorkflowIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type PricingRequest struct {
	Model           string
	Modality        string
	Operation       string
	Unit            string
	ResolutionTier  string
	AspectRatio     string
	Size            string
	Resolution      string
	Quantity        int
	ReferenceImages int
	ReferenceVideos int
	ReferenceAudios int
	ReferenceMedia  int
	GenerateAudio   bool
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

func PublicAPIModels() ([]model.PublicAPIModel, error) {
	settings, err := PublicSettings()
	return publicAPIModels(settings.ModelChannel), err
}

func publicAPIModels(setting model.PublicModelChannelSetting) []model.PublicAPIModel {
	available := make(map[string]bool, len(setting.AvailableModels))
	for _, modelID := range setting.AvailableModels {
		available[modelID] = true
	}
	result := make([]model.PublicAPIModel, 0, len(setting.Models))
	for _, item := range setting.Models {
		if !item.Enabled || !available[item.ID] || item.Modality == "text" {
			continue
		}
		result = append(result, model.PublicAPIModel{
			ID:                  item.ID,
			Object:              "model",
			OwnedBy:             "infinite-canvas",
			Name:                item.Name,
			Modality:            item.Modality,
			Operations:          item.Operations,
			AspectRatios:        item.AspectRatios,
			ResolutionTiers:     item.ResolutionTiers,
			Durations:           item.Durations,
			MaxReferenceImages:  item.MaxReferenceImages,
			MaxReferenceVideos:  item.MaxReferenceVideos,
			MaxReferenceAudios:  item.MaxReferenceAudios,
			MaxReferenceMedia:   item.MaxReferenceMedia,
			SupportsAudioOutput: item.SupportsAudioOutput,
			ReferenceMode:       item.ReferenceMode,
		})
	}
	return result
}

func AdminSettings() (model.Settings, error) {
	settings, err := repository.GetSettings()
	return hidePrivateAPIKeys(normalizeAdminSettings(settings)), err
}

func normalizeAdminSettings(settings model.Settings) model.Settings {
	settings = normalizeSettings(settings)
	if validatePaymentSetting(settings.Private.Payment) != nil {
		settings.Private.Payment.Enabled = false
	}
	return settings
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
	if err := validateModelChannels(settings.Private.Channels); err != nil {
		return model.Settings{}, err
	}
	if err := validateEmailSetting(settings.Private.Email); err != nil {
		return model.Settings{}, err
	}
	if err := validateStorageSetting(settings.Private.Storage); err != nil {
		return model.Settings{}, err
	}
	if err := validatePaymentSetting(settings.Private.Payment); err != nil {
		return model.Settings{}, err
	}
	result, err := repository.SaveSettings(settings, now())
	if err == nil {
		RefreshPromptSyncScheduler()
	}
	return hidePrivateAPIKeys(result), err
}

func AdminChannelModels(index *int, channel model.ModelChannel) ([]model.DiscoveredModel, error) {
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
	if resolved.Protocol != modelChannelProtocolAutoDLComfyUI {
		if channelModel, ok := findChannelModel(resolved.Models, modelName); ok {
			modelName = channelModel.UpstreamModel
		}
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
	setting.ModelChannel.Models = mergeEnabledChannelCapabilities(setting.ModelChannel.Models, channels)
	setting.ModelChannel.PricingRules = normalizePricingRules(setting.ModelChannel.PricingRules)
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
		return 0, safeMessageError{message: "该模型或当前规格未设置价格"}
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
		if item.Model == "" || item.Modality == "" || item.Operation == "" || item.Unit == "" {
			continue
		}
		if (item.Modality == "image" || item.Modality == "video") && item.ResolutionTier == "" {
			continue
		}
		if item.Modality != "image" && item.Modality != "video" && item.ResolutionTier != "" {
			continue
		}
		result = append(result, item)
	}
	return result
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
		item.Durations = normalizeDurations(item.Durations)
		item.MaxReferenceImages = max(0, item.MaxReferenceImages)
		item.MaxReferenceVideos = max(0, item.MaxReferenceVideos)
		item.MaxReferenceAudios = max(0, item.MaxReferenceAudios)
		item.MaxReferenceMedia = max(0, item.MaxReferenceMedia)
		item.ReferenceMode = normalizeReferenceMode(item.ReferenceMode)
		if item.Modality != "image" && item.Modality != "video" {
			item.AspectRatios = []string{}
			item.ResolutionTiers = []string{}
			item.Durations = []int{}
			item.MaxReferenceImages = 0
			item.MaxReferenceVideos = 0
			item.MaxReferenceAudios = 0
			item.MaxReferenceMedia = 0
			item.SupportsAudioOutput = false
			item.ReferenceMode = "none"
		} else if item.Modality != "video" {
			item.Durations = []int{}
			item.MaxReferenceVideos = 0
			item.MaxReferenceAudios = 0
			item.MaxReferenceMedia = 0
			item.SupportsAudioOutput = false
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

func mergeEnabledChannelCapabilities(items []model.ModelDefinition, channels []model.ModelChannel) []model.ModelDefinition {
	type capabilities struct {
		modality               string
		operations             []string
		aspectRatios           []string
		resolutionTiers        []string
		durations              []int
		maxReferenceImages     int
		maxReferenceVideos     int
		maxReferenceAudios     int
		maxReferenceMedia      int
		supportsAudioOutput    bool
		referenceMode          string
		unrestrictedResolution bool
	}

	merged := map[string]capabilities{}
	for _, channel := range channels {
		if !channel.Enabled {
			continue
		}
		for _, channelModel := range channel.Models {
			modelID := strings.TrimSpace(channelModel.Model)
			modality := normalizePricingToken(channelModel.Modality)
			if modelID == "" {
				continue
			}
			if modality == "" {
				modality = defaultModelModality(modelID)
			}
			current, exists := merged[modelID]
			if exists && current.modality != modality {
				continue
			}
			current.modality = modality
			current.operations = append(current.operations, channelModel.Operations...)
			current.aspectRatios = append(current.aspectRatios, channelModel.AspectRatios...)
			if len(channelModel.ResolutionTiers) == 0 {
				current.unrestrictedResolution = true
			} else {
				current.resolutionTiers = append(current.resolutionTiers, channelModel.ResolutionTiers...)
			}
			current.durations = append(current.durations, channelModel.Durations...)
			if channelModel.MaxReferenceImages > current.maxReferenceImages || (channelModel.MaxReferenceImages == current.maxReferenceImages && current.referenceMode == "none" && channelModel.ReferenceMode != "none") {
				current.maxReferenceImages = channelModel.MaxReferenceImages
				current.referenceMode = channelModel.ReferenceMode
			}
			current.maxReferenceVideos = max(current.maxReferenceVideos, channelModel.MaxReferenceVideos)
			current.maxReferenceAudios = max(current.maxReferenceAudios, channelModel.MaxReferenceAudios)
			current.maxReferenceMedia = max(current.maxReferenceMedia, channelModel.MaxReferenceMedia)
			current.supportsAudioOutput = current.supportsAudioOutput || channelModel.SupportsAudioOutput
			merged[modelID] = current
		}
	}

	for index := range items {
		capability, ok := merged[items[index].ID]
		if !ok {
			continue
		}
		items[index].Modality = capability.modality
		items[index].Operations = normalizeModelOperations(append(items[index].Operations, capability.operations...), items[index].ID, capability.modality)
		items[index].AspectRatios = normalizeStringList(append(items[index].AspectRatios, capability.aspectRatios...), normalizePricingToken)
		items[index].ResolutionTiers = intersectResolutionTiers(items[index].ResolutionTiers, capability.resolutionTiers, capability.unrestrictedResolution)
		items[index].Durations = normalizeDurations(append(items[index].Durations, capability.durations...))
		items[index].MaxReferenceImages = capability.maxReferenceImages
		items[index].MaxReferenceVideos = capability.maxReferenceVideos
		items[index].MaxReferenceAudios = capability.maxReferenceAudios
		items[index].MaxReferenceMedia = capability.maxReferenceMedia
		items[index].SupportsAudioOutput = capability.supportsAudioOutput
		items[index].ReferenceMode = normalizeReferenceMode(capability.referenceMode)
		if capability.modality != "image" && capability.modality != "video" {
			items[index].AspectRatios = []string{}
			items[index].ResolutionTiers = []string{}
			items[index].Durations = []int{}
			items[index].MaxReferenceImages = 0
			items[index].MaxReferenceVideos = 0
			items[index].MaxReferenceAudios = 0
			items[index].MaxReferenceMedia = 0
			items[index].SupportsAudioOutput = false
			items[index].ReferenceMode = "none"
		} else if capability.modality != "video" {
			items[index].Durations = []int{}
			items[index].MaxReferenceVideos = 0
			items[index].MaxReferenceAudios = 0
			items[index].MaxReferenceMedia = 0
			items[index].SupportsAudioOutput = false
		}
	}
	return items
}

func intersectResolutionTiers(public []string, channels []string, unrestricted bool) []string {
	public = normalizeStringList(public, normalizeResolutionTier)
	if unrestricted {
		return public
	}
	available := map[string]bool{}
	for _, tier := range normalizeStringList(channels, normalizeResolutionTier) {
		available[tier] = true
	}
	result := make([]string, 0, len(public))
	for _, tier := range public {
		if available[tier] {
			result = append(result, tier)
		}
	}
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
	request.AspectRatio = normalizeVideoAspectRatio(firstPricingNonEmpty(request.AspectRatio, request.Size))
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
	request.ReferenceImages = max(0, request.ReferenceImages)
	request.ReferenceVideos = max(0, request.ReferenceVideos)
	request.ReferenceAudios = max(0, request.ReferenceAudios)
	request.ReferenceMedia = request.ReferenceImages + request.ReferenceVideos + request.ReferenceAudios
	return request
}

// NormalizePricingRequest resolves the request fields used by routing, billing, and task logs.
func NormalizePricingRequest(request PricingRequest) PricingRequest {
	return normalizePricingRequest(request)
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
	if rule.Modality != request.Modality || rule.Operation != request.Operation || rule.Unit != request.Unit {
		return 0, false
	}
	if request.Modality == "image" || request.Modality == "video" {
		if rule.ResolutionTier == "" || rule.ResolutionTier != request.ResolutionTier {
			return 0, false
		}
		return 4, true
	}
	if rule.ResolutionTier != "" {
		return 0, false
	}
	return 3, true
}

func normalizeDurations(items []int) []int {
	result := []int{}
	seen := map[int]bool{}
	for _, item := range items {
		if item <= 0 || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	sort.Ints(result)
	return result
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
		if width <= 2048*1024/height {
			return "1k"
		}
		if width <= 2048*2048/height {
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

func normalizeVideoAspectRatio(value string) string {
	value = normalizePricingToken(value)
	if strings.Contains(value, ":") {
		return value
	}
	width, height, ok := parsePricingDimensions(value)
	if !ok {
		return ""
	}
	ratio := float64(width) / float64(height)
	if math.Abs(ratio-16.0/9.0) < 0.01 {
		return "16:9"
	}
	if math.Abs(ratio-9.0/16.0) < 0.01 {
		return "9:16"
	}
	a, b := width, height
	for b != 0 {
		a, b = b, a%b
	}
	return fmt.Sprintf("%d:%d", width/a, height/a)
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
	setting.Storage = normalizeStorageSetting(setting.Storage)
	setting.Email = normalizeEmailSetting(setting.Email)
	setting.Payment = normalizePaymentSetting(setting.Payment)
	setting.Referral = normalizeReferralSetting(setting.Referral)
	setting.OperationsAlerts = normalizeOperationsAlertSetting(setting.OperationsAlerts)
	for i := range setting.Channels {
		setting.Channels[i] = normalizeModelChannel(setting.Channels[i])
	}
	return setting
}

func normalizeStorageSetting(setting model.StorageSetting) model.StorageSetting {
	setting.Driver = normalizePricingToken(setting.Driver)
	if setting.Driver != "local" && setting.Driver != "qiniu" {
		if strings.TrimSpace(config.Cfg.QiniuAccessKey) == "" || strings.TrimSpace(config.Cfg.QiniuSecretKey) == "" || strings.TrimSpace(config.Cfg.QiniuBucket) == "" || strings.TrimSpace(config.Cfg.QiniuDownloadDomain) == "" {
			setting.Driver = "local"
		} else {
			setting.Driver = "qiniu"
		}
	} else if setting.Driver != "local" {
		setting.Driver = "qiniu"
	}
	setting.RetentionDays = max(0, setting.RetentionDays)
	setting.LocalPath = strings.TrimSpace(setting.LocalPath)
	if setting.LocalPath == "" {
		setting.LocalPath = strings.TrimSpace(config.Cfg.LocalStoragePath)
	}
	setting.QiniuAccessKey = strings.TrimSpace(setting.QiniuAccessKey)
	setting.QiniuBucket = strings.TrimSpace(setting.QiniuBucket)
	setting.QiniuRegion = strings.TrimSpace(setting.QiniuRegion)
	setting.QiniuDownloadDomain = strings.TrimRight(strings.TrimSpace(setting.QiniuDownloadDomain), "/")
	if setting.QiniuAccessKey == "" {
		setting.QiniuAccessKey = strings.TrimSpace(config.Cfg.QiniuAccessKey)
	}
	if setting.QiniuSecretKey == "" {
		setting.QiniuSecretKey = strings.TrimSpace(config.Cfg.QiniuSecretKey)
	}
	if setting.QiniuBucket == "" {
		setting.QiniuBucket = strings.TrimSpace(config.Cfg.QiniuBucket)
	}
	if setting.QiniuRegion == "" {
		setting.QiniuRegion = strings.TrimSpace(config.Cfg.QiniuRegion)
	}
	if setting.QiniuDownloadDomain == "" {
		setting.QiniuDownloadDomain = strings.TrimRight(strings.TrimSpace(config.Cfg.QiniuDownloadDomain), "/")
	}
	return setting
}

func validateStorageSetting(setting model.StorageSetting) error {
	if setting.RetentionDays < 0 || setting.RetentionDays > 36500 {
		return safeMessageError{message: "旧文件保留天数必须在 0 到 36500 之间"}
	}
	if setting.Driver == "local" {
		path, err := filepath.Abs(setting.LocalPath)
		if err != nil || strings.TrimSpace(setting.LocalPath) == "" || filepath.Dir(path) == path {
			return safeMessageError{message: "本地存储目录无效，不能使用磁盘根目录"}
		}
		return nil
	}
	if setting.QiniuAccessKey == "" || setting.QiniuSecretKey == "" || setting.QiniuBucket == "" || setting.QiniuDownloadDomain == "" {
		return safeMessageError{message: "使用七牛云前请完整配置 Access Key、Secret Key、Bucket 和 HTTPS 下载域名"}
	}
	if !validHTTPSURL(setting.QiniuDownloadDomain) {
		return safeMessageError{message: "七牛下载域名必须是有效的 HTTPS 地址"}
	}
	return nil
}

func normalizeReferralSetting(setting model.ReferralSetting) model.ReferralSetting {
	if setting.CommissionRate < 0 {
		setting.CommissionRate = 0
	}
	if setting.CommissionRate > 100 {
		setting.CommissionRate = 100
	}
	if setting.CommissionRate == 0 {
		setting.Enabled = false
	}
	return setting
}

func normalizePaymentSetting(setting model.PaymentSetting) model.PaymentSetting {
	setting.GatewayURL = strings.TrimRight(strings.TrimSpace(setting.GatewayURL), "/")
	setting.MerchantID = strings.TrimSpace(setting.MerchantID)
	setting.MerchantKey = strings.TrimSpace(setting.MerchantKey)
	setting.SiteName = strings.TrimSpace(setting.SiteName)
	setting.ProductName = strings.TrimSpace(setting.ProductName)
	if setting.CreditsPerYuan < 0 {
		setting.CreditsPerYuan = 0
	}
	if setting.GatewayURL == "" {
		setting.GatewayURL = "https://www.ezfpy.cn"
	}
	if setting.SiteName == "" {
		setting.SiteName = "道生画境"
	}
	if setting.ProductName == "" {
		setting.ProductName = "余额充值"
	}
	if setting.Methods == nil {
		setting.Methods = []model.PaymentMethod{model.PaymentMethodAlipay, model.PaymentMethodWxpay}
	}
	methods := make([]model.PaymentMethod, 0, len(setting.Methods))
	seenMethods := map[model.PaymentMethod]bool{}
	for _, method := range setting.Methods {
		if (method == model.PaymentMethodAlipay || method == model.PaymentMethodWxpay || method == model.PaymentMethodQQPay) && !seenMethods[method] {
			seenMethods[method] = true
			methods = append(methods, method)
		}
	}
	setting.Methods = methods
	packages := make([]model.PaymentPackage, 0, len(setting.Packages))
	seenPackages := map[string]bool{}
	for _, item := range setting.Packages {
		item.ID = strings.TrimSpace(item.ID)
		item.Name = strings.TrimSpace(item.Name)
		if item.ID == "" || item.Name == "" || item.AmountCents <= 0 || item.BalanceCents <= 0 || seenPackages[item.ID] {
			continue
		}
		seenPackages[item.ID] = true
		packages = append(packages, item)
	}
	setting.Packages = packages
	return setting
}

func validatePaymentSetting(setting model.PaymentSetting) error {
	if !setting.Enabled {
		return nil
	}
	if setting.MerchantID == "" || setting.MerchantKey == "" {
		return safeMessageError{message: "启用在线支付前请填写易支付商户 ID 和商户密钥"}
	}
	if len(setting.Methods) == 0 || len(setting.Packages) == 0 {
		return safeMessageError{message: "启用在线支付前请至少配置一种支付方式和一个充值档位"}
	}
	gateway, err := url.Parse(setting.GatewayURL)
	if err != nil || gateway.Scheme != "https" || gateway.Host == "" || gateway.RawQuery != "" || gateway.Fragment != "" {
		return safeMessageError{message: "易支付网关必须是有效的 HTTPS 地址"}
	}
	merchantID, err := strconv.ParseInt(setting.MerchantID, 10, 64)
	if err != nil || merchantID <= 0 {
		return safeMessageError{message: "易支付商户 ID 必须是正整数"}
	}
	if _, err := paymentPublicBaseURL(); err != nil {
		return err
	}
	return nil
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
	settings.Private.Storage.QiniuSecretKeyConfigured = settings.Private.Storage.QiniuSecretKey != ""
	settings.Private.Storage.QiniuSecretKey = ""
	settings.Private.Email.PasswordConfigured = settings.Private.Email.SMTPPassword != ""
	settings.Private.Email.SMTPPassword = ""
	settings.Private.Payment.MerchantKeyConfigured = settings.Private.Payment.MerchantKey != ""
	settings.Private.Payment.MerchantKey = ""
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
	if settings.Private.Storage.QiniuSecretKey == "" {
		settings.Private.Storage.QiniuSecretKey = saved.Private.Storage.QiniuSecretKey
	}
	settings.Private.Storage.QiniuSecretKeyConfigured = false
	if settings.Private.Email.SMTPPassword == "" {
		settings.Private.Email.SMTPPassword = saved.Private.Email.SMTPPassword
	}
	settings.Private.Email.PasswordConfigured = false
	if settings.Private.Payment.MerchantKey == "" {
		settings.Private.Payment.MerchantKey = saved.Private.Payment.MerchantKey
	}
	settings.Private.Payment.MerchantKeyConfigured = false
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
	return SelectModelChannelExcluding(request, nil)
}

func SelectModelChannelByName(request PricingRequest, channelName string) (ModelChannelSelection, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return ModelChannelSelection{}, err
	}
	request = normalizePricingRequest(request)
	for _, selection := range modelChannelSelectionsForRequest(normalizePrivateSetting(settings.Private).Channels, request) {
		if selection.Channel.Name == channelName {
			return selection, nil
		}
	}
	return ModelChannelSelection{}, safeMessageError{message: "原生成渠道已不可用"}
}

// SelectModelChannelExcluding selects a compatible channel that has not already failed this request.
func SelectModelChannelExcluding(request PricingRequest, excludedChannels []string) (ModelChannelSelection, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return ModelChannelSelection{}, err
	}
	request = normalizePricingRequest(request)
	selections := modelChannelSelectionsForRequest(normalizePrivateSetting(settings.Private).Channels, request)
	selections = modelChannelSelectionsExcluding(selections, excludedChannels)
	if len(selections) == 0 {
		reason := "当前操作或规格"
		if request.ReferenceImages > 0 {
			reason = "当前操作、规格或参考图数量"
		}
		return ModelChannelSelection{}, safeMessageError{message: fmt.Sprintf("模型 %s 没有支持%s的可用渠道", request.Model, reason)}
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

func modelChannelSelectionsExcluding(selections []ModelChannelSelection, excludedChannels []string) []ModelChannelSelection {
	excluded := map[string]bool{}
	for _, channel := range excludedChannels {
		excluded[channel] = true
	}
	result := make([]ModelChannelSelection, 0, len(selections))
	for _, selection := range selections {
		if !excluded[selection.Channel.Name] {
			result = append(result, selection)
		}
	}
	return result
}

func BuildModelChannelURL(channel model.ModelChannel, path string) string {
	baseURL := normalizeModelChannelBaseURL(channel.BaseURL)
	lowerBaseURL := strings.ToLower(baseURL)
	if !strings.HasSuffix(lowerBaseURL, "/v1") {
		baseURL += "/v1"
	}
	return baseURL + path
}

func normalizeModelChannelBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
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
	channel.Protocol = normalizePricingToken(channel.Protocol)
	if channel.Protocol != modelChannelProtocolAutoDLComfyUI {
		channel.Protocol = "openai"
	}
	channel.Name = strings.TrimSpace(channel.Name)
	channel.BaseURL = normalizeModelChannelBaseURL(channel.BaseURL)
	if channel.Protocol == modelChannelProtocolAutoDLComfyUI && channel.BaseURL == "" {
		channel.BaseURL = "https://autodl.art"
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
		item.Modality = normalizePricingToken(item.Modality)
		if item.Modality == "" {
			item.Modality = defaultModelModality(item.Model)
		}
		item.Operations = normalizeModelOperations(item.Operations, item.Model, item.Modality)
		item.AspectRatios = normalizeStringList(item.AspectRatios, normalizePricingToken)
		item.ResolutionTiers = normalizeStringList(item.ResolutionTiers, normalizeResolutionTier)
		item.Durations = normalizeDurations(item.Durations)
		item.MaxReferenceImages = max(0, item.MaxReferenceImages)
		item.MaxReferenceVideos = max(0, item.MaxReferenceVideos)
		item.MaxReferenceAudios = max(0, item.MaxReferenceAudios)
		item.MaxReferenceMedia = max(0, item.MaxReferenceMedia)
		item.ReferenceMode = normalizeReferenceMode(item.ReferenceMode)
		if item.Workflow != nil {
			item.Workflow.WorkflowID = strings.TrimSpace(item.Workflow.WorkflowID)
			item.Workflow.RequestTemplate = strings.TrimSpace(item.Workflow.RequestTemplate)
			item.Workflow.ValueMaps = strings.TrimSpace(item.Workflow.ValueMaps)
			if item.Workflow.ValueMaps == "" {
				item.Workflow.ValueMaps = "{}"
			}
		}
		if item.Modality == "image" && item.MaxReferenceImages == 0 && containsPricingValue(item.Operations, "edit") {
			item.MaxReferenceImages = 1
		}
		if item.Modality != "image" && item.Modality != "video" {
			item.MaxReferenceImages = 0
			item.MaxReferenceVideos = 0
			item.MaxReferenceAudios = 0
			item.MaxReferenceMedia = 0
			item.SupportsAudioOutput = false
			item.ReferenceMode = "none"
		} else if item.Modality != "video" {
			item.MaxReferenceVideos = 0
			item.MaxReferenceAudios = 0
			item.MaxReferenceMedia = 0
			item.SupportsAudioOutput = false
		}
		models = append(models, item)
	}
	channel.Models = models
	if channel.Weight <= 0 {
		channel.Weight = 1
	}
	return channel
}

func validateModelChannels(channels []model.ModelChannel) error {
	for _, channel := range channels {
		if channel.Protocol != modelChannelProtocolAutoDLComfyUI {
			continue
		}
		baseURL, err := url.Parse(channel.BaseURL)
		if err != nil || baseURL.Scheme != "https" || baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
			return safeMessageError{message: fmt.Sprintf("AutoDL 渠道 %s 必须使用有效的 HTTPS 接口地址", channel.Name)}
		}
		for _, item := range channel.Models {
			if item.Modality != "image" && item.Modality != "video" {
				return safeMessageError{message: fmt.Sprintf("AutoDL 模型 %s 只支持图片或视频类型", item.Model)}
			}
			if err := validateComfyUIWorkflow(item.Workflow); err != nil {
				return safeMessageError{message: fmt.Sprintf("AutoDL 模型 %s：%s", item.Model, err.Error())}
			}
		}
	}
	return nil
}

func validateComfyUIWorkflow(workflow *model.ComfyUIWorkflowConfig) error {
	if workflow == nil || !comfyUIWorkflowIDPattern.MatchString(workflow.WorkflowID) {
		return errors.New("工作流 ID 只能包含字母、数字、点、下划线和短横线")
	}
	if len(workflow.RequestTemplate) == 0 || len(workflow.RequestTemplate) > 64<<10 {
		return errors.New("请求模板不能为空且不能超过 64KB")
	}
	if len(workflow.ValueMaps) > 64<<10 {
		return errors.New("参数映射不能超过 64KB")
	}
	var template map[string]any
	if json.Unmarshal([]byte(workflow.RequestTemplate), &template) != nil {
		return errors.New("请求模板必须是 JSON 对象")
	}
	if err := validateComfyUITemplateValue(template); err != nil {
		return err
	}
	var valueMaps map[string]map[string]any
	if json.Unmarshal([]byte(workflow.ValueMaps), &valueMaps) != nil {
		return errors.New("参数映射必须是 JSON 对象，例如 {\"resolution\":{\"720p\":\"720P\"}}")
	}
	return nil
}

func validateComfyUITemplateValue(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			if err := validateComfyUITemplateValue(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := validateComfyUITemplateValue(child); err != nil {
				return err
			}
		}
	case string:
		if strings.Contains(typed, "${") && !isComfyUIPlaceholder(typed) {
			return fmt.Errorf("模板占位符 %s 不受支持或未独占整个 JSON 值", typed)
		}
	}
	return nil
}

func isComfyUIPlaceholder(value string) bool {
	if !strings.HasPrefix(value, "${") || !strings.HasSuffix(value, "}") || strings.Count(value, "${") != 1 {
		return false
	}
	name := strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}")
	switch name {
	case "prompt", "negative_prompt", "width", "height", "size", "ratio", "resolution", "seconds", "count", "seed", "generate_audio", "reference_image_url", "reference_image_urls", "reference_video_urls", "reference_audio_urls":
		return true
	}
	if strings.HasPrefix(name, "reference_image_") {
		index, err := strconv.Atoi(strings.TrimPrefix(name, "reference_image_"))
		return err == nil && index >= 0 && index < 16
	}
	if strings.HasPrefix(name, "reference_audio_") {
		index, err := strconv.Atoi(strings.TrimPrefix(name, "reference_audio_"))
		return err == nil && index >= 0 && index < 3
	}
	return false
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

func fetchAdminChannelModels(channel model.ModelChannel) ([]model.DiscoveredModel, error) {
	if channel.Protocol == modelChannelProtocolAutoDLComfyUI {
		result := make([]model.DiscoveredModel, 0, len(channel.Models))
		for _, item := range channel.Models {
			result = append(result, model.DiscoveredModel{ID: item.Model, Kind: item.Modality, Modality: item.Modality, SupportedRatios: item.AspectRatios, SupportedResolutions: item.ResolutionTiers, SupportedDurations: item.Durations, MaxReferenceImages: item.MaxReferenceImages, MaxReferenceVideos: item.MaxReferenceVideos, MaxReferenceAudios: item.MaxReferenceAudios, MaxReferenceMedia: item.MaxReferenceMedia, SupportsAudioOutput: item.SupportsAudioOutput, ReferenceMode: item.ReferenceMode, ReferenceCapabilityProvided: true, ReferenceVideosProvided: true, ReferenceAudiosProvided: true, ReferenceMediaProvided: true, AudioOutputProvided: true})
		}
		return result, nil
	}
	request, err := http.NewRequest(http.MethodGet, BuildModelChannelURL(channel, "/models?extended=true"), nil)
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
		return nil, readAdminChannelError(body, response.StatusCode, "读取模型失败")
	}
	var payload struct {
		Object string `json:"object"`
		Data   []struct {
			ID                   string            `json:"id"`
			Kind                 string            `json:"kind"`
			SupportedRatios      []string          `json:"supported_ratios"`
			SupportedResolutions []string          `json:"supported_resolutions"`
			SupportedDurations   []json.RawMessage `json:"supported_durations"`
			MaxReferenceImages   *int              `json:"max_reference_images"`
			MaxReferenceVideos   *int              `json:"max_reference_videos"`
			MaxReferenceAudios   *int              `json:"max_reference_audios"`
			MaxReferenceMedia    *int              `json:"max_reference_media"`
			SupportsAudioOutput  *bool             `json:"supports_audio_output"`
			ReferenceMode        *string           `json:"reference_mode"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Object != "list" || payload.Data == nil {
		return nil, safeMessageError{message: "读取模型失败：上游返回的不是 OpenAI 兼容 /models?extended=true 格式"}
	}
	result := make([]model.DiscoveredModel, 0, len(payload.Data))
	for _, item := range payload.Data {
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" {
			continue
		}
		modality := discoveredModelModality(item.Kind, item.ID)
		resolutionNormalizer := normalizeResolutionTier
		if modality == "video" {
			resolutionNormalizer = normalizeVideoResolutionTier
		}
		resolutions := normalizeStringList(item.SupportedResolutions, resolutionNormalizer)
		ratios := normalizeStringList(item.SupportedRatios, normalizePricingToken)
		for _, resolution := range item.SupportedResolutions {
			if ratio := normalizeVideoAspectRatio(resolution); modality == "video" && ratio != "" {
				ratios = normalizeStringList(append(ratios, ratio), normalizePricingToken)
			}
		}
		maxReferenceImages := 0
		if item.MaxReferenceImages != nil {
			maxReferenceImages = max(0, *item.MaxReferenceImages)
		}
		maxReferenceVideos := 0
		if item.MaxReferenceVideos != nil {
			maxReferenceVideos = max(0, *item.MaxReferenceVideos)
		}
		maxReferenceAudios := 0
		if item.MaxReferenceAudios != nil {
			maxReferenceAudios = max(0, *item.MaxReferenceAudios)
		}
		maxReferenceMedia := 0
		if item.MaxReferenceMedia != nil {
			maxReferenceMedia = max(0, *item.MaxReferenceMedia)
		}
		supportsAudioOutput := item.SupportsAudioOutput != nil && *item.SupportsAudioOutput
		referenceMode := "none"
		if item.ReferenceMode != nil {
			referenceMode = normalizeReferenceMode(*item.ReferenceMode)
		}
		result = append(result, model.DiscoveredModel{ID: item.ID, Kind: strings.TrimSpace(item.Kind), Modality: modality, SupportedRatios: ratios, SupportedResolutions: resolutions, SupportedDurations: parseSupportedDurations(item.SupportedDurations), MaxReferenceImages: maxReferenceImages, MaxReferenceVideos: maxReferenceVideos, MaxReferenceAudios: maxReferenceAudios, MaxReferenceMedia: maxReferenceMedia, SupportsAudioOutput: supportsAudioOutput, ReferenceMode: referenceMode, ReferenceCapabilityProvided: item.MaxReferenceImages != nil || item.ReferenceMode != nil, ReferenceVideosProvided: item.MaxReferenceVideos != nil, ReferenceAudiosProvided: item.MaxReferenceAudios != nil, ReferenceMediaProvided: item.MaxReferenceMedia != nil, AudioOutputProvided: item.SupportsAudioOutput != nil})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func parseSupportedDurations(values []json.RawMessage) []int {
	durations := make([]int, 0, len(values))
	for _, raw := range values {
		var seconds int
		if json.Unmarshal(raw, &seconds) == nil {
			durations = append(durations, seconds)
			continue
		}
		var text string
		if json.Unmarshal(raw, &text) == nil {
			seconds, err := strconv.Atoi(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(text)), "s"))
			if err == nil {
				durations = append(durations, seconds)
			}
		}
	}
	return normalizeDurations(durations)
}

func normalizeReferenceMode(value string) string {
	switch normalizePricingToken(value) {
	case "frame", "asset":
		return normalizePricingToken(value)
	default:
		return "none"
	}
}

func testAdminChannelModel(channel model.ModelChannel, modelName string) (string, error) {
	if strings.TrimSpace(modelName) == "" {
		return "", errors.New("缺少模型名称")
	}
	targetModel := strings.TrimSpace(modelName)
	if channel.Protocol == modelChannelProtocolAutoDLComfyUI {
		configured, ok := findChannelModel(channel.Models, targetModel)
		if !ok {
			return "", safeMessageError{message: "测试失败：渠道未配置该模型"}
		}
		if err := validateComfyUIWorkflow(configured.Workflow); err != nil {
			return "", safeMessageError{message: "配置校验失败：" + err.Error()}
		}
		if err := testAutoDLComfyUICredentials(context.Background(), channel); err != nil {
			return "", err
		}
		return "AutoDL Token 与 ComfyUI API 权限校验通过（未提交付费任务）", nil
	}
	if configured, ok := findChannelModel(channel.Models, targetModel); ok && strings.TrimSpace(configured.UpstreamModel) != "" {
		targetModel = configured.UpstreamModel
	}
	models, err := fetchAdminChannelModels(channel)
	if err != nil {
		return "", err
	}
	for _, item := range models {
		if item.ID == targetModel {
			return fmt.Sprintf("OpenAI /models?extended=true 校验成功（%s）", item.Modality), nil
		}
	}
	return "", safeMessageError{message: "测试失败：上游 /models?extended=true 未返回该模型"}
}

func discoveredModelModality(kind string, modelName string) string {
	switch normalizePricingToken(kind) {
	case "image":
		return "image"
	case "video":
		return "video"
	case "audio", "speech":
		return "audio"
	case "text", "chat", "completion":
		return "text"
	default:
		return defaultModelModality(modelName)
	}
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
	if request.Modality != "" && item.Modality != "" && item.Modality != request.Modality {
		return false
	}
	if request.Operation != "" && !containsPricingValue(item.Operations, request.Operation) {
		return false
	}
	if request.ResolutionTier != "" && len(item.ResolutionTiers) > 0 && !containsPricingValue(item.ResolutionTiers, request.ResolutionTier) {
		return false
	}
	if request.Modality == "video" && request.AspectRatio != "" && len(item.AspectRatios) > 0 && !containsPricingValue(item.AspectRatios, request.AspectRatio) {
		return false
	}
	if request.ReferenceImages > item.MaxReferenceImages {
		return false
	}
	if request.ReferenceVideos > item.MaxReferenceVideos || request.ReferenceAudios > item.MaxReferenceAudios {
		return false
	}
	if item.MaxReferenceMedia > 0 && request.ReferenceMedia > item.MaxReferenceMedia {
		return false
	}
	if request.GenerateAudio && !item.SupportsAudioOutput {
		return false
	}
	return request.Modality != "video" || request.Quantity < 1 || len(item.Durations) == 0 || containsInt(item.Durations, request.Quantity)
}

func containsInt(items []int, value int) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
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
