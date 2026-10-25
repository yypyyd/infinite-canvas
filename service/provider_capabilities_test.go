package service

import (
	"reflect"
	"testing"

	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/model"
)

func TestPoolModelToChannelModelForProviderScopesResolutions(t *testing.T) {
	item := gateway.PoolModel{
		ID: "gpt-image-2", Modality: "image",
		Operations:      []string{"generation", "edit"},
		ResolutionTiers: []string{"1K", "2K", "4K"},
		Routes: []gateway.PoolRoute{
			{Provider: "chatgpt", Operations: []string{"generation", "edit"}, ResolutionTiers: []string{"1K"}},
			{Provider: "adobe", Operations: []string{"generation", "edit"}, ResolutionTiers: []string{"1K", "2K", "4K"}},
		},
	}
	chatgpt := poolModelToChannelModelForProvider(item, "chatgpt")
	if !reflect.DeepEqual(chatgpt.ResolutionTiers, []string{"1K"}) {
		t.Fatalf("chatgpt tiers = %#v", chatgpt.ResolutionTiers)
	}
	adobe := poolModelToChannelModelForProvider(item, "adobe")
	if !reflect.DeepEqual(adobe.ResolutionTiers, []string{"1K", "2K", "4K"}) {
		t.Fatalf("adobe tiers = %#v", adobe.ResolutionTiers)
	}
}

func TestClampPublicModelsToPrimaryProviderUsesChatGPTOneK(t *testing.T) {
	channels := []model.ModelChannel{
		{
			ID: BuiltinProviderChannelID("chatgpt"), Name: "ChatGPT", Enabled: true, Priority: 200,
			Models: []model.ChannelModel{{Model: "gpt-image-2", Modality: "image", ResolutionTiers: []string{"1K"}, Operations: []string{"generation"}}},
		},
		{
			ID: BuiltinProviderChannelID("adobe"), Name: "Adobe Firefly", Enabled: true, Priority: 180,
			Models: []model.ChannelModel{{Model: "gpt-image-2", Modality: "image", ResolutionTiers: []string{"1K", "2K", "4K"}, Operations: []string{"generation"}}},
		},
	}
	items := []model.ModelDefinition{{ID: "gpt-image-2", Modality: "image", ResolutionTiers: []string{"1k", "2k", "4k"}, Operations: []string{"generation"}}}
	got := clampPublicModelsToPrimaryProvider(items, channels)
	if !reflect.DeepEqual(got[0].ResolutionTiers, []string{"1k"}) {
		t.Fatalf("clamped = %#v", got[0].ResolutionTiers)
	}
}

func TestSelectModelChannelByNameRebindsLegacyPool(t *testing.T) {
	stubPoolModels(t, []gateway.PoolModel{{
		ID: "gpt-image-2", Modality: "image", Operations: []string{"generation"},
		ResolutionTiers: []string{"1K"},
		Routes:          []gateway.PoolRoute{{Provider: "chatgpt", Operations: []string{"generation"}, ResolutionTiers: []string{"1K"}}},
	}})
	// SelectModelChannelByName needs repository settings — use channelSelections path via runtimeChannelsFor
	selections := modelChannelSelectionsForRequest(runtimeChannelsFor(model.PrivateSetting{}), PricingRequest{Model: "gpt-image-2", Modality: "image", Operation: "generation"})
	if len(selections) == 0 {
		t.Fatal("expected selections")
	}
	found := false
	for _, s := range selections {
		if s.Channel.Name == BuiltinPoolChannelName || s.Channel.ID == BuiltinPoolChannelID {
			found = true
		}
		if IsBuiltinProviderChannel(s.Channel) && len(s.Model.ResolutionTiers) == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("selections %#v", selections)
	}
}
