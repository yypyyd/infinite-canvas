package service

import (
	"math/rand"
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
)

// Source ordering. Every model source (a third-party channel model or the
// built-in pool) has an effective priority and weight. Selection takes the
// highest-priority group of compatible sources and picks by weight inside it;
// a failed source is excluded and the next pick runs on the rest.
//
// Third-party channels default to priority 0 and the pool to
// DefaultBuiltinPoolPriority, so the out-of-the-box order is "pool first,
// then a weighted pick among channels" — the behaviour before priorities
// became configurable.
const (
	SourcePriorityMin          = 0
	SourcePriorityMax          = 1000
	SourceWeightMin            = 1
	SourceWeightMax            = 1000
	// DefaultBuiltinPoolPriority is legacy (single virtual pool). Flat providers
	// use DefaultProviderPriority* in builtin_providers.go instead.
	DefaultBuiltinPoolPriority = 100
	DefaultBuiltinPoolWeight   = 1
)

// runtimeChannelsFor returns the channels requests are routed over: saved
// third-party channels plus flat built-in providers, with disabled sources removed.
func runtimeChannelsFor(private model.PrivateSetting) []model.ModelChannel {
	private.BuiltinProviders = migrateBuiltinProviders(private)
	return applySourceSettings(runtimeChannelsFromPrivate(private), private)
}

// applySourceSettings copies channels and drops disabled models/channels.
// Provider channels already carry priority/weight from builtinProviderChannels.
func applySourceSettings(channels []model.ModelChannel, private model.PrivateSetting) []model.ModelChannel {
	out := make([]model.ModelChannel, 0, len(channels))
	providerSettings := builtinProviderSourceMap(private.BuiltinProviders)
	for _, channel := range channels {
		if provider := BuiltinProviderOf(channel); provider != "" {
			src, ok := providerSettings[provider]
			if ok {
				enabled, priority, weight := builtinProviderDefaults(src, provider)
				if !enabled {
					continue
				}
				channel.Priority, channel.Weight, channel.Enabled = priority, weight, true
			}
		} else if isLegacyBuiltinPoolChannel(channel) {
			var ok bool
			if channel, ok = applyBuiltinPoolSource(channel, private.BuiltinPool); !ok {
				continue
			}
		}
		if !channel.Enabled && !IsBuiltinProviderChannel(channel) && !isLegacyBuiltinPoolChannel(channel) {
			// Third-party disabled channels are still listed by callers that
			// filter Enabled themselves; keep prior behaviour: include them
			// for overview, selection filters by Enabled elsewhere.
		}
		models := make([]model.ChannelModel, 0, len(channel.Models))
		for _, item := range channel.Models {
			if !item.Disabled {
				models = append(models, item)
			}
		}
		channel.Models = models
		out = append(out, channel)
	}
	return out
}

// applyBuiltinPoolSource sets the pool channel's priority, weight and
// per-model overrides. It reports false when the pool source is disabled.
func applyBuiltinPoolSource(channel model.ModelChannel, pool *model.BuiltinPoolSource) (model.ModelChannel, bool) {
	enabled, priority, weight := builtinPoolSourceDefaults(pool)
	if !enabled {
		return channel, false
	}
	channel.Priority = priority
	channel.Weight = weight
	overrides := builtinPoolModelOverrides(pool)
	models := make([]model.ChannelModel, len(channel.Models))
	for i, item := range channel.Models {
		if override, ok := overrides[item.Model]; ok {
			item.Disabled = item.Disabled || override.Disabled
			item.Priority = cloneIntPtr(override.Priority)
			item.Weight = cloneIntPtr(override.Weight)
		}
		models[i] = item
	}
	channel.Models = models
	return channel, true
}

// builtinPoolSourceDefaults resolves the pool-wide switch, priority and weight.
func builtinPoolSourceDefaults(pool *model.BuiltinPoolSource) (enabled bool, priority, weight int) {
	enabled, priority, weight = true, DefaultBuiltinPoolPriority, DefaultBuiltinPoolWeight
	if pool == nil {
		return
	}
	if pool.Enabled != nil {
		enabled = *pool.Enabled
	}
	if pool.Priority != nil {
		priority = *pool.Priority
	}
	if pool.Weight != nil {
		weight = *pool.Weight
	}
	return
}

func builtinPoolModelOverrides(pool *model.BuiltinPoolSource) map[string]model.BuiltinPoolModelSource {
	out := map[string]model.BuiltinPoolModelSource{}
	if pool == nil {
		return out
	}
	for _, item := range pool.Models {
		out[strings.TrimSpace(item.Model)] = item
	}
	return out
}

// normalizeBuiltinPoolSource clamps values and drops empty overrides. nil
// stays nil so "not sent" can be told apart from "reset".
func normalizeBuiltinPoolSource(pool *model.BuiltinPoolSource) *model.BuiltinPoolSource {
	if pool == nil {
		return nil
	}
	out := &model.BuiltinPoolSource{Enabled: pool.Enabled, Priority: clampPriorityPtr(pool.Priority), Weight: clampWeightPtr(pool.Weight)}
	seen := map[string]bool{}
	for _, item := range pool.Models {
		item.Model = strings.TrimSpace(item.Model)
		item.Priority = clampPriorityPtr(item.Priority)
		item.Weight = clampWeightPtr(item.Weight)
		if item.Model == "" || seen[item.Model] || (!item.Disabled && item.Priority == nil && item.Weight == nil) {
			continue
		}
		seen[item.Model] = true
		out.Models = append(out.Models, item)
	}
	return out
}

func clampPriority(value int) int { return min(max(value, SourcePriorityMin), SourcePriorityMax) }
func clampWeight(value int) int   { return min(max(value, SourceWeightMin), SourceWeightMax) }

func clampPriorityPtr(value *int) *int {
	if value == nil {
		return nil
	}
	v := clampPriority(*value)
	return &v
}

func clampWeightPtr(value *int) *int {
	if value == nil {
		return nil
	}
	v := clampWeight(*value)
	return &v
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	v := *value
	return &v
}

// selectionPriority is the effective priority of a selection: the model
// override, else the channel value.
func selectionPriority(selection ModelChannelSelection) int {
	if selection.Model.Priority != nil {
		return *selection.Model.Priority
	}
	return selection.Channel.Priority
}

// selectionWeight is the effective weight: the model override, else the channel value.
func selectionWeight(selection ModelChannelSelection) int {
	if selection.Model.Weight != nil {
		return max(*selection.Model.Weight, 0)
	}
	return max(selection.Channel.Weight, 0)
}

// topPriorityGroup keeps the selections that share the highest priority, in order.
func topPriorityGroup(selections []ModelChannelSelection) []ModelChannelSelection {
	if len(selections) == 0 {
		return nil
	}
	best := selectionPriority(selections[0])
	for _, selection := range selections[1:] {
		best = max(best, selectionPriority(selection))
	}
	group := make([]ModelChannelSelection, 0, len(selections))
	for _, selection := range selections {
		if selectionPriority(selection) == best {
			group = append(group, selection)
		}
	}
	return group
}

// pickChannelSelection picks from the highest-priority group by weight. hit
// maps the group's total weight to a draw in [0,total); nil uses rand.
func pickChannelSelection(selections []ModelChannelSelection, hit func(total int) int) (ModelChannelSelection, bool) {
	group := topPriorityGroup(selections)
	if len(group) == 0 {
		return ModelChannelSelection{}, false
	}
	total := 0
	for _, selection := range group {
		total += selectionWeight(selection)
	}
	if total <= 0 {
		return group[0], true
	}
	if hit == nil {
		hit = rand.Intn
	}
	draw := hit(total)
	for _, selection := range group {
		draw -= selectionWeight(selection)
		if draw < 0 {
			return selection, true
		}
	}
	return group[0], true
}
