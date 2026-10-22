package service

import (
	"context"
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

// builtinPoolChannel returns the virtual channel for the running pool.
func builtinPoolChannel() (model.ModelChannel, bool) {
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
	return normalizeModelChannel(channel), true
}

// IsBuiltinPoolChannel reports whether a channel is the built-in pool.
func IsBuiltinPoolChannel(channel model.ModelChannel) bool { return isBuiltinPoolChannel(channel) }

func isBuiltinPoolChannel(channel model.ModelChannel) bool {
	return channel.ID == BuiltinPoolChannelID || IsBuiltinPoolBaseURL(channel.BaseURL)
}

// IsBuiltinPoolBaseURL reports whether a channel base URL is the in-process
// account pool.
func IsBuiltinPoolBaseURL(baseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	return err == nil && gateway.IsInternalURL(parsed.Hostname())
}

// runtimeChannels returns the saved channels plus the built-in pool. A saved
// channel that already points at the pool (the old manual
// http://nexa.internal/v1 setup) suppresses the virtual one.
func runtimeChannels(saved []model.ModelChannel) []model.ModelChannel {
	for _, channel := range saved {
		if isBuiltinPoolChannel(channel) {
			return saved
		}
	}
	pool, ok := builtinPoolChannel()
	if !ok {
		return saved
	}
	out := make([]model.ModelChannel, 0, len(saved)+1)
	out = append(out, saved...)
	return append(out, pool)
}

// preferBuiltinPool puts the pool first: for a model it serves, the pool is
// used and third-party channels remain as failover (the retry path excludes
// a failed channel and selects again).
func preferBuiltinPool(selections []ModelChannelSelection) (ModelChannelSelection, bool) {
	for _, selection := range selections {
		if isBuiltinPoolChannel(selection.Channel) {
			return selection, true
		}
	}
	return ModelChannelSelection{}, false
}

// appendBuiltinPoolDefinitions adds pool models that the admin has not yet
// managed, so they show up without any setup. Existing definitions (renamed,
// sorted or disabled by the admin) are kept as they are.
func appendBuiltinPoolDefinitions(items []model.ModelDefinition, channels []model.ModelChannel) []model.ModelDefinition {
	var pool *model.ModelChannel
	for i := range channels {
		if channels[i].ID == BuiltinPoolChannelID {
			pool = &channels[i]
			break
		}
	}
	if pool == nil || len(items) == 0 {
		return items
	}
	seen := make(map[string]bool, len(items))
	sortBase := 0
	for _, item := range items {
		seen[item.ID] = true
		sortBase = max(sortBase, item.Sort+1)
	}
	for _, channelModel := range pool.Models {
		if seen[channelModel.Model] {
			continue
		}
		seen[channelModel.Model] = true
		items = append(items, model.ModelDefinition{
			ID: channelModel.Model, Name: channelModel.Model, Modality: channelModel.Modality,
			Operations: channelModel.Operations, Enabled: true, Sort: sortBase,
			AspectRatios: channelModel.AspectRatios, ResolutionTiers: channelModel.ResolutionTiers,
			Durations: channelModel.Durations, MaxReferenceImages: channelModel.MaxReferenceImages,
			MaxReferenceVideos: channelModel.MaxReferenceVideos, MaxReferenceAudios: channelModel.MaxReferenceAudios,
			MaxReferenceMedia: channelModel.MaxReferenceMedia, SupportsAudioOutput: channelModel.SupportsAudioOutput,
			ReferenceMode: channelModel.ReferenceMode, Remark: BuiltinPoolChannelName,
		})
		sortBase++
	}
	return items
}

// applyImageCountLimits applies the pool's per-request image cap
// (max_images_per_request from route capabilities) to image models that only
// the built-in pool serves. Models also served by another enabled channel keep
// no extra limit: multi-image requests are routed there.
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
			if isBuiltinPoolChannel(channel) {
				pool[item.Model] = true
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
