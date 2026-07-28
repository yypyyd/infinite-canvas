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
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"z-model","kind":"text"},{"id":"a-model","kind":"video","supported_ratios":["16:9"],"supported_resolutions":["1280x720","1080p"],"supported_durations":[10,5,10]},{"id":""}]}`))
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
		{ID: "a-model", Kind: "video", Modality: "video", SupportedRatios: []string{"16:9"}, SupportedResolutions: []string{"720p", "1080p"}, SupportedDurations: []int{5, 10}},
		{ID: "z-model", Kind: "text", Modality: "text", SupportedRatios: []string{}, SupportedResolutions: []string{}, SupportedDurations: []int{}},
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
	assertHasPricingRule(t, settings.Public.ModelChannel.PricingRules, "gpt-image-2", "image", "generation", "image", "")
}

func TestNormalizeSettingsAppendsDefaultPricingRulesForEnabledModels(t *testing.T) {
	settings := normalizeSettings(model.Settings{
		Public: model.PublicSetting{
			ModelChannel: model.PublicModelChannelSetting{
				Models: []model.ModelDefinition{
					{ID: "gpt-5.5", Modality: "text", Enabled: true},
					{ID: "gpt-image-2", Modality: "image", Enabled: true},
					{ID: "dall-e-3", Modality: "image", Enabled: true},
					{ID: "custom-image", Modality: "image", Operations: []string{"generation", "edit"}, Enabled: true},
					{ID: "firefly-video", Modality: "video", Enabled: true},
					{ID: "tts-1", Modality: "audio", Enabled: true},
					{ID: "disabled-video", Modality: "video", Enabled: false},
				},
			},
		},
	})

	rules := settings.Public.ModelChannel.PricingRules
	assertHasPricingRule(t, rules, "gpt-5.5", "text", "completion", "request", "")
	assertHasPricingRule(t, rules, "gpt-image-2", "image", "generation", "image", "")
	assertHasPricingRule(t, rules, "gpt-image-2", "image", "edit", "image", "")
	assertHasPricingRule(t, rules, "dall-e-3", "image", "generation", "image", "")
	assertNoPricingRule(t, rules, "dall-e-3", "image", "edit", "image", "")
	assertHasPricingRule(t, rules, "custom-image", "image", "generation", "image", "")
	assertHasPricingRule(t, rules, "custom-image", "image", "edit", "image", "")
	assertHasPricingRule(t, rules, "firefly-video", "video", "generation", "second", "")
	assertHasPricingRule(t, rules, "tts-1", "audio", "speech", "request", "")
	assertNoPricingRule(t, rules, "disabled-video", "video", "generation", "second", "")
}

func TestNormalizeSettingsKeepsExistingFallbackPricingRules(t *testing.T) {
	existing := model.PricingRule{
		Model:          "firefly-video",
		Modality:       "video",
		Operation:      "generation",
		Unit:           "second",
		BillingMode:    "fixed",
		Credits:        3,
		Enabled:        true,
		ResolutionTier: "",
	}
	settings := normalizeSettings(model.Settings{
		Public: model.PublicSetting{
			ModelChannel: model.PublicModelChannelSetting{
				Models: []model.ModelDefinition{
					{ID: "firefly-video", Modality: "video", Enabled: true},
				},
				PricingRules: []model.PricingRule{existing},
			},
		},
	})

	matches := 0
	for _, rule := range settings.Public.ModelChannel.PricingRules {
		if rule.Model == "firefly-video" && rule.Modality == "video" && rule.Operation == "generation" && rule.Unit == "second" && rule.ResolutionTier == "" {
			matches++
			if rule.Credits != 3 {
				t.Fatalf("existing fallback credits = %d, want 3", rule.Credits)
			}
		}
	}
	if matches != 1 {
		t.Fatalf("fallback rule count = %d, want 1", matches)
	}
}

func TestNormalizeSettingsAddsFallbackBesideResolutionPricingRule(t *testing.T) {
	settings := normalizeSettings(model.Settings{
		Public: model.PublicSetting{
			ModelChannel: model.PublicModelChannelSetting{
				Models: []model.ModelDefinition{
					{ID: "firefly-image-5", Modality: "image", Enabled: true},
				},
				PricingRules: []model.PricingRule{{
					Model:          "firefly-image-5",
					Modality:       "image",
					Operation:      "generation",
					Unit:           "image",
					ResolutionTier: "2K",
					BillingMode:    "fixed",
					Credits:        4,
					Enabled:        true,
				}},
			},
		},
	})

	rules := settings.Public.ModelChannel.PricingRules
	assertHasPricingRule(t, rules, "firefly-image-5", "image", "generation", "image", "2k")
	assertHasPricingRule(t, rules, "firefly-image-5", "image", "generation", "image", "")
	assertHasPricingRule(t, rules, "firefly-image-5", "image", "edit", "image", "")
}

func TestDefaultModelOperationsIncludesOfficialEditableModels(t *testing.T) {
	for _, modelName := range []string{"flux-klein-2", "firefly-image-5"} {
		if got := defaultModelOperations(modelName, "image"); !reflect.DeepEqual(got, []string{"generation", "edit"}) {
			t.Fatalf("defaultModelOperations(%q) = %#v", modelName, got)
		}
	}
}

func assertHasPricingRule(t *testing.T, rules []model.PricingRule, modelName string, modality string, operation string, unit string, resolutionTier string) {
	t.Helper()
	for _, rule := range rules {
		if rule.Model == modelName &&
			rule.Modality == modality &&
			rule.Operation == operation &&
			rule.Unit == unit &&
			rule.ResolutionTier == resolutionTier &&
			rule.Enabled {
			return
		}
	}
	t.Fatalf("missing pricing rule model=%s modality=%s operation=%s unit=%s resolution=%s in %#v", modelName, modality, operation, unit, resolutionTier, rules)
}

func assertNoPricingRule(t *testing.T, rules []model.PricingRule, modelName string, modality string, operation string, unit string, resolutionTier string) {
	t.Helper()
	for _, rule := range rules {
		if rule.Model == modelName &&
			rule.Modality == modality &&
			rule.Operation == operation &&
			rule.Unit == unit &&
			rule.ResolutionTier == resolutionTier &&
			rule.Enabled {
			t.Fatalf("unexpected pricing rule model=%s modality=%s operation=%s unit=%s resolution=%s", modelName, modality, operation, unit, resolutionTier)
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
