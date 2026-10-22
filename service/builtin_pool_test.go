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
	channels := runtimeChannels(nil, nil)
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
	if got := runtimeChannels(saved, nil); len(got) != 1 || got[0].Name != "third" {
		t.Fatalf("channels = %#v", got)
	}
}

func TestSavedInternalChannelSuppressesVirtualPool(t *testing.T) {
	stubPoolModels(t, testPoolModels)
	saved := []model.ModelChannel{{Name: "manual", BaseURL: "http://nexa.internal/v1", APIKey: "x", Enabled: true}}
	if got := runtimeChannels(saved, nil); len(got) != 1 || got[0].Name != "manual" {
		t.Fatalf("duplicate pool channel: %#v", got)
	}
}

func TestBuiltinPoolIsPreferredOverThirdParty(t *testing.T) {
	stubPoolModels(t, testPoolModels)
	third := normalizeModelChannel(model.ModelChannel{Name: "third", BaseURL: "https://api.example.com", APIKey: "k", Weight: 1000, Enabled: true,
		Models: []model.ChannelModel{{Model: "gpt-image-2", Modality: "image", Operations: []string{"generation"}, MaxReferenceImages: 4}}})
	request := PricingRequest{Model: "gpt-image-2", Modality: "image", Operation: "generation"}
	selections := modelChannelSelectionsForRequest(runtimeChannels([]model.ModelChannel{third}, nil), request)
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
	items := appendBuiltinPoolDefinitions(existing, runtimeChannels(nil, nil))
	if len(items) != 2 || items[0].Name != "Renamed" || items[0].Enabled || items[1].ID != "kling-3" || !items[1].Enabled || items[1].Sort != 4 {
		t.Fatalf("definitions = %#v", items)
	}
}

func TestPoolAliasServedByPoolWithTargetCapabilities(t *testing.T) {
	stubPoolModels(t, testPoolModels)
	aliases := []model.PoolModelAlias{
		{Name: "my-image", Target: "gpt-image-2", Enabled: true},
		{Name: "off", Target: "gpt-image-2", Enabled: false},
		{Name: "gone", Target: "removed-model", Enabled: true},
		{Name: "kling-3", Target: "gpt-image-2", Enabled: true}, // collides with a pool ID
	}
	channels := runtimeChannels(nil, aliases)
	if len(channels) != 1 || len(channels[0].Models) != 3 {
		t.Fatalf("pool models = %#v", channels)
	}
	alias := channels[0].Models[2]
	if alias.Model != "my-image" || alias.UpstreamModel != "gpt-image-2" || alias.Modality != "image" || alias.MaxReferenceImages != 4 || len(alias.Operations) != 2 {
		t.Fatalf("alias = %#v", alias)
	}
	for _, item := range channels[0].Models {
		if item.Model == "kling-3" && item.UpstreamModel != "kling-3" {
			t.Fatalf("alias replaced a pool model: %#v", item)
		}
	}
	items := appendBuiltinPoolDefinitions([]model.ModelDefinition{{ID: "x", Modality: "image"}}, channels)
	if items[len(items)-1].ID != "my-image" || items[len(items)-1].Remark != BuiltinPoolChannelName+" → gpt-image-2" {
		t.Fatalf("definitions = %#v", items)
	}
}

func TestPoolAliasSharedWithThirdPartyChannel(t *testing.T) {
	stubPoolModels(t, testPoolModels)
	third := normalizeModelChannel(model.ModelChannel{Name: "third", BaseURL: "https://api.example.com", APIKey: "k", Weight: 5, Enabled: true,
		Models: []model.ChannelModel{{Model: "my-image", UpstreamModel: "vendor-image-v1", Modality: "image", Operations: []string{"generation"}}}})
	channels := runtimeChannels([]model.ModelChannel{third}, []model.PoolModelAlias{{Name: "my-image", Target: "gpt-image-2", Enabled: true}})
	selections := modelChannelSelectionsForRequest(channels, PricingRequest{Model: "my-image", Modality: "image", Operation: "generation"})
	if len(selections) != 2 {
		t.Fatalf("selections = %#v", selections)
	}
	pool, ok := preferBuiltinPool(selections)
	if !ok || pool.Model.UpstreamModel != "gpt-image-2" {
		t.Fatalf("pool selection = %#v", pool)
	}
	rest := modelChannelSelectionsExcluding(selections, []string{BuiltinPoolChannelName})
	if len(rest) != 1 || rest[0].Model.UpstreamModel != "vendor-image-v1" {
		t.Fatalf("failover = %#v", rest)
	}
	// Served by another channel too, so no pool image cap applies.
	limits := applyImageCountLimits([]model.ModelDefinition{{ID: "my-image", Modality: "image"}}, channels, []gateway.PoolModel{{ID: "gpt-image-2", MaxImagesPerRequest: 1}})
	if limits[0].MaxImageCount != 0 {
		t.Fatalf("limit = %d", limits[0].MaxImageCount)
	}
}

func TestPoolAliasInheritsImageCap(t *testing.T) {
	stubPoolModels(t, testPoolModels)
	channels := runtimeChannels(nil, []model.PoolModelAlias{{Name: "my-image", Target: "gpt-image-2", Enabled: true}})
	limits := applyImageCountLimits([]model.ModelDefinition{{ID: "my-image", Modality: "image"}}, channels, []gateway.PoolModel{{ID: "gpt-image-2", MaxImagesPerRequest: 2}})
	if limits[0].MaxImageCount != 2 {
		t.Fatalf("limit = %d", limits[0].MaxImageCount)
	}
}

func TestValidatePoolModelAliases(t *testing.T) {
	pool := testPoolModels
	ok := []model.PoolModelAlias{{Name: "my-image", Target: "gpt-image-2", Enabled: true}}
	if err := validatePoolModelAliases(ok, nil, pool); err != nil {
		t.Fatal(err)
	}
	bad := map[string][]model.PoolModelAlias{
		"missing target": {{Name: "a", Target: "nope"}},
		"collision":      {{Name: "kling-3", Target: "gpt-image-2"}},
		"duplicate":      {{Name: "a", Target: "gpt-image-2"}, {Name: "a", Target: "kling-3"}},
		"empty":          {{Name: "a"}},
		"slash":          {{Name: "a/b", Target: "gpt-image-2"}},
	}
	for name, aliases := range bad {
		if err := validatePoolModelAliases(aliases, nil, pool); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
	// A saved alias whose target later disappeared does not block saving.
	saved := []model.PoolModelAlias{{Name: "old", Target: "retired"}}
	if err := validatePoolModelAliases(saved, saved, pool); err != nil {
		t.Fatalf("unchanged alias rejected: %v", err)
	}
	if err := validatePoolModelAliases([]model.PoolModelAlias{{Name: "old", Target: "other-retired"}}, saved, pool); err == nil {
		t.Fatal("changed alias with missing target accepted")
	}
	if err := validatePoolModelAliases(ok, nil, nil); err == nil {
		t.Fatal("new alias accepted while the pool is down")
	}
}

func TestPoolAliasPricedUnderPublicName(t *testing.T) {
	stubPoolModels(t, []gateway.PoolModel{{ID: "gpt-image-2", Modality: "image", Operations: []string{"generation"}, ResolutionTiers: []string{"1k"},
		Routes: []gateway.PoolRoute{{Provider: "chatgpt", Operations: []string{"generation"}, ResolutionTiers: []string{"1k"}}}}})
	gaps := poolPricingGaps(nil, []model.PoolModelAlias{{Name: "my-image", Target: "gpt-image-2", Enabled: true}})
	found := false
	for _, gap := range gaps {
		if gap.Model == "my-image" {
			found = len(gap.Providers) == 1 && gap.Providers[0] == "chatgpt"
		}
	}
	if !found {
		t.Fatalf("gaps = %#v", gaps)
	}
}
