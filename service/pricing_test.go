package service

import (
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestNormalizePricingRequestImageTier(t *testing.T) {
	request := normalizePricingRequest(PricingRequest{
		Model:     "gpt-image-2",
		Modality:  "image",
		Operation: "generation",
		Unit:      "image",
		Size:      "2880x1620",
		Quantity:  2,
	})

	if request.ResolutionTier != "4k" {
		t.Fatalf("resolution tier = %q, want 4k", request.ResolutionTier)
	}
}

func TestNormalizePricingRequestOneKPortraitTier(t *testing.T) {
	request := normalizePricingRequest(PricingRequest{
		Model:     "gpt-image-2",
		Modality:  "image",
		Operation: "generation",
		Unit:      "image",
		Size:      "1024x1824",
		Quantity:  1,
	})

	if request.ResolutionTier != "1k" {
		t.Fatalf("resolution tier = %q, want 1k", request.ResolutionTier)
	}
}

func TestNormalizePricingRequestDefaultsImageTierWithoutQuality(t *testing.T) {
	request := normalizePricingRequest(PricingRequest{
		Model:     "gpt-image-2",
		Modality:  "image",
		Operation: "generation",
		Unit:      "image",
		Quantity:  1,
	})

	if request.ResolutionTier != "1k" {
		t.Fatalf("resolution tier = %q, want default 1k", request.ResolutionTier)
	}
}

func TestSelectVideoPricingRuleMatchesResolutionAndBillsBySeconds(t *testing.T) {
	request := normalizePricingRequest(PricingRequest{
		Model:      "video-model",
		Modality:   "video",
		Operation:  "generation",
		Unit:       "second",
		Resolution: "1080p",
		Quantity:   6,
	})
	rules := normalizePricingRules([]model.PricingRule{
		{Model: "video-model", Modality: "video", Operation: "generation", Unit: "second", Credits: 1, Enabled: true},
		{Model: "video-model", Modality: "video", Operation: "generation", Unit: "second", ResolutionTier: "1080p", Credits: 3, Enabled: true},
	})

	rule, ok := selectPricingRule(rules, request)
	if !ok {
		t.Fatal("expected matching rule")
	}
	if rule.Credits != 3 {
		t.Fatalf("credits = %d, want resolution rule credits 3", rule.Credits)
	}
	if credits := calculateRuleCredits(rule, request.Quantity, 1); credits != 18 {
		t.Fatalf("credits = %d, want 3 credits/second * 6 seconds", credits)
	}
}

func TestSelectPricingRuleDoesNotUseBlankResolutionFallback(t *testing.T) {
	request := normalizePricingRequest(PricingRequest{Model: "video-model", Modality: "video", Operation: "generation", Unit: "second", Resolution: "1080p", Quantity: 6})
	rules := normalizePricingRules([]model.PricingRule{{Model: "video-model", Modality: "video", Operation: "generation", Unit: "second", Credits: 1, Enabled: true}})
	if _, ok := selectPricingRule(rules, request); ok {
		t.Fatal("expected blank resolution rule not to match video request")
	}
}

func TestSelectPricingRuleIgnoresImageQuality(t *testing.T) {
	request := normalizePricingRequest(PricingRequest{
		Model:     "gpt-image-2",
		Modality:  "image",
		Operation: "generation",
		Unit:      "image",
		Size:      "1024x1024",
		Quantity:  1,
	})
	rules := normalizePricingRules([]model.PricingRule{
		{Model: "gpt-image-2", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Credits: 1, Enabled: true},
	})

	rule, ok := selectPricingRule(rules, request)
	if !ok {
		t.Fatal("expected matching rule")
	}
	if rule.Credits != 1 {
		t.Fatalf("credits = %d, want 1", rule.Credits)
	}
}

func TestSelectPricingRuleSkipsDisabledAndMismatchedRules(t *testing.T) {
	request := normalizePricingRequest(PricingRequest{
		Model:     "gpt-image-2",
		Modality:  "image",
		Operation: "edit",
		Unit:      "image",
	})
	rules := normalizePricingRules([]model.PricingRule{
		{Model: "gpt-image-2", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Credits: 1, Enabled: true},
		{Model: "gpt-image-2", Modality: "image", Operation: "edit", Unit: "image", ResolutionTier: "1k", Credits: 2, Enabled: false},
	})

	if _, ok := selectPricingRule(rules, request); ok {
		t.Fatal("expected no matching enabled rule")
	}
}

func TestModelChannelSelectionsFilterOperationAndResolution(t *testing.T) {
	channels := normalizePrivateSetting(model.PrivateSetting{Channels: []model.ModelChannel{
		{
			Name: "1k-only", BaseURL: "https://one.example", APIKey: "key", Enabled: true,
			Models: []model.ChannelModel{{Model: "gpt-image-2", UpstreamModel: "image-basic", Operations: []string{"generation"}, ResolutionTiers: []string{"1k"}}},
		},
		{
			Name: "full", BaseURL: "https://full.example", APIKey: "key", Enabled: true,
			Models: []model.ChannelModel{{Model: "gpt-image-2", UpstreamModel: "image-full", Operations: []string{"generation", "edit"}, ResolutionTiers: []string{"1k", "2k", "4k"}}},
		},
	}}).Channels

	fourK := normalizePricingRequest(PricingRequest{Model: "gpt-image-2", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "4k"})
	fourKSelections := modelChannelSelectionsForRequest(channels, fourK)
	if len(fourKSelections) != 1 || fourKSelections[0].Channel.Name != "full" || fourKSelections[0].Model.UpstreamModel != "image-full" {
		t.Fatalf("4k selections = %#v, want full channel", fourKSelections)
	}

	oneK := normalizePricingRequest(PricingRequest{Model: "gpt-image-2", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k"})
	if selections := modelChannelSelectionsForRequest(channels, oneK); len(selections) != 2 {
		t.Fatalf("1k selection count = %d, want 2", len(selections))
	} else if remaining := modelChannelSelectionsExcluding(selections, []string{"1k-only"}); len(remaining) != 1 || remaining[0].Channel.Name != "full" {
		t.Fatalf("remaining selections = %#v, want full channel", remaining)
	}

	edit := normalizePricingRequest(PricingRequest{Model: "gpt-image-2", Modality: "image", Operation: "edit", Unit: "image", ResolutionTier: "1k"})
	editSelections := modelChannelSelectionsForRequest(channels, edit)
	if len(editSelections) != 1 || editSelections[0].Channel.Name != "full" {
		t.Fatalf("edit selections = %#v, want full channel", editSelections)
	}
}

func TestVideoModelChannelSelectionsFilterResolutionRatioAndDuration(t *testing.T) {
	channels := normalizePrivateSetting(model.PrivateSetting{Channels: []model.ModelChannel{
		{
			Name: "standard-video", BaseURL: "https://standard.example", APIKey: "key", Enabled: true,
			Models: []model.ChannelModel{{Model: "video-model", Modality: "video", UpstreamModel: "video-standard", Operations: []string{"generation"}, AspectRatios: []string{"16:9"}, ResolutionTiers: []string{"480p", "720p"}, Durations: []int{5, 10}}},
		},
		{
			Name: "hd-video", BaseURL: "https://hd.example", APIKey: "key", Enabled: true,
			Models: []model.ChannelModel{{Model: "video-model", Modality: "video", UpstreamModel: "video-hd", Operations: []string{"generation"}, AspectRatios: []string{"16:9", "9:16"}, ResolutionTiers: []string{"480p", "720p", "1080p"}, Durations: []int{10}}},
		},
	}}).Channels

	fullHD := normalizePricingRequest(PricingRequest{Model: "video-model", Modality: "video", Operation: "generation", Unit: "second", Size: "720x1280", Resolution: "1080p", Quantity: 10})
	fullHDSelections := modelChannelSelectionsForRequest(channels, fullHD)
	if len(fullHDSelections) != 1 || fullHDSelections[0].Channel.Name != "hd-video" || fullHDSelections[0].Model.UpstreamModel != "video-hd" {
		t.Fatalf("1080p selections = %#v, want hd-video channel", fullHDSelections)
	}

	hd := normalizePricingRequest(PricingRequest{Model: "video-model", Modality: "video", Operation: "generation", Unit: "second", Size: "1280x720", Resolution: "720p", Quantity: 5})
	if selections := modelChannelSelectionsForRequest(channels, hd); len(selections) != 1 || selections[0].Channel.Name != "standard-video" {
		t.Fatalf("720p 16:9 5s selections = %#v, want standard-video", selections)
	}

	landscape480p := normalizePricingRequest(PricingRequest{Model: "video-model", Modality: "video", Operation: "generation", Unit: "second", Size: "854x480", Resolution: "480p", Quantity: 5})
	if selections := modelChannelSelectionsForRequest(channels, landscape480p); landscape480p.AspectRatio != "16:9" || len(selections) != 1 || selections[0].Channel.Name != "standard-video" {
		t.Fatalf("480p 16:9 5s selections = %#v with ratio %s, want standard-video", selections, landscape480p.AspectRatio)
	}

	portrait480p := normalizePricingRequest(PricingRequest{Model: "video-model", Modality: "video", Operation: "generation", Unit: "second", Size: "480x854", Resolution: "480p", Quantity: 10})
	if selections := modelChannelSelectionsForRequest(channels, portrait480p); portrait480p.AspectRatio != "9:16" || len(selections) != 1 || selections[0].Channel.Name != "hd-video" {
		t.Fatalf("480p 9:16 10s selections = %#v with ratio %s, want hd-video", selections, portrait480p.AspectRatio)
	}
}
