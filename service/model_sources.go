package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

// OptionalInt tells an absent JSON field (Set=false) from null (Set=true,
// Value=nil, meaning "clear the override") and a number.
type OptionalInt struct {
	Set   bool
	Value *int
}

func (o *OptionalInt) UnmarshalJSON(data []byte) error {
	o.Set = true
	if strings.TrimSpace(string(data)) == "null" {
		o.Value = nil
		return nil
	}
	var v int
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// ModelSourcePatch changes one model source.
//
//	kind=channel, channelId           → channel-wide enabled/priority/weight
//	kind=channel, channelId, model    → that channel model (null clears an override)
//	kind=pool                         → pool-wide enabled/priority/weight (null resets to default)
//	kind=pool, model                  → pool source of one public model
//	kind=pool-route, logicalModelId, routeId → one gateway route (null inherits the provider value)
type ModelSourcePatch struct {
	Kind           string      `json:"kind"`
	ChannelID      string      `json:"channelId"`
	Provider       string      `json:"provider"`
	Model          string      `json:"model"`
	LogicalModelID string      `json:"logicalModelId"`
	RouteID        string      `json:"routeId"`
	Enabled        *bool       `json:"enabled"`
	Priority       OptionalInt `json:"priority"`
	Weight         OptionalInt `json:"weight"`
}

// updatePoolRoute is replaceable in tests.
var updatePoolRoute = func(logicalModelID, routeID string, update gateway.RouteUpdate) error {
	m := gateway.Current()
	if m == nil {
		return safeMessageError{message: "账号池未运行"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return m.UpdateRoute(ctx, logicalModelID, routeID, update)
}

func sourceRangeError(patch ModelSourcePatch) error {
	if patch.Priority.Value != nil && (*patch.Priority.Value < SourcePriorityMin || *patch.Priority.Value > SourcePriorityMax) {
		return safeMessageError{message: fmt.Sprintf("优先级需在 %d 到 %d 之间", SourcePriorityMin, SourcePriorityMax)}
	}
	if patch.Weight.Value != nil && (*patch.Weight.Value < SourceWeightMin || *patch.Weight.Value > SourceWeightMax) {
		return safeMessageError{message: fmt.Sprintf("权重需在 %d 到 %d 之间", SourceWeightMin, SourceWeightMax)}
	}
	return nil
}

// PatchModelSource applies one source change and saves it.
func PatchModelSource(patch ModelSourcePatch) error {
	if err := sourceRangeError(patch); err != nil {
		return err
	}
	patch.Model = strings.TrimSpace(patch.Model)
	if patch.Kind == "pool-route" {
		if patch.LogicalModelID == "" || patch.RouteID == "" {
			return safeMessageError{message: "缺少路由"}
		}
		update := gateway.RouteUpdate{Enabled: patch.Enabled}
		if patch.Priority.Set {
			update.Priority, update.ClearPriority = patch.Priority.Value, patch.Priority.Value == nil
		}
		if patch.Weight.Set {
			update.Weight, update.ClearWeight = patch.Weight.Value, patch.Weight.Value == nil
		}
		return updatePoolRoute(patch.LogicalModelID, patch.RouteID, update)
	}
	settings, err := repository.GetSettings()
	if err != nil {
		return err
	}
	switch patch.Kind {
	case "channel":
		if err := patchChannelSource(&settings.Private, patch); err != nil {
			return err
		}
	case "pool":
		settings.Private.BuiltinPool = patchPoolSource(settings.Private.BuiltinPool, patch)
		settings.Private.BuiltinProviders = patchBuiltinProvidersFromPool(settings.Private.BuiltinProviders, settings.Private.BuiltinPool, patch)
	case "provider":
		settings.Private.BuiltinProviders = patchOneBuiltinProvider(settings.Private.BuiltinProviders, patch)
	default:
		return safeMessageError{message: "未知的来源类型"}
	}
	_, err = SaveSettings(settings)
	return err
}

func patchChannelSource(private *model.PrivateSetting, patch ModelSourcePatch) error {
	for i := range private.Channels {
		channel := &private.Channels[i]
		if channel.ID == "" || channel.ID != patch.ChannelID {
			continue
		}
		if patch.Model == "" {
			if patch.Enabled != nil {
				channel.Enabled = *patch.Enabled
			}
			if patch.Priority.Set {
				channel.Priority = valueOr(patch.Priority.Value, 0)
			}
			if patch.Weight.Set {
				channel.Weight = valueOr(patch.Weight.Value, 1)
			}
			return nil
		}
		for j := range channel.Models {
			item := &channel.Models[j]
			if strings.TrimSpace(item.Model) != patch.Model {
				continue
			}
			if patch.Enabled != nil {
				item.Disabled = !*patch.Enabled
			}
			if patch.Priority.Set {
				item.Priority = cloneIntPtr(patch.Priority.Value)
			}
			if patch.Weight.Set {
				item.Weight = cloneIntPtr(patch.Weight.Value)
			}
			return nil
		}
		return safeMessageError{message: "渠道中没有模型 " + patch.Model}
	}
	return safeMessageError{message: "渠道不存在"}
}


// patchBuiltinProvidersFromPool keeps flat provider settings aligned with legacy
// pool patches so selection and overview see the same priority/enable changes.

func patchOneBuiltinProvider(providers []model.BuiltinProviderSource, patch ModelSourcePatch) []model.BuiltinProviderSource {
	provider := normalizeBuiltinProvider(patch.Provider)
	if provider == "" {
		provider = BuiltinProviderOf(model.ModelChannel{ID: patch.ChannelID})
	}
	if provider == "" {
		return providers
	}
	if len(providers) == 0 {
		providers = migrateBuiltinProviders(model.PrivateSetting{})
	}
	found := false
	for i := range providers {
		if normalizeBuiltinProvider(providers[i].Provider) != provider {
			continue
		}
		found = true
		providers[i] = patchProviderSource(providers[i], patch)
	}
	if !found {
		providers = append(providers, patchProviderSource(model.BuiltinProviderSource{Provider: provider}, patch))
	}
	return normalizeBuiltinProviders(providers)
}

func patchProviderSource(src model.BuiltinProviderSource, patch ModelSourcePatch) model.BuiltinProviderSource {
	if patch.Model == "" {
		if patch.Enabled != nil {
			enabled := *patch.Enabled
			src.Enabled = &enabled
		}
		if patch.Priority.Set {
			src.Priority = cloneIntPtr(patch.Priority.Value)
		}
		if patch.Weight.Set {
			src.Weight = cloneIntPtr(patch.Weight.Value)
		}
		return src
	}
	index := -1
	for i, item := range src.Models {
		if strings.TrimSpace(item.Model) == patch.Model {
			index = i
		}
	}
	if index < 0 {
		src.Models = append(src.Models, model.BuiltinPoolModelSource{Model: patch.Model})
		index = len(src.Models) - 1
	}
	item := &src.Models[index]
	if patch.Enabled != nil {
		item.Disabled = !*patch.Enabled
	}
	if patch.Priority.Set {
		item.Priority = cloneIntPtr(patch.Priority.Value)
	}
	if patch.Weight.Set {
		item.Weight = cloneIntPtr(patch.Weight.Value)
	}
	return src
}

func patchBuiltinProvidersFromPool(providers []model.BuiltinProviderSource, pool *model.BuiltinPoolSource, patch ModelSourcePatch) []model.BuiltinProviderSource {
	if len(providers) == 0 {
		providers = migrateBuiltinProviders(model.PrivateSetting{BuiltinPool: pool})
	}
	out := make([]model.BuiltinProviderSource, len(providers))
	copy(out, providers)
	for i := range out {
		if patch.Model == "" {
			if patch.Enabled != nil {
				enabled := *patch.Enabled
				out[i].Enabled = &enabled
			}
			if patch.Priority.Set {
				out[i].Priority = cloneIntPtr(patch.Priority.Value)
			}
			if patch.Weight.Set {
				out[i].Weight = cloneIntPtr(patch.Weight.Value)
			}
			continue
		}
		index := -1
		for j, item := range out[i].Models {
			if strings.TrimSpace(item.Model) == patch.Model {
				index = j
			}
		}
		if index < 0 {
			out[i].Models = append(out[i].Models, model.BuiltinPoolModelSource{Model: patch.Model})
			index = len(out[i].Models) - 1
		}
		item := &out[i].Models[index]
		if patch.Enabled != nil {
			item.Disabled = !*patch.Enabled
		}
		if patch.Priority.Set {
			item.Priority = cloneIntPtr(patch.Priority.Value)
		}
		if patch.Weight.Set {
			item.Weight = cloneIntPtr(patch.Weight.Value)
		}
	}
	return normalizeBuiltinProviders(out)
}

func patchPoolSource(pool *model.BuiltinPoolSource, patch ModelSourcePatch) *model.BuiltinPoolSource {
	next := model.BuiltinPoolSource{}
	if pool != nil {
		next = *pool
		next.Models = append([]model.BuiltinPoolModelSource(nil), pool.Models...)
	}
	if patch.Model == "" {
		if patch.Enabled != nil {
			enabled := *patch.Enabled
			next.Enabled = &enabled
		}
		if patch.Priority.Set {
			next.Priority = cloneIntPtr(patch.Priority.Value)
		}
		if patch.Weight.Set {
			next.Weight = cloneIntPtr(patch.Weight.Value)
		}
		return &next
	}
	index := -1
	for i, item := range next.Models {
		if strings.TrimSpace(item.Model) == patch.Model {
			index = i
		}
	}
	if index < 0 {
		next.Models = append(next.Models, model.BuiltinPoolModelSource{Model: patch.Model})
		index = len(next.Models) - 1
	}
	item := &next.Models[index]
	if patch.Enabled != nil {
		item.Disabled = !*patch.Enabled
	}
	if patch.Priority.Set {
		item.Priority = cloneIntPtr(patch.Priority.Value)
	}
	if patch.Weight.Set {
		item.Weight = cloneIntPtr(patch.Weight.Value)
	}
	return &next
}

func valueOr(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}
