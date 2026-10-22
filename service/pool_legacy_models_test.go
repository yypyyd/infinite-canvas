package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func imageRule(id, tier string, credits int, enabled, draft bool) model.PricingRule {
	return model.PricingRule{Model: id, Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: tier, BillingMode: "fixed", Credits: credits, Enabled: enabled, Draft: draft}
}

func TestLegacyPoolPricingRulesMergeOntoUnifiedIDs(t *testing.T) {
	rules := normalizePricingRules([]model.PricingRule{
		imageRule("other-model", "1k", 1, true, false),
		imageRule("chatgpt-gpt-image-2", "1k", 4, true, false),
		imageRule("byteplus-gpt-image-2", "1k", 6, true, false), // higher live price wins
		imageRule("adobe-gpt-image-2", "1k", 9, false, true),    // draft never beats a live rule
		imageRule("byteplus-gpt-image-2", "2k", 8, true, false),
		imageRule("gpt-image-2", "2k", 7, true, false), // an explicit unified rule always wins
		imageRule("adobe-nano-banana-2", "4k", 0, false, true),
		{Model: "adobe-seedance-2.0", Modality: "video", Operation: "generation", Unit: "second", ResolutionTier: "720p", Credits: 3, Enabled: true},
	})
	got := map[string]model.PricingRule{}
	for _, rule := range rules {
		if _, legacy := unifiedPoolModelID(rule.Model); legacy {
			t.Fatalf("legacy ID survived: %+v", rule)
		}
		key := pricingSpecKey(rule.Model, rule.Modality, rule.Operation, rule.Unit, rule.ResolutionTier)
		if _, dup := got[key]; dup {
			t.Fatalf("duplicate spec %s", key)
		}
		got[key] = rule
	}
	if len(rules) != 5 || rules[0].Model != "other-model" {
		t.Fatalf("rules=%+v", rules)
	}
	oneK := got[pricingSpecKey("gpt-image-2", "image", "generation", "image", "1k")]
	if oneK.Credits != 6 || !oneK.Enabled || !strings.Contains(oneK.Remark, "chatgpt-gpt-image-2") || !strings.Contains(oneK.Remark, "adobe-gpt-image-2") {
		t.Fatalf("1k merge=%+v", oneK)
	}
	if twoK := got[pricingSpecKey("gpt-image-2", "image", "generation", "image", "2k")]; twoK.Credits != 7 {
		t.Fatalf("unified rule must win, got %+v", twoK)
	}
	if nano := got[pricingSpecKey("nano-banana-2", "image", "generation", "image", "4k")]; !nano.Draft {
		t.Fatalf("draft kept as draft: %+v", nano)
	}
	if video, ok := got[pricingSpecKey("seedance-2.0", "video", "generation", "second", "720p")]; !ok || video.Credits != 3 {
		t.Fatalf("video rule=%+v", video)
	}
	again, changed := mergeLegacyPoolPricingRules(rules)
	if changed != 0 || len(again) != len(rules) {
		t.Fatal("merge must be idempotent")
	}
}

func TestMigrateLegacyPoolPricingRulesPersists(t *testing.T) {
	saved, err := repository.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = repository.SaveSettings(saved, now()) })
	settings := saved
	settings.Private.Storage.Driver, settings.Private.Storage.LocalPath = "local", t.TempDir()
	settings.Public.ModelChannel.PricingRules = []model.PricingRule{imageRule("adobe-nano-banana-pro", "2k", 5, true, false)}
	if _, err := repository.SaveSettings(settings, now()); err != nil {
		t.Fatal(err)
	}
	if n, err := MigrateLegacyPoolPricingRules(); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	current, _ := repository.GetSettings()
	rules := current.Public.ModelChannel.PricingRules
	if len(rules) != 1 || rules[0].Model != "nano-banana-pro" || rules[0].Credits != 5 {
		t.Fatalf("stored rules=%+v", rules)
	}
	if n, _ := MigrateLegacyPoolPricingRules(); n != 0 {
		t.Fatal("second run must be a no-op")
	}
}

func TestPricingGapListsServingProviders(t *testing.T) {
	routes := []gateway.PoolRoute{
		{Provider: "chatgpt", Operations: []string{"generation", "edit"}, ResolutionTiers: []string{"1K"}},
		{Provider: "byteplus", Operations: []string{"generation", "edit"}, ResolutionTiers: []string{"1K", "2K", "4K"}},
		{Provider: "adobe", Operations: []string{"generation"}, ResolutionTiers: []string{"1K", "2K", "4K"}},
	}
	cases := map[string]PoolPricingGap{
		"adobe,byteplus,chatgpt": {Operation: "generation", ResolutionTier: "1k"},
		"adobe,byteplus":         {Operation: "generation", ResolutionTier: "4k"},
		"byteplus":               {Operation: "edit", ResolutionTier: "2k"},
	}
	for want, spec := range cases {
		if got := strings.Join(specProviders(routes, spec), ","); got != want {
			t.Fatalf("%+v providers=%s, want %s", spec, got, want)
		}
	}
}

func TestPoolUsageGroupsByUnifiedModelWithProviderBreakdown(t *testing.T) {
	usePoolTasks(t, &fakePoolTasks{})
	now := time.Now()
	events := []gateway.PoolEvent{
		{ID: "u1", RequestID: "r1", Kind: "image", Status: "success", Model: "gpt-image-2", Provider: "chatgpt", AccountID: "a", TS: now},
		{ID: "u2", RequestID: "r2", Kind: "image", Status: "failed", Model: "gpt-image-2", Provider: "adobe", AccountID: "b", TS: now},
		{ID: "u3", RequestID: "r3", Kind: "image", Status: "success", Model: "adobe-gpt-image-2", Provider: "adobe", AccountID: "b", TS: now},
		{ID: "u4", RequestID: "r4", Kind: "image", Status: "success", Model: "nano-banana-2", Provider: "byteplus", AccountID: "c", TS: now},
	}
	original := currentPoolEvents
	t.Cleanup(func() { currentPoolEvents = original })
	currentPoolEvents = func() poolEventSource { return fakePoolEvents{events: events} }
	report, err := BuildPoolUsage(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	var gpt *PoolUsageRow
	for i := range report.Models {
		if report.Models[i].Key == "gpt-image-2" {
			gpt = &report.Models[i]
		}
		if strings.HasPrefix(report.Models[i].Key, "adobe-") {
			t.Fatalf("legacy model row: %+v", report.Models[i])
		}
	}
	if gpt == nil || gpt.Requests != 3 || len(gpt.Providers) != 2 {
		t.Fatalf("gpt row=%+v", gpt)
	}
	byProvider := map[string]PoolUsageRow{}
	for _, row := range gpt.Providers {
		byProvider[row.Provider] = row
	}
	if adobe := byProvider["adobe"]; adobe.Requests != 2 || adobe.Succeeded != 1 || adobe.Failed != 1 || adobe.Key != "image.gpt-image-2.adobe" {
		t.Fatalf("adobe breakdown=%+v", adobe)
	}
	if chatgpt := byProvider["chatgpt"]; chatgpt.Requests != 1 || chatgpt.Succeeded != 1 {
		t.Fatalf("chatgpt breakdown=%+v", chatgpt)
	}
}
