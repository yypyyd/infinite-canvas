package service

import (
	"fmt"
	"strings"

	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/model"
)

// Built-in providers are first-class channels (NewAPI-style), not a single
// virtual "builtin-account-pool". Each participates in site-level selection
// with third-party channels. Account-level failover stays inside the gateway.

const (
	BuiltinProviderChannelPrefix = "builtin-provider-"

	DefaultProviderPriorityChatGPT  = 200
	DefaultProviderPriorityGrok     = 200
	DefaultProviderPriorityBytePlus = 190
	DefaultProviderPriorityAdobe    = 180
	DefaultProviderWeight           = 1
)

// BuiltinProviderOrder is the stable admin / migration order.
var BuiltinProviderOrder = []string{"chatgpt", "grok", "byteplus", "adobe"}

var builtinProviderLabels = map[string]string{
	"chatgpt":  "ChatGPT",
	"grok":     "Grok",
	"byteplus": "BytePlus",
	"adobe":    "Adobe Firefly",
}

var defaultBuiltinProviderPriority = map[string]int{
	"chatgpt":  DefaultProviderPriorityChatGPT,
	"grok":     DefaultProviderPriorityGrok,
	"byteplus": DefaultProviderPriorityBytePlus,
	"adobe":    DefaultProviderPriorityAdobe,
}

// BuiltinProviderChannelID returns the virtual channel id for a provider.
func BuiltinProviderChannelID(provider string) string {
	return BuiltinProviderChannelPrefix + normalizeBuiltinProvider(provider)
}

// BuiltinProviderChannelName returns the display name for a provider channel.
func BuiltinProviderChannelName(provider string) string {
	provider = normalizeBuiltinProvider(provider)
	if label, ok := builtinProviderLabels[provider]; ok {
		return label
	}
	return provider
}

func normalizeBuiltinProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

// IsBuiltinProviderChannel reports whether the channel is a built-in provider
// channel (or the legacy single pool channel / internal base URL).
func IsBuiltinProviderChannel(channel model.ModelChannel) bool {
	return BuiltinProviderOf(channel) != "" || isLegacyBuiltinPoolChannel(channel)
}

// BuiltinProviderOf returns the provider key for a built-in provider channel.
func BuiltinProviderOf(channel model.ModelChannel) string {
	id := strings.TrimSpace(channel.ID)
	if strings.HasPrefix(id, BuiltinProviderChannelPrefix) {
		return normalizeBuiltinProvider(strings.TrimPrefix(id, BuiltinProviderChannelPrefix))
	}
	return ""
}

func isLegacyBuiltinPoolChannel(channel model.ModelChannel) bool {
	if channel.ID == BuiltinPoolChannelID {
		return true
	}
	return IsBuiltinPoolBaseURL(channel.BaseURL) && !strings.HasPrefix(channel.ID, BuiltinProviderChannelPrefix)
}

// IsBuiltinPoolChannel reports whether a channel is any built-in pool surface
// (legacy unified pool or a flat provider channel). Kept for call-site compat.
func IsBuiltinPoolChannel(channel model.ModelChannel) bool {
	return IsBuiltinProviderChannel(channel) || isBuiltinPoolChannel(channel)
}
func defaultPriorityForProvider(provider string) int {
	if p, ok := defaultBuiltinProviderPriority[normalizeBuiltinProvider(provider)]; ok {
		return p
	}
	return DefaultProviderPriorityChatGPT
}

// migrateBuiltinProviders builds per-provider settings from legacy BuiltinPool
// + PoolModelAliases when BuiltinProviders is empty. Idempotent.
func migrateBuiltinProviders(private model.PrivateSetting) []model.BuiltinProviderSource {
	if len(private.BuiltinProviders) > 0 {
		return normalizeBuiltinProviders(private.BuiltinProviders)
	}
	enabled, priority, weight := true, 0, DefaultProviderWeight
	useLegacyPriority := false
	if private.BuiltinPool != nil {
		if private.BuiltinPool.Enabled != nil {
			enabled = *private.BuiltinPool.Enabled
		}
		if private.BuiltinPool.Priority != nil {
			priority = *private.BuiltinPool.Priority
			useLegacyPriority = true
		}
		if private.BuiltinPool.Weight != nil {
			weight = *private.BuiltinPool.Weight
		}
	}
	overrides := builtinPoolModelOverrides(private.BuiltinPool)
	aliases := normalizePoolModelAliases(private.PoolModelAliases)
	out := make([]model.BuiltinProviderSource, 0, len(BuiltinProviderOrder))
	for _, provider := range BuiltinProviderOrder {
		item := model.BuiltinProviderSource{Provider: provider}
		en := enabled
		item.Enabled = &en
		w := weight
		item.Weight = &w
		if useLegacyPriority {
			p := priority
			item.Priority = &p
		} else {
			p := defaultPriorityForProvider(provider)
			item.Priority = &p
		}
		for modelID, override := range overrides {
			item.Models = append(item.Models, model.BuiltinPoolModelSource{
				Model: modelID, Disabled: override.Disabled,
				Priority: cloneIntPtr(override.Priority), Weight: cloneIntPtr(override.Weight),
			})
		}
		if len(aliases) > 0 {
			item.ModelMappings = append([]model.PoolModelAlias(nil), aliases...)
		}
		out = append(out, item)
	}
	return normalizeBuiltinProviders(out)
}

func normalizeBuiltinProviders(items []model.BuiltinProviderSource) []model.BuiltinProviderSource {
	byProvider := map[string]model.BuiltinProviderSource{}
	order := []string{}
	for _, item := range items {
		provider := normalizeBuiltinProvider(item.Provider)
		if provider == "" {
			continue
		}
		if _, seen := byProvider[provider]; !seen {
			order = append(order, provider)
		}
		item.Provider = provider
		item.Priority = clampPriorityPtr(item.Priority)
		item.Weight = clampWeightPtr(item.Weight)
		item.ModelMappings = normalizePoolModelAliases(item.ModelMappings)
		models := make([]model.BuiltinPoolModelSource, 0, len(item.Models))
		seenModel := map[string]bool{}
		for _, m := range item.Models {
			m.Model = strings.TrimSpace(m.Model)
			m.Priority = clampPriorityPtr(m.Priority)
			m.Weight = clampWeightPtr(m.Weight)
			if m.Model == "" || seenModel[m.Model] || (!m.Disabled && m.Priority == nil && m.Weight == nil) {
				continue
			}
			seenModel[m.Model] = true
			models = append(models, m)
		}
		item.Models = models
		byProvider[provider] = item
	}
	for _, provider := range BuiltinProviderOrder {
		if _, ok := byProvider[provider]; ok {
			continue
		}
		en := true
		p := defaultPriorityForProvider(provider)
		w := DefaultProviderWeight
		byProvider[provider] = model.BuiltinProviderSource{Provider: provider, Enabled: &en, Priority: &p, Weight: &w}
		order = append(order, provider)
	}
	out := make([]model.BuiltinProviderSource, 0, len(order))
	seen := map[string]bool{}
	for _, provider := range append(append([]string{}, BuiltinProviderOrder...), order...) {
		if seen[provider] {
			continue
		}
		item, ok := byProvider[provider]
		if !ok {
			continue
		}
		seen[provider] = true
		out = append(out, item)
	}
	return out
}

func builtinProviderSourceMap(items []model.BuiltinProviderSource) map[string]model.BuiltinProviderSource {
	out := map[string]model.BuiltinProviderSource{}
	for _, item := range items {
		out[normalizeBuiltinProvider(item.Provider)] = item
	}
	return out
}

func builtinProviderDefaults(src model.BuiltinProviderSource, provider string) (enabled bool, priority, weight int) {
	enabled, priority, weight = true, defaultPriorityForProvider(provider), DefaultProviderWeight
	if src.Enabled != nil {
		enabled = *src.Enabled
	}
	if src.Priority != nil {
		priority = *src.Priority
	}
	if src.Weight != nil {
		weight = *src.Weight
	}
	return
}

func builtinProviderModelOverrides(src model.BuiltinProviderSource) map[string]model.BuiltinPoolModelSource {
	out := map[string]model.BuiltinPoolModelSource{}
	for _, item := range src.Models {
		out[strings.TrimSpace(item.Model)] = item
	}
	return out
}

// builtinProviderChannels builds one virtual channel per enabled provider.
func builtinProviderChannels(private model.PrivateSetting) []model.ModelChannel {
	items := builtinPoolModels()
	if len(items) == 0 {
		return nil
	}
	providers := migrateBuiltinProviders(private)
	byProvider := builtinProviderSourceMap(providers)
	modelsByProvider := map[string][]gateway.PoolModel{}
	for _, item := range items {
		providersForModel := map[string]bool{}
		for _, route := range item.Routes {
			p := normalizeBuiltinProvider(route.Provider)
			if p != "" {
				providersForModel[p] = true
			}
		}
		if len(providersForModel) == 0 {
			for _, p := range BuiltinProviderOrder {
				providersForModel[p] = true
			}
		}
		for p := range providersForModel {
			modelsByProvider[p] = append(modelsByProvider[p], item)
		}
	}
	out := make([]model.ModelChannel, 0, len(BuiltinProviderOrder))
	for _, provider := range BuiltinProviderOrder {
		src := byProvider[provider]
		enabled, priority, weight := builtinProviderDefaults(src, provider)
		if !enabled {
			continue
		}
		poolModels := modelsByProvider[provider]
		if len(poolModels) == 0 && len(src.ModelMappings) == 0 {
			continue
		}
		channel := model.ModelChannel{
			ID: BuiltinProviderChannelID(provider), Protocol: "openai",
			Name: BuiltinProviderChannelName(provider), BaseURL: gateway.InternalBaseURL,
			APIKey: "in-process", Weight: weight, Priority: priority, Enabled: true,
			Remark: fmt.Sprintf("built-in provider %s", BuiltinProviderChannelName(provider)),
		}
		overrides := builtinProviderModelOverrides(src)
		for _, item := range poolModels {
			cm := poolModelToChannelModelForProvider(item, provider)
			if override, ok := overrides[cm.Model]; ok {
				cm.Disabled = cm.Disabled || override.Disabled
				cm.Priority = cloneIntPtr(override.Priority)
				cm.Weight = cloneIntPtr(override.Weight)
			}
			channel.Models = append(channel.Models, cm)
		}
		mappings := src.ModelMappings
		if len(mappings) == 0 {
			mappings = private.PoolModelAliases
		}
		channel.Models = append(channel.Models, poolAliasModels(channel.Models, mappings)...)
		kept := channel.Models[:0]
		for _, cm := range channel.Models {
			if !cm.Disabled {
				kept = append(kept, cm)
			}
		}
		channel.Models = kept
		if len(channel.Models) == 0 {
			continue
		}
		out = append(out, normalizeModelChannel(channel))
	}
	return out
}

func poolModelToChannelModel(item gateway.PoolModel) model.ChannelModel {
	maxImages := item.MaxReferenceImages
	referenceMode := "none"
	if item.Modality == "video" && maxImages > 0 {
		referenceMode = "frame"
	}
	return model.ChannelModel{
		Model: item.ID, UpstreamModel: item.ID, Modality: item.Modality,
		Operations: item.Operations, AspectRatios: item.AspectRatios,
		ResolutionTiers: item.ResolutionTiers, Durations: item.Durations,
		MaxReferenceImages: maxImages, MaxReferenceVideos: item.MaxReferenceVideos,
		MaxReferenceAudios: item.MaxReferenceAudios, MaxReferenceMedia: item.MaxReferenceMedia,
		SupportsAudioOutput: item.SupportsAudioOutput, ReferenceMode: referenceMode,
	}
}

// BuiltinPoolTaskChannelNames lists channel names that identify built-in pool
// traffic in generation_task rows (legacy virtual pool + flat providers).
func BuiltinPoolTaskChannelNames() []string {
	names := []string{BuiltinPoolChannelName}
	for _, provider := range BuiltinProviderOrder {
		names = append(names, BuiltinProviderChannelName(provider))
	}
	return names
}

// BuiltinPoolTaskChannelIDs lists channel ids that identify built-in pool traffic.
func BuiltinPoolTaskChannelIDs() []string {
	ids := []string{BuiltinPoolChannelID}
	for _, provider := range BuiltinProviderOrder {
		ids = append(ids, BuiltinProviderChannelID(provider))
	}
	return ids
}
