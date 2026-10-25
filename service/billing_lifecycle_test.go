package service

import (
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestGenerationBillingPathNames(t *testing.T) {
	begin, finish, late := GenerationBillingPathNames()
	if begin != "BeginGenerationTask" || finish != "FinishGenerationTask" || late != model.CreditLogTypeAILateRecharge {
		t.Fatalf("unexpected path names: %s %s %s", begin, finish, late)
	}
}

func TestEstimateVideoRenderPricingPersistsSnapshot(t *testing.T) {
	result, err := EstimateVideoRenderPricing("", "default", 1500, model.VideoOutputSpec{Width: 1920, Height: 1080})
	if err != nil {
		t.Fatal(err)
	}
	if result.Credits < 1 {
		t.Fatalf("expected positive credits, got %d", result.Credits)
	}
	if result.Snapshot.Model != videoRenderModelID || result.Snapshot.Credits != result.Credits || result.Snapshot.Quantity < 1 {
		t.Fatalf("snapshot incomplete: %#v", result.Snapshot)
	}
	if result.Snapshot.Unit != "second" && result.Snapshot.Unit != "request" {
		t.Fatalf("unexpected unit %q", result.Snapshot.Unit)
	}
}

func TestEstimateGenerationPricingRequiresRule(t *testing.T) {
	_, err := EstimateGenerationPricing("", "default", PricingRequest{
		Model: "missing-model-phase3", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Quantity: 1,
	})
	if err == nil {
		t.Fatal("expected missing pricing rule error")
	}
}
