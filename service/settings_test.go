package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/basketikun/infinite-canvas/model"
)

func TestNormalizeOperationsAlertSettingsUsesDefaultsAndAllowsDisabling(t *testing.T) {
	setting := normalizePrivateSetting(model.PrivateSetting{}).OperationsAlerts
	if setting.Enabled == nil || !*setting.Enabled || setting.BatchQueuedThreshold == nil || *setting.BatchQueuedThreshold != 100 || setting.EmailFailedThreshold == nil || *setting.EmailFailedThreshold != 1 {
		t.Fatalf("unexpected default operations alerts: %#v", setting)
	}
	negative := int64(-1)
	setting = normalizePrivateSetting(model.PrivateSetting{OperationsAlerts: model.OperationsAlertSetting{EmailFailedThreshold: &negative}}).OperationsAlerts
	if setting.EmailFailedThreshold == nil || *setting.EmailFailedThreshold != 0 {
		t.Fatalf("negative threshold was not disabled: %#v", setting)
	}
	publicJSON, err := json.Marshal(model.PublicSetting{})
	if err != nil || strings.Contains(string(publicJSON), "operationsAlerts") {
		t.Fatalf("public settings exposed operations alerts: %s, err=%v", publicJSON, err)
	}
}

func TestFetchAdminChannelModelsParsesOpenAIModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("extended") != "true" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"z-model","kind":"text"},{"id":"a-model","kind":"video","supported_ratios":["16:9"],"supported_resolutions":["1280x720","1080p"],"supported_durations":["10s",5,"5s","invalid"],"max_reference_images":2,"reference_mode":"frame"},{"id":""}]}`))
	}))
	defer server.Close()

	models, err := fetchAdminChannelModels(model.ModelChannel{
		BaseURL: server.URL,
		APIKey:  "test-key",
	})
	if err != nil {
		t.Fatalf("fetchAdminChannelModels returned error: %v", err)
	}
	if want := []model.DiscoveredModel{
		{ID: "a-model", Kind: "video", Modality: "video", SupportedRatios: []string{"16:9"}, SupportedResolutions: []string{"720p", "1080p"}, SupportedDurations: []int{5, 10}, MaxReferenceImages: 2, ReferenceMode: "frame", ReferenceCapabilityProvided: true},
		{ID: "z-model", Kind: "text", Modality: "text", SupportedRatios: []string{}, SupportedResolutions: []string{}, SupportedDurations: []int{}, ReferenceMode: "none"},
	}; !reflect.DeepEqual(models, want) {
		t.Fatalf("models = %#v, want %#v", models, want)
	}
}

func TestNormalizeSettingsPublishesEnabledChannelModelsAndRepairsDefaults(t *testing.T) {
	settings := normalizeSettings(model.Settings{
		Public: model.PublicSetting{
			ModelChannel: model.PublicModelChannelSetting{
				AvailableModels:   []string{"grok-imagine-video", "disabled-model"},
				DefaultModel:      "grok-imagine-video",
				DefaultTextModel:  "missing-text",
				DefaultImageModel: "missing-image",
				DefaultVideoModel: "missing-video",
			},
		},
		Private: model.PrivateSetting{
			Channels: []model.ModelChannel{
				{Enabled: true, Models: channelModels("gpt-5.5", "doubao-seedream-5.0-lite", "doubao-seedance-2.0-fast", "gpt-5.5")},
				{Enabled: false, Models: channelModels("disabled-model")},
			},
		},
	})

	channel := settings.Public.ModelChannel
	wantModels := []string{"gpt-5.5", "doubao-seedream-5.0-lite", "doubao-seedance-2.0-fast"}
	if !reflect.DeepEqual(channel.AvailableModels, wantModels) {
		t.Fatalf("available models = %#v, want %#v", channel.AvailableModels, wantModels)
	}
	if channel.DefaultModel != "gpt-5.5" {
		t.Fatalf("default model = %q, want text model", channel.DefaultModel)
	}
	if channel.DefaultTextModel != "gpt-5.5" {
		t.Fatalf("default text model = %q, want text model", channel.DefaultTextModel)
	}
	if channel.DefaultImageModel != "doubao-seedream-5.0-lite" {
		t.Fatalf("default image model = %q, want seedream", channel.DefaultImageModel)
	}
	if channel.DefaultVideoModel != "doubao-seedance-2.0-fast" {
		t.Fatalf("default video model = %q, want seedance", channel.DefaultVideoModel)
	}
}

func TestPublicAPIModelsReturnsOnlyEnabledNonTextModels(t *testing.T) {
	items := publicAPIModels(model.PublicModelChannelSetting{
		AvailableModels: []string{"text-model", "image-model", "video-model", "audio-model"},
		Models: []model.ModelDefinition{
			{ID: "text-model", Name: "Text", Modality: "text", Enabled: true},
			{ID: "image-model", Name: "Image", Modality: "image", Operations: []string{"generation", "edit"}, ResolutionTiers: []string{"1k", "2k"}, Enabled: true},
			{ID: "video-model", Name: "Video", Modality: "video", Operations: []string{"generation"}, ResolutionTiers: []string{"720p"}, Durations: []int{5, 10}, MaxReferenceImages: 2, ReferenceMode: "frame", Enabled: true},
			{ID: "audio-model", Name: "Audio", Modality: "audio", Operations: []string{"speech"}, Enabled: false},
		},
	})

	if len(items) != 2 || items[0].ID != "image-model" || items[0].Object != "model" || items[0].OwnedBy != "infinite-canvas" || items[1].ID != "video-model" || !reflect.DeepEqual(items[1].Durations, []int{5, 10}) || items[1].MaxReferenceImages != 2 || items[1].ReferenceMode != "frame" {
		t.Fatalf("public API models = %#v", items)
	}
}

func TestNormalizeSettingsAddsNewChannelModelsToExistingCatalog(t *testing.T) {
	settings := normalizeSettings(model.Settings{
		Public: model.PublicSetting{ModelChannel: model.PublicModelChannelSetting{Models: []model.ModelDefinition{
			{ID: "gpt-5.5", Name: "GPT 5.5", Modality: "text", Enabled: false},
		}}},
		Private: model.PrivateSetting{Channels: []model.ModelChannel{
			{Enabled: true, Models: channelModels("gpt-5.5", "gpt-image-2", "firefly-video")},
		}},
	})

	models := settings.Public.ModelChannel.Models
	if len(models) != 3 || models[0].ID != "gpt-5.5" || models[0].Enabled || models[1].ID != "gpt-image-2" || !models[1].Enabled || models[2].ID != "firefly-video" || !reflect.DeepEqual(models[2].ResolutionTiers, []string{"720p"}) {
		t.Fatalf("model catalog = %#v", models)
	}
	if want := []string{"gpt-image-2", "firefly-video"}; !reflect.DeepEqual(settings.Public.ModelChannel.AvailableModels, want) {
		t.Fatalf("available models = %#v, want %#v", settings.Public.ModelChannel.AvailableModels, want)
	}
	if len(settings.Public.ModelChannel.PricingRules) != 0 {
		t.Fatalf("channel sync added pricing rules: %#v", settings.Public.ModelChannel.PricingRules)
	}
}

func TestNormalizeSettingsMergesEnabledChannelCapabilitiesIntoCatalog(t *testing.T) {
	settings := normalizeSettings(model.Settings{
		Public: model.PublicSetting{ModelChannel: model.PublicModelChannelSetting{Models: []model.ModelDefinition{{
			ID: "firefly-ray", Name: "Firefly Ray", Modality: "video", Enabled: true, Sort: 7,
			AspectRatios: []string{"4:3"}, ResolutionTiers: []string{"480p"}, Durations: []int{4}, Remark: "public model",
		}}}},
		Private: model.PrivateSetting{Channels: []model.ModelChannel{
			{Enabled: true, Models: []model.ChannelModel{{Model: "firefly-ray", Modality: "video", AspectRatios: []string{"16:9", "21:9"}, ResolutionTiers: []string{"720p"}, Durations: []int{5}, MaxReferenceImages: 2, ReferenceMode: "frame"}}},
			{Enabled: true, Models: []model.ChannelModel{{Model: "firefly-ray", Modality: "video", AspectRatios: []string{"9:16", "16:9"}, ResolutionTiers: []string{"1080p"}, Durations: []int{10, 5}, MaxReferenceImages: 6, ReferenceMode: "asset"}}},
			{Enabled: true, Models: []model.ChannelModel{{Model: "firefly-ray", Modality: "image", Operations: []string{"edit"}, AspectRatios: []string{"3:2"}, ResolutionTiers: []string{"2k"}}}},
			{Enabled: false, Models: []model.ChannelModel{{Model: "firefly-ray", Modality: "video", AspectRatios: []string{"1:1"}, ResolutionTiers: []string{"480p"}, Durations: []int{20}}}},
		}},
	})

	got := settings.Public.ModelChannel.Models[0]
	if got.Name != "Firefly Ray" || got.Modality != "video" || !reflect.DeepEqual(got.Operations, []string{"generation"}) || got.Sort != 7 || got.Remark != "public model" || !got.Enabled {
		t.Fatalf("model metadata changed: %#v", got)
	}
	if want := []string{"16:9", "21:9", "9:16"}; !reflect.DeepEqual(got.AspectRatios, want) {
		t.Fatalf("aspect ratios = %#v, want %#v", got.AspectRatios, want)
	}
	if want := []string{"720p", "1080p"}; !reflect.DeepEqual(got.ResolutionTiers, want) {
		t.Fatalf("resolution tiers = %#v, want %#v", got.ResolutionTiers, want)
	}
	if want := []int{5, 10}; !reflect.DeepEqual(got.Durations, want) {
		t.Fatalf("durations = %#v, want %#v", got.Durations, want)
	}
	if got.MaxReferenceImages != 6 || got.ReferenceMode != "asset" {
		t.Fatalf("reference capability = %d/%q, want 6/asset", got.MaxReferenceImages, got.ReferenceMode)
	}
}

func TestChannelModelSupportsReferenceImageLimit(t *testing.T) {
	item := model.ChannelModel{Model: "video-model", Modality: "video", Operations: []string{"generation"}, Durations: []int{5}, MaxReferenceImages: 6, ReferenceMode: "frame"}
	if !channelModelSupportsRequest(item, PricingRequest{Model: "video-model", Modality: "video", Operation: "generation", Quantity: 5, ReferenceImages: 6}) {
		t.Fatal("expected six reference images to be supported")
	}
	if channelModelSupportsRequest(item, PricingRequest{Model: "video-model", Modality: "video", Operation: "generation", Quantity: 5, ReferenceImages: 7}) {
		t.Fatal("expected seven reference images to be rejected")
	}
}

func TestNormalizeSettingsKeepsOnlyExplicitPricingRules(t *testing.T) {
	settings := normalizeSettings(model.Settings{
		Public: model.PublicSetting{
			ModelChannel: model.PublicModelChannelSetting{
				Models: []model.ModelDefinition{{ID: "gpt-image-2", Modality: "image", Enabled: true}, {ID: "firefly-video", Modality: "video", Enabled: true}},
				PricingRules: []model.PricingRule{
					{Model: "gpt-image-2", Modality: "IMAGE", Operation: "generation", Unit: "image", ResolutionTier: "2K", BillingMode: "fixed", Credits: 4, Enabled: true},
					{Model: "firefly-video", Modality: "video", Operation: "generation", Unit: "second", BillingMode: "fixed", Credits: 1, Enabled: true},
				},
			},
		},
	})

	rules := settings.Public.ModelChannel.PricingRules
	if len(rules) != 1 || rules[0].Model != "gpt-image-2" || rules[0].Modality != "image" || rules[0].ResolutionTier != "2k" || rules[0].Credits != 4 {
		t.Fatalf("pricing rules = %#v, want only normalized explicit rule", rules)
	}
}

func TestDefaultModelOperationsIncludesOfficialEditableModels(t *testing.T) {
	for _, modelName := range []string{"flux-klein-2", "firefly-image-5"} {
		if got := defaultModelOperations(modelName, "image"); !reflect.DeepEqual(got, []string{"generation", "edit"}) {
			t.Fatalf("defaultModelOperations(%q) = %#v", modelName, got)
		}
	}
}

func channelModels(names ...string) []model.ChannelModel {
	result := make([]model.ChannelModel, 0, len(names))
	for _, name := range names {
		result = append(result, model.ChannelModel{Model: name, UpstreamModel: name})
	}
	return result
}
