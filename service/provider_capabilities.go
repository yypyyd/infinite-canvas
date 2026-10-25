package service

import (
	"sort"
	"strings"

	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/model"
)

// ProviderModelCapability is one logical model as declared by a single builtin
// provider route (not the accidental union across chatgpt/adobe/byteplus).
type ProviderModelCapability struct {
	Provider        string   `json:"provider"`
	Model           string   `json:"model"`
	Modality        string   `json:"modality"`
	Operations      []string `json:"operations,omitempty"`
	AspectRatios    []string `json:"aspectRatios,omitempty"`
	ResolutionTiers []string `json:"resolutionTiers,omitempty"`
	Durations       []int    `json:"durations,omitempty"`
	Priority        int      `json:"priority"`
}

// ListProviderModelCapabilities returns per-provider declarations for enabled
// builtin supplies. Callers that clamp UI or validate modality must prefer
// these over a flat union of every route serving the same logical model.
func ListProviderModelCapabilities(private model.PrivateSetting) []ProviderModelCapability {
	channels := builtinProviderChannels(private)
	out := make([]ProviderModelCapability, 0)
	for _, channel := range channels {
		provider := BuiltinProviderOf(channel)
		if provider == "" {
			continue
		}
		for _, cm := range channel.Models {
			out = append(out, ProviderModelCapability{
				Provider:        provider,
				Model:           cm.Model,
				Modality:        cm.Modality,
				Operations:      append([]string(nil), cm.Operations...),
				AspectRatios:    append([]string(nil), cm.AspectRatios...),
				ResolutionTiers: append([]string(nil), cm.ResolutionTiers...),
				Durations:       append([]int(nil), cm.Durations...),
				Priority:        channel.Priority,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		if out[i].Model != out[j].Model {
			return out[i].Model < out[j].Model
		}
		return out[i].Provider < out[j].Provider
	})
	return out
}

// PrimaryProviderCapability picks the highest-priority enabled builtin
// declaration for a logical model. Used by public catalog clamp so gpt-image*
// follows ChatGPT's 1K route when that supply is on, instead of Adobe/BytePlus
// 2K/4K leaking through a union.
func PrimaryProviderCapability(private model.PrivateSetting, modelID string) (ProviderModelCapability, bool) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ProviderModelCapability{}, false
	}
	for _, item := range ListProviderModelCapabilities(private) {
		if item.Model == modelID {
			return item, true
		}
	}
	return ProviderModelCapability{}, false
}

// ClampResolutionTiersToDeclarations replaces a unioned catalog tier list with
// the primary enabled provider's declaration when that declaration is non-empty.
// Third-party-only models (no builtin declaration) keep the input tiers.
func ClampResolutionTiersToDeclarations(private model.PrivateSetting, modelID string, tiers []string) []string {
	cap, ok := PrimaryProviderCapability(private, modelID)
	if !ok || len(cap.ResolutionTiers) == 0 {
		return tiers
	}
	return normalizeStringList(cap.ResolutionTiers, normalizeResolutionTier)
}

// poolModelToChannelModelForProvider scopes a pool model to one provider's
// route capabilities. Empty Routes keeps the logical-model union (test stubs
// and legacy rows without route summaries).
func poolModelToChannelModelForProvider(item gateway.PoolModel, provider string) model.ChannelModel {
	cm := poolModelToChannelModel(item)
	provider = normalizeBuiltinProvider(provider)
	if provider == "" || len(item.Routes) == 0 {
		return cm
	}
	var matched *gateway.PoolRoute
	for i := range item.Routes {
		if normalizeBuiltinProvider(item.Routes[i].Provider) == provider {
			matched = &item.Routes[i]
			break
		}
	}
	if matched == nil {
		return cm
	}
	if len(matched.Operations) > 0 {
		cm.Operations = append([]string(nil), matched.Operations...)
	}
	if len(matched.ResolutionTiers) > 0 {
		cm.ResolutionTiers = append([]string(nil), matched.ResolutionTiers...)
	}
	return cm
}
