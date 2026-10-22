package service

import (
	"testing"

	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func TestPoolPricingDraftsAreListedCreatedAndNeverBilled(t *testing.T) {
	original := builtinPoolModels
	t.Cleanup(func() { builtinPoolModels = original })
	builtinPoolModels = func() []gateway.PoolModel {
		return []gateway.PoolModel{{ID: "pool-img", Name: "pool-img", Modality: "image", Operations: []string{"generation", "edit"}, ResolutionTiers: []string{"1k", "2k"}}}
	}
	saved, err := repository.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = repository.SaveSettings(saved, now()) })
	settings := saved
	settings.Private.Storage.Driver, settings.Private.Storage.LocalPath = "local", t.TempDir()
	settings.Public.ModelChannel.PricingRules = append([]model.PricingRule{}, model.PricingRule{Model: "pool-img", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", BillingMode: "fixed", Credits: 5, Enabled: true})
	if _, err := repository.SaveSettings(settings, now()); err != nil {
		t.Fatal(err)
	}

	gaps, err := PoolPricingGaps()
	if err != nil || len(gaps) != 3 {
		t.Fatalf("gaps=%+v err=%v", gaps, err)
	}
	created, err := CreatePoolPricingDrafts()
	if err != nil || created != 3 {
		t.Fatalf("created=%d err=%v", created, err)
	}
	if again, _ := CreatePoolPricingDrafts(); again != 0 {
		t.Fatal("drafts must not be duplicated")
	}
	gaps, _ = PoolPricingGaps()
	if len(gaps) != 3 || !gaps[0].Draft {
		t.Fatalf("drafts stay listed until priced: %+v", gaps)
	}
	current, _ := repository.GetSettings()
	for _, rule := range normalizePricingRules(current.Public.ModelChannel.PricingRules) {
		if rule.Draft && (rule.Enabled || rule.Credits != 0) {
			t.Fatalf("draft has a price or is enabled: %+v", rule)
		}
	}
	// A draft that someone tries to enable without clearing it stays unusable.
	rules := normalizePricingRules([]model.PricingRule{{Model: "pool-img", Modality: "image", Operation: "edit", Unit: "image", ResolutionTier: "2k", Credits: 9, Enabled: true, Draft: true}})
	if rules[0].Enabled {
		t.Fatal("draft rules are forced disabled")
	}
	if _, ok := selectPricingRule([]model.PricingRule{{Model: "pool-img", Modality: "image", Operation: "edit", Unit: "image", ResolutionTier: "2k", Credits: 9, Enabled: true, Draft: true}}, PricingRequest{Model: "pool-img", Modality: "image", Operation: "edit", Unit: "image", ResolutionTier: "2k", Quantity: 1}); ok {
		t.Fatal("draft rule must never price a request")
	}
	resolver, err := NewPricingResolver("", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(PricingRequest{Model: "pool-img", Modality: "image", Operation: "edit", Unit: "image", ResolutionTier: "1k", Quantity: 1}); err == nil {
		t.Fatal("unpriced draft spec must still fail with 未设置价格")
	}
	if result, err := resolver.Resolve(PricingRequest{Model: "pool-img", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Quantity: 1}); err != nil || result.Credits != 5 {
		t.Fatalf("priced spec unaffected: %+v %v", result, err)
	}
}
