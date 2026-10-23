package service

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/model"
)

// The built-in account pool is offered as a virtual channel. It is never
// stored in settings, so it needs no configuration and disappears cleanly
// when the pool is not running.
const (
	BuiltinPoolChannelID   = "builtin-account-pool"
	BuiltinPoolChannelName = "内置账号池"
)

// builtinPoolModels is replaceable in tests.
var builtinPoolModels = func() []gateway.PoolModel {
	m := gateway.Current()
	if m == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	items, err := m.Models(ctx)
	if err != nil {
		logPoolModelsError(err)
		return nil
	}
	return items
}

var lastPoolModelsLog atomic.Int64

func logPoolModelsError(err error) {
	now := time.Now().Unix()
	if last := lastPoolModelsLog.Load(); now-last >= 60 && lastPoolModelsLog.CompareAndSwap(last, now) {
		log.Printf("built-in account pool models unavailable: %v", err)
	}
}

// builtinPoolChannel returns the virtual channel for the running pool,
// including the enabled aliases whose target the pool currently serves.
func builtinPoolChannel(aliases []model.PoolModelAlias) (model.ModelChannel, bool) {
	items := builtinPoolModels()
	if len(items) == 0 {
		return model.ModelChannel{}, false
	}
	channel := model.ModelChannel{
		ID:       BuiltinPoolChannelID,
		Protocol: "openai",
		Name:     BuiltinPoolChannelName,
		BaseURL:  gateway.InternalBaseURL,
		// Not a secret: the in-process listener ignores Authorization. It is
		// non-empty only because channel selection skips keyless channels.
		APIKey:  "in-process",
		Weight:  1,
		Enabled: true,
		Remark:  "由内置账号池自动提供，无需配置",
	}
	for _, item := range items {
		maxImages := item.MaxReferenceImages
		// referenceMode only applies to video: pool video models take
		// reference images as frames (image-to-video).
		referenceMode := "none"
		if item.Modality == "video" && maxImages > 0 {
			referenceMode = "frame"
		}
		channel.Models = append(channel.Models, model.ChannelModel{
			Model:               item.ID,
			UpstreamModel:       item.ID,
			Modality:            item.Modality,
			Operations:          item.Operations,
			AspectRatios:        item.AspectRatios,
			ResolutionTiers:     item.ResolutionTiers,
			Durations:           item.Durations,
			MaxReferenceImages:  maxImages,
			MaxReferenceVideos:  item.MaxReferenceVideos,
			MaxReferenceAudios:  item.MaxReferenceAudios,
			MaxReferenceMedia:   item.MaxReferenceMedia,
			SupportsAudioOutput: item.SupportsAudioOutput,
			ReferenceMode:       referenceMode,
		})
	}
	channel.Models = append(channel.Models, poolAliasModels(channel.Models, aliases)...)
	return normalizeModelChannel(channel), true
}

// poolAliasModels copies each aliased pool model under its public name. The
// upstream model stays the gateway ID, so forwarding translates the name.
// Aliases whose target is gone, or whose name is now a pool model ID, are
// skipped: the public name is then served only by other channels, if any.
func poolAliasModels(poolModels []model.ChannelModel, aliases []model.PoolModelAlias) []model.ChannelModel {
	byID := make(map[string]model.ChannelModel, len(poolModels))
	for _, item := range poolModels {
		byID[item.Model] = item
	}
	seen := map[string]bool{}
	var out []model.ChannelModel
	for _, alias := range normalizePoolModelAliases(aliases) {
		target, ok := byID[alias.Target]
		if !alias.Enabled || !ok || seen[alias.Name] {
			continue
		}
		if _, collides := byID[alias.Name]; collides {
			continue
		}
		seen[alias.Name] = true
		item := target
		item.Model = alias.Name
		item.UpstreamModel = target.Model
		out = append(out, item)
	}
	return out
}

// normalizePoolModelAliases trims names and drops empty rows. A nil list stays
// nil so that "not sent" can be told apart from "cleared".
func normalizePoolModelAliases(aliases []model.PoolModelAlias) []model.PoolModelAlias {
	if aliases == nil {
		return nil
	}
	out := make([]model.PoolModelAlias, 0, len(aliases))
	for _, alias := range aliases {
		alias.Name = strings.TrimSpace(alias.Name)
		alias.Target = strings.TrimSpace(alias.Target)
		alias.Remark = strings.TrimSpace(alias.Remark)
		if alias.Name == "" && alias.Target == "" {
			continue
		}
		out = append(out, alias)
	}
	return out
}

// validatePoolModelAliases checks aliases that are new or changed against the
// running pool. Unchanged aliases are not re-checked, so a pool model that
// disappears later does not block saving unrelated settings (the alias is
// then simply inactive).
func validatePoolModelAliases(aliases, saved []model.PoolModelAlias, poolModels []gateway.PoolModel) error {
	poolIDs := make(map[string]bool, len(poolModels))
	for _, item := range poolModels {
		poolIDs[item.ID] = true
	}
	previous := map[string]string{}
	for _, alias := range normalizePoolModelAliases(saved) {
		previous[alias.Name] = alias.Target
	}
	names := map[string]bool{}
	for _, alias := range aliases {
		if alias.Name == "" || alias.Target == "" {
			return safeMessageError{message: "账号池模型别名的公开名和目标模型都不能为空"}
		}
		if len(alias.Name) > 128 || strings.ContainsAny(alias.Name, " \t\r\n/") {
			return safeMessageError{message: fmt.Sprintf("别名 %s 不合法：不能包含空白或斜杠，且不超过 128 个字符", alias.Name)}
		}
		if names[alias.Name] {
			return safeMessageError{message: fmt.Sprintf("别名 %s 重复", alias.Name)}
		}
		names[alias.Name] = true
		if target, ok := previous[alias.Name]; ok && target == alias.Target {
			continue
		}
		if len(poolModels) == 0 {
			return safeMessageError{message: "内置账号池未运行，无法校验别名 " + alias.Name + " 的目标模型"}
		}
		if poolIDs[alias.Name] {
			return safeMessageError{message: fmt.Sprintf("别名 %s 与账号池模型 ID 重名", alias.Name)}
		}
		if !poolIDs[alias.Target] {
			return safeMessageError{message: fmt.Sprintf("别名 %s 的目标模型 %s 不在账号池模型列表中", alias.Name, alias.Target)}
		}
	}
	return nil
}

// PoolModelOption is one pool model an alias can target.
type PoolModelOption struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Modality string `json:"modality"`
}

// PoolModelOptions lists the running pool's models for the alias editor.
func PoolModelOptions() []PoolModelOption {
	out := []PoolModelOption{}
	for _, item := range builtinPoolModels() {
		out = append(out, PoolModelOption{ID: item.ID, Name: item.Name, Modality: item.Modality})
	}
	return out
}

// isBuiltinPoolChannel reports the legacy single virtual pool id/base URL.
func isBuiltinPoolChannel(channel model.ModelChannel) bool {
	return channel.ID == BuiltinPoolChannelID || (IsBuiltinPoolBaseURL(channel.BaseURL) && !strings.HasPrefix(channel.ID, BuiltinProviderChannelPrefix))
}

// IsBuiltinPoolBaseURL reports whether a channel base URL is the in-process
// account pool.
func IsBuiltinPoolBaseURL(baseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	return err == nil && gateway.IsInternalURL(parsed.Hostname())
}

// runtimeChannels returns saved third-party channels plus flat built-in
// provider channels. A saved channel that already points at the internal pool
// URL suppresses virtual providers (legacy manual nexa.internal setup).
func runtimeChannels(saved []model.ModelChannel, aliases []model.PoolModelAlias) []model.ModelChannel {
	for _, channel := range saved {
		if isBuiltinPoolChannel(channel) || IsBuiltinProviderChannel(channel) {
			return saved
		}
	}
	private := model.PrivateSetting{PoolModelAliases: aliases}
	providers := builtinProviderChannels(private)
	if len(providers) == 0 {
		// Fallback: legacy single pool when provider split yields nothing.
		pool, ok := builtinPoolChannel(aliases)
		if !ok {
			return saved
		}
		out := make([]model.ModelChannel, 0, len(saved)+1)
		out = append(out, saved...)
		return append(out, pool)
	}
	out := make([]model.ModelChannel, 0, len(saved)+len(providers))
	out = append(out, saved...)
	return append(out, providers...)
}

// runtimeChannelsFromPrivate is the preferred entry that carries full settings.
func runtimeChannelsFromPrivate(private model.PrivateSetting) []model.ModelChannel {
	hasProvider := false
	hasLegacy := false
	saved := make([]model.ModelChannel, 0, len(private.Channels))
	for _, channel := range private.Channels {
		if IsBuiltinProviderChannel(channel) && !isLegacyBuiltinPoolChannel(channel) {
			hasProvider = true
			saved = append(saved, channel)
			continue
		}
		if isLegacyBuiltinPoolChannel(channel) || isBuiltinPoolChannel(channel) {
			hasLegacy = true
			continue // drop legacy unified pool; flat providers replace it
		}
		saved = append(saved, channel)
	}
	if hasProvider {
		return saved
	}
	providers := builtinProviderChannels(private)
	if len(providers) == 0 {
		if hasLegacy {
			pool, ok := builtinPoolChannel(private.PoolModelAliases)
			if ok {
				return append(saved, pool)
			}
		}
		return runtimeChannels(saved, private.PoolModelAliases)
	}
	return append(saved, providers...)
}

// appendBuiltinPoolDefinitions adds pool models that the admin has not yet
// managed, so they show up without any setup. Existing definitions (renamed,
// sorted or disabled by the admin) are kept as they are.
func appendBuiltinPoolDefinitions(items []model.ModelDefinition, channels []model.ModelChannel) []model.ModelDefinition {
	poolModels := make([]model.ChannelModel, 0)
	for i := range channels {
		if IsBuiltinProviderChannel(channels[i]) || isBuiltinPoolChannel(channels[i]) {
			poolModels = append(poolModels, channels[i].Models...)
		}
	}
	if len(poolModels) == 0 || len(items) == 0 {
		return items
	}
	seen := make(map[string]bool, len(items))
	sortBase := 0
	for _, item := range items {
		seen[item.ID] = true
		sortBase = max(sortBase, item.Sort+1)
	}
	for _, channelModel := range poolModels {
		if seen[channelModel.Model] {
			continue
		}
		seen[channelModel.Model] = true
		remark := BuiltinPoolChannelName
		if channelModel.UpstreamModel != "" && channelModel.UpstreamModel != channelModel.Model {
			remark = BuiltinPoolChannelName + " → " + channelModel.UpstreamModel
		}
		items = append(items, model.ModelDefinition{
			ID: channelModel.Model, Name: channelModel.Model, Modality: channelModel.Modality,
			Operations: channelModel.Operations, Enabled: true, Sort: sortBase,
			AspectRatios: channelModel.AspectRatios, ResolutionTiers: channelModel.ResolutionTiers,
			Durations: channelModel.Durations, MaxReferenceImages: channelModel.MaxReferenceImages,
			MaxReferenceVideos: channelModel.MaxReferenceVideos, MaxReferenceAudios: channelModel.MaxReferenceAudios,
			MaxReferenceMedia: channelModel.MaxReferenceMedia, SupportsAudioOutput: channelModel.SupportsAudioOutput,
			ReferenceMode: channelModel.ReferenceMode, Remark: remark,
		})
		sortBase++
	}
	return items
}

func applyImageCountLimits(items []model.ModelDefinition, channels []model.ModelChannel, poolModels []gateway.PoolModel) []model.ModelDefinition {
	poolCap := map[string]int{}
	for _, item := range poolModels {
		poolCap[item.ID] = item.MaxImagesPerRequest
	}
	pool, other := map[string]bool{}, map[string]bool{}
	for _, channel := range channels {
		if !channel.Enabled {
			continue
		}
		for _, item := range channel.Models {
			if IsBuiltinProviderChannel(channel) || isBuiltinPoolChannel(channel) {
				pool[item.Model] = true
				// An alias takes its target's cap.
				if item.UpstreamModel != "" && item.UpstreamModel != item.Model {
					if limit, ok := poolCap[item.UpstreamModel]; ok {
						poolCap[item.Model] = limit
					}
				}
			} else {
				other[item.Model] = true
			}
		}
	}
	for i := range items {
		items[i].MaxImageCount = 0
		if items[i].Modality == "image" && pool[items[i].ID] && !other[items[i].ID] {
			items[i].MaxImageCount = poolCap[items[i].ID]
		}
	}
	return items
}
