package service

import (
	"sort"
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
)

// SupplyRoute is one entry in the unified route table that admin「供应源」,
// capability/overview availability, plaza status and enable toggles share.
//
// Kind is the primary discriminator:
//   - "provider" — 号池平台 flat provider (chatgpt/adobe/byteplus/grok)
//   - "channel"  — 外部渠道 (ModelChannel / LecAPI-style)
//
// Legacy Kind "pool" still appears on per-model source rows that aggregate the
// old single virtual pool; new code should prefer "provider".
type SupplyRoute struct {
	Kind       string `json:"kind"` // provider | channel
	ID         string `json:"id"`
	Name       string `json:"name"`
	Provider   string `json:"provider,omitempty"`
	Enabled    bool   `json:"enabled"`
	Priority   int    `json:"priority"`
	Weight     int    `json:"weight"`
	ModelCount int    `json:"modelCount"`
	Protocol   string `json:"protocol,omitempty"`
}

// ListSupplyRoutes builds the conceptual route table from site settings.
// Disabled supplies stay in the list (Enabled=false) so admin toggles and
// overview can show them; runtime selection filters via runtimeChannelsFor.
func ListSupplyRoutes(private model.PrivateSetting) []SupplyRoute {
	private.BuiltinProviders = migrateBuiltinProviders(private)
	out := make([]SupplyRoute, 0, len(private.BuiltinProviders)+len(private.Channels))

	modelCounts := map[string]int{}
	for _, ch := range builtinProviderChannels(private) {
		modelCounts[BuiltinProviderOf(ch)] = len(ch.Models)
	}

	for _, src := range private.BuiltinProviders {
		provider := normalizeBuiltinProvider(src.Provider)
		if provider == "" {
			continue
		}
		enabled, priority, weight := builtinProviderDefaults(src, provider)
		out = append(out, SupplyRoute{
			Kind:       "provider",
			ID:         BuiltinProviderChannelID(provider),
			Name:       BuiltinProviderChannelName(provider),
			Provider:   provider,
			Enabled:    enabled,
			Priority:   priority,
			Weight:     weight,
			ModelCount: modelCounts[provider],
			Protocol:   "openai",
		})
	}

	for _, channel := range private.Channels {
		if IsBuiltinProviderChannel(channel) || isBuiltinPoolChannel(channel) {
			continue
		}
		out = append(out, SupplyRoute{
			Kind:       "channel",
			ID:         channel.ID,
			Name:       channel.Name,
			Enabled:    channel.Enabled,
			Priority:   channel.Priority,
			Weight:     channel.Weight,
			ModelCount: len(channel.Models),
			Protocol:   channel.Protocol,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// SupplyRouteByID finds one supply in the unified table.
func SupplyRouteByID(private model.PrivateSetting, id string) (SupplyRoute, bool) {
	id = strings.TrimSpace(id)
	for _, item := range ListSupplyRoutes(private) {
		if item.ID == id || (item.Provider != "" && item.Provider == id) {
			return item, true
		}
	}
	return SupplyRoute{}, false
}

// IsProviderSourceKind reports whether a per-model source kind is a 号池 platform entry.
func IsProviderSourceKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "provider", "pool":
		return true
	default:
		return false
	}
}
