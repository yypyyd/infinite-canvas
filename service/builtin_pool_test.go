package service

import (
	"testing"

	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/model"
)

func stubPoolModels(t *testing.T, items []gateway.PoolModel) {
	t.Helper()
	previous := builtinPoolModels
	builtinPoolModels = func() []gateway.PoolModel { return items }
	t.Cleanup(func() { builtinPoolModels = previous })
}

var testPoolModels = []gateway.PoolModel{
	{ID: "gpt-image-2", Name: "GPT Image 2", Modality: "image", Operations: []string{"generation", "edit"}, AspectRatios: []string{"1:1"}, MaxReferenceImages: 4},
	{ID: "kling-3", Name: "Kling 3", Modality: "video", Operations: []string{"generation"}, Durations: []int{5, 10}},
}

func TestRuntimeChannelsAddsBuiltinPoolWithoutConfig(t *testing.T) {
	stubPoolModels(t, testPoolModels)
	channels := runtimeChannels(nil)
	if len(channels) != 1 || channels[0].ID != BuiltinPoolChannelID || channels[0].BaseURL != gateway.InternalBaseURL || !channels[0].Enabled {
		t.Fatalf("channels = %#v", channels)
	}
	if len(channels[0].Models) != 2 || channels[0].Models[0].MaxReferenceImages != 4 {
		t.Fatalf("pool models = %#v", channels[0].Models)
	}
	public := normalizePublicSettingWithChannels(model.PublicSetting{}, channels)
	got := map[string]bool{}
	for _, id := range public.ModelChannel.AvailableModels {
		got[id] = true
	}
	if !got["gpt-image-2"] || !got["kling-3"] {
		t.Fatalf("pool models not listed: %v", public.ModelChannel.AvailableModels)
	}
}

func TestRuntimeChannelsWithoutPoolIsUnchanged(t *testing.T) {
	stubPoolModels(t, nil)
	saved := []model.ModelChannel{{Name: "third", BaseURL: "https://api.example.com", APIKey: "k", Enabled: true}}
	if got := runtimeChannels(saved); len(got) != 1 || got[0].Name != "third" {
		t.Fatalf("channels = %#v", got)
	}
}

func TestSavedInternalChannelSuppressesVirtualPool(t *testing.T) {
	stubPoolModels(t, testPoolModels)
	saved := []model.ModelChannel{{Name: "manual", BaseURL: "http://nexa.internal/v1", APIKey: "x", Enabled: true}}
	if got := runtimeChannels(saved); len(got) != 1 || got[0].Name != "manual" {
		t.Fatalf("duplicate pool channel: %#v", got)
	}
}

func TestBuiltinPoolIsPreferredOverThirdParty(t *testing.T) {
	stubPoolModels(t, testPoolModels)
	third := normalizeModelChannel(model.ModelChannel{Name: "third", BaseURL: "https://api.example.com", APIKey: "k", Weight: 1000, Enabled: true,
		Models: []model.ChannelModel{{Model: "gpt-image-2", Modality: "image", Operations: []string{"generation"}, MaxReferenceImages: 4}}})
	request := PricingRequest{Model: "gpt-image-2", Modality: "image", Operation: "generation"}
	selections := modelChannelSelectionsForRequest(runtimeChannels([]model.ModelChannel{third}), request)
	if len(selections) != 2 {
		t.Fatalf("selections = %d", len(selections))
	}
	pool, ok := preferBuiltinPool(selections)
	if !ok || pool.Channel.ID != BuiltinPoolChannelID {
		t.Fatalf("pool not preferred: %#v", pool)
	}
	// After the pool fails, the retry path excludes it and uses the third party.
	rest := modelChannelSelectionsExcluding(selections, []string{BuiltinPoolChannelName})
	if _, ok := preferBuiltinPool(rest); ok || len(rest) != 1 || rest[0].Channel.Name != "third" {
		t.Fatalf("failover = %#v", rest)
	}
}

func TestManagedDefinitionsGainNewPoolModels(t *testing.T) {
	stubPoolModels(t, testPoolModels)
	existing := []model.ModelDefinition{{ID: "gpt-image-2", Name: "Renamed", Modality: "image", Enabled: false, Sort: 3}}
	items := appendBuiltinPoolDefinitions(existing, runtimeChannels(nil))
	if len(items) != 2 || items[0].Name != "Renamed" || items[0].Enabled || items[1].ID != "kling-3" || !items[1].Enabled || items[1].Sort != 4 {
		t.Fatalf("definitions = %#v", items)
	}
}
