package service

import (
	"testing"

	"github.com/basketikun/infinite-canvas/model"
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

func TestSelectPricingRulePrefersSpecificRule(t *testing.T) {
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
		t.Fatalf("credits = %d, want specific rule credits 3", rule.Credits)
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
		{Model: "gpt-image-2", Modality: "image", Operation: "generation", Unit: "image", Credits: 1, Enabled: true},
		{Model: "gpt-image-2", Modality: "image", Operation: "edit", Unit: "image", Credits: 2, Enabled: false},
	})

	if _, ok := selectPricingRule(rules, request); ok {
		t.Fatal("expected no matching enabled rule")
	}
}
