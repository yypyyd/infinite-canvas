package service

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func TestPricingResolverUsesExactUserSpecBeforeGroupRatio(t *testing.T) {
	rules := normalizePricingRules([]model.PricingRule{
		{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Credits: 10, Enabled: true},
		{Model: "image-model", Modality: "image", Operation: "edit", Unit: "image", ResolutionTier: "1k", Credits: 10, Enabled: true},
	})
	discount := model.UserPricingDiscount{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Ratio: 0.5}
	resolver := &PricingResolver{
		modelChannel: model.PublicModelChannelSetting{PricingRules: rules, GroupRatios: normalizeGroupRatios(map[string]float64{"default": 1, "vip": 0.8})},
		userGroup:    "vip",
		discounts:    map[string]model.UserPricingDiscount{pricingSpecKey(discount.Model, discount.Modality, discount.Operation, discount.Unit, discount.ResolutionTier): discount},
	}

	userResult, err := resolver.Resolve(PricingRequest{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Quantity: 2})
	if err != nil {
		t.Fatal(err)
	}
	if userResult.Credits != 10 || userResult.EffectiveRatio != 0.5 || userResult.Source != PricingSourceUserSpec {
		t.Fatalf("unexpected user pricing result: %#v", userResult)
	}
	if userResult.Snapshot.GroupRatio != 0.8 || userResult.Snapshot.UserSpecRatio == nil || *userResult.Snapshot.UserSpecRatio != 0.5 || userResult.Snapshot.Credits != 10 {
		t.Fatalf("unexpected user pricing snapshot: %#v", userResult.Snapshot)
	}

	groupResult, err := resolver.Resolve(PricingRequest{Model: "image-model", Modality: "image", Operation: "edit", Unit: "image", ResolutionTier: "1k", Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	if groupResult.Credits != 8 || groupResult.EffectiveRatio != 0.8 || groupResult.Source != PricingSourceGroup || groupResult.Snapshot.UserSpecRatio != nil {
		t.Fatalf("unexpected group pricing result: %#v", groupResult)
	}
}

func TestPricingResolverSeparatesImageOperationsAndResolutionTiers(t *testing.T) {
	rules := normalizePricingRules([]model.PricingRule{
		{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Credits: 10, Enabled: true},
		{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "2k", Credits: 20, Enabled: true},
		{Model: "image-model", Modality: "image", Operation: "edit", Unit: "image", ResolutionTier: "1k", Credits: 12, Enabled: true},
	})
	discounts := map[string]model.UserPricingDiscount{}
	for _, discount := range []model.UserPricingDiscount{
		{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Ratio: 0.8},
		{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "2k", Ratio: 0.7},
	} {
		discounts[pricingSpecKey(discount.Model, discount.Modality, discount.Operation, discount.Unit, discount.ResolutionTier)] = discount
	}
	resolver := &PricingResolver{modelChannel: model.PublicModelChannelSetting{PricingRules: rules, GroupRatios: map[string]float64{"default": 1, "vip": 0.9}}, userGroup: "vip", discounts: discounts}
	tests := []struct {
		operation, tier, source string
		credits                 int
	}{
		{operation: "generation", tier: "1k", source: "user_spec", credits: 8},
		{operation: "generation", tier: "2k", source: "user_spec", credits: 14},
		{operation: "edit", tier: "1k", source: "group", credits: 11},
	}
	for _, test := range tests {
		result, err := resolver.Resolve(PricingRequest{Model: "image-model", Modality: "image", Operation: test.operation, Unit: "image", ResolutionTier: test.tier, Quantity: 1})
		if err != nil {
			t.Fatal(err)
		}
		if result.Credits != test.credits || string(result.Source) != test.source {
			t.Fatalf("%s %s result = %#v", test.operation, test.tier, result)
		}
	}
}

func TestPricingResolverUsesVideoResolutionRatioBeforeDurationQuantity(t *testing.T) {
	rules := normalizePricingRules([]model.PricingRule{
		{Model: "video-model", Modality: "video", Operation: "generation", Unit: "second", ResolutionTier: "720p", Credits: 2, Enabled: true},
		{Model: "video-model", Modality: "video", Operation: "generation", Unit: "second", ResolutionTier: "1080p", Credits: 4, Enabled: true},
	})
	discounts := map[string]model.UserPricingDiscount{}
	for _, discount := range []model.UserPricingDiscount{
		{Model: "video-model", Modality: "video", Operation: "generation", Unit: "second", ResolutionTier: "720p", Ratio: 0.5},
		{Model: "video-model", Modality: "video", Operation: "generation", Unit: "second", ResolutionTier: "1080p", Ratio: 0.75},
	} {
		discounts[pricingSpecKey(discount.Model, discount.Modality, discount.Operation, discount.Unit, discount.ResolutionTier)] = discount
	}
	resolver := &PricingResolver{modelChannel: model.PublicModelChannelSetting{PricingRules: rules, GroupRatios: map[string]float64{"default": 1}}, userGroup: "default", discounts: discounts}
	for _, test := range []struct {
		tier     string
		duration int
		credits  int
	}{{tier: "720p", duration: 5, credits: 5}, {tier: "720p", duration: 10, credits: 10}, {tier: "1080p", duration: 5, credits: 15}, {tier: "1080p", duration: 10, credits: 30}} {
		result, err := resolver.Resolve(PricingRequest{Model: "video-model", Modality: "video", Operation: "generation", Unit: "second", ResolutionTier: test.tier, Quantity: test.duration})
		if err != nil {
			t.Fatal(err)
		}
		if result.Credits != test.credits || result.Source != PricingSourceUserSpec {
			t.Fatalf("%s %ds result = %#v", test.tier, test.duration, result)
		}
	}
}

func TestUserPricingRatioMustBeFiniteAndWithinDiscountRange(t *testing.T) {
	for _, ratio := range []float64{0, -0.1, 1.01, 0.12345, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if validPricingRatio(ratio) {
			t.Fatalf("ratio %v should be invalid", ratio)
		}
	}
	for _, ratio := range []float64{0.01, 0.5, 1} {
		if !validPricingRatio(ratio) {
			t.Fatalf("ratio %v should be valid", ratio)
		}
	}
}

func TestUserPricingRemarkLimitCountsUnicodeCharacters(t *testing.T) {
	user, _, _ := seedTenant(t, "pricing-unicode-remark")
	savedSettings, err := repository.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings := savedSettings
	settings.Public.ModelChannel.PricingRules = []model.PricingRule{{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Credits: 10, Enabled: true}}
	if _, err := repository.SaveSettings(settings, "pricing-unicode-remark-settings"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = repository.SaveSettings(savedSettings, "pricing-unicode-remark-cleanup") })
	item := model.UserPricingDiscount{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Ratio: 0.8, Remark: strings.Repeat("价", 255)}
	if _, err := ReplaceUserPricingDiscounts(user.ID, []model.UserPricingDiscount{item}); err != nil {
		t.Fatalf("255 Chinese characters should be accepted: %v", err)
	}
	item.Remark += "格"
	if _, err := ReplaceUserPricingDiscounts(user.ID, []model.UserPricingDiscount{item}); err == nil {
		t.Fatal("256 Chinese characters should be rejected")
	}
}

func TestReplaceUserPricingDiscountsKeepsInvalidRuleOnlyWhenUnchanged(t *testing.T) {
	user, _, _ := seedTenant(t, "invalid-user-pricing")
	savedSettings, err := repository.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings := savedSettings
	settings.Public.ModelChannel.PricingRules = []model.PricingRule{{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Credits: 10, Enabled: true}}
	if _, err := repository.SaveSettings(settings, "invalid-user-pricing-enabled"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = repository.SaveSettings(savedSettings, "invalid-user-pricing-cleanup") })

	item := model.UserPricingDiscount{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Ratio: 0.8, Remark: "协议价"}
	created, err := ReplaceUserPricingDiscounts(user.ID, []model.UserPricingDiscount{item})
	if err != nil || len(created) != 1 {
		t.Fatalf("create pricing discount: items=%#v err=%v", created, err)
	}
	settings.Public.ModelChannel.PricingRules[0].Enabled = false
	if _, err := repository.SaveSettings(settings, "invalid-user-pricing-disabled"); err != nil {
		t.Fatal(err)
	}
	kept, err := ReplaceUserPricingDiscounts(user.ID, []model.UserPricingDiscount{item})
	if err != nil || len(kept) != 1 || kept[0].ID != created[0].ID {
		t.Fatalf("unchanged invalid discount should be preserved: items=%#v err=%v", kept, err)
	}
	changed := item
	changed.Ratio = 0.7
	if _, err := ReplaceUserPricingDiscounts(user.ID, []model.UserPricingDiscount{changed}); err == nil {
		t.Fatal("changing an invalid discount ratio should fail")
	}
	changed = item
	changed.Remark = "已修改"
	if _, err := ReplaceUserPricingDiscounts(user.ID, []model.UserPricingDiscount{changed}); err == nil {
		t.Fatal("changing an invalid discount remark should fail")
	}
	deleted, err := ReplaceUserPricingDiscounts(user.ID, []model.UserPricingDiscount{})
	if err != nil || len(deleted) != 0 {
		t.Fatalf("explicitly omitting invalid discount should delete it: items=%#v err=%v", deleted, err)
	}
}

func TestReplaceUserPricingDiscountsRejectsInvalidSetsWithoutPartialReplacement(t *testing.T) {
	user, _, _ := seedTenant(t, "pricing-validation")
	savedSettings, err := repository.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings := savedSettings
	settings.Public.ModelChannel.PricingRules = []model.PricingRule{{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Credits: 10, Enabled: true}}
	if _, err := repository.SaveSettings(settings, "pricing-validation-settings"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = repository.SaveSettings(savedSettings, "pricing-validation-cleanup") })

	original := model.UserPricingDiscount{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Ratio: 0.8}
	if _, err := ReplaceUserPricingDiscounts(user.ID, []model.UserPricingDiscount{original}); err != nil {
		t.Fatal(err)
	}
	invalidSets := [][]model.UserPricingDiscount{
		{original, original},
		{{Model: "unknown-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Ratio: 0.5}},
		make([]model.UserPricingDiscount, MaxUserPricingDiscounts+1),
	}
	for _, items := range invalidSets {
		if _, err := ReplaceUserPricingDiscounts(user.ID, items); err == nil {
			t.Fatalf("invalid replacement should fail: count=%d", len(items))
		}
		remaining, err := repository.ListUserPricingDiscounts(user.ID)
		if err != nil || len(remaining) != 1 || remaining[0].Ratio != original.Ratio {
			t.Fatalf("invalid replacement changed the saved set: items=%#v err=%v", remaining, err)
		}
	}
}

func TestPricingResolverIsolatesUsersAndDoesNotInheritNewResolutionDiscounts(t *testing.T) {
	userA, _, _ := seedTenant(t, "pricing-isolation-a")
	userB, _, _ := seedTenant(t, "pricing-isolation-b")
	savedSettings, err := repository.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings := savedSettings
	settings.Public.ModelChannel.GroupRatios = map[string]float64{"default": 0.9}
	settings.Public.ModelChannel.PricingRules = []model.PricingRule{
		{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Credits: 10, Enabled: true},
		{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "4k", Credits: 40, Enabled: true},
	}
	if _, err := repository.SaveSettings(settings, "pricing-isolation-settings"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = repository.SaveSettings(savedSettings, "pricing-isolation-cleanup") })
	if _, err := ReplaceUserPricingDiscounts(userA.ID, []model.UserPricingDiscount{{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Ratio: 0.5}}); err != nil {
		t.Fatal(err)
	}

	resolverA, err := NewPricingResolver(userA.ID, "default")
	if err != nil {
		t.Fatal(err)
	}
	resolverB, err := NewPricingResolver(userB.ID, "default")
	if err != nil {
		t.Fatal(err)
	}
	oneK, err := resolverA.Resolve(PricingRequest{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	fourK, err := resolverA.Resolve(PricingRequest{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "4k", Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	otherUser, err := resolverB.Resolve(PricingRequest{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	if oneK.Source != PricingSourceUserSpec || oneK.Credits != 5 || fourK.Source != PricingSourceDefault || fourK.Credits != 36 || otherUser.Source != PricingSourceDefault || otherUser.Credits != 9 {
		t.Fatalf("unexpected isolated pricing: userA1k=%#v userA4k=%#v userB1k=%#v", oneK, fourK, otherUser)
	}
}

func TestPricingSnapshotKeepsUserSpecRatioAcrossJSONRoundTrip(t *testing.T) {
	ratio := 0.7123
	original := model.PricingSnapshot{Model: "image-model", Modality: "image", Operation: "edit", Unit: "image", ResolutionTier: "2k", Quantity: 1, BillingMode: "fixed", RuleCredits: 8, UserGroup: "vip", GroupRatio: 0.9, UserSpecRatio: &ratio, EffectiveRatio: ratio, Source: "user_spec", Credits: 6}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded model.PricingSnapshot
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Fatalf("pricing snapshot changed after JSON round trip: got=%#v want=%#v", decoded, original)
	}
}
