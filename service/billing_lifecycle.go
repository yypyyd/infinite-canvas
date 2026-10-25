package service

import (
	"github.com/yypyyd/infinite-canvas/model"
)

// Billing lifecycle (Phase 3 one-path)
//
// Generation (interactive handler, batch item, agent media tool, managed protocol):
//
//	1. Estimate  - PricingResolver.Resolve -> PricingResult + PricingSnapshot
//	2. Begin     - BeginGenerationTask (sole debit / charge write)
//	3. Finish    - FinishGenerationTask
//	     success -> CompleteGenerationTask (no extra debit)
//	     failed  -> FailGenerationTaskAndRefund (sole refund point)
//
// Late pool delivery after a failed+refunded task uses AILateRecharge once via
// DeliverLateAndRecharge. That is a recharge of a prior refund, never a second
// debit on an already-successful task. Status transitions failed -> success are
// the idempotency gate.
//
// Hold / settle envelope (batch production job + video-render job):
//
//	HoldCredits -> (work) -> SettleHeldCredits and/or ReleaseHeldCredits
//
// Batch items still call BeginGenerationTask / FinishGenerationTask; the job
// envelope only reserves the aggregate. Video-render estimates through
// EstimateVideoRenderPricing (same PricingResolver + snapshot) then holds /
// settles with the shared repository helpers.
//
// Config side-doors (not debit paths): pool_pricing gaps/drafts, admin pricing
// rules, user pricing discounts. They configure PricingResolver; they never
// write CreditLog consume/refund rows themselves.

// EstimateGenerationPricing is the shared estimate entry for generation-shaped
// requests. Callers must persist result.Snapshot on the GenerationTask (or
// batch item) before BeginGenerationTask.
func EstimateGenerationPricing(userID, userGroup string, request PricingRequest) (PricingResult, error) {
	resolver, err := NewPricingResolver(userID, userGroup)
	if err != nil {
		return PricingResult{}, err
	}
	return resolver.Resolve(request)
}

// GenerationBillingPathNames documents the canonical debit/refund symbols for
// tests and admin tooling. Keep in sync with BeginGenerationTask /
// FinishGenerationTask.
func GenerationBillingPathNames() (begin string, finish string, lateRecharge model.CreditLogType) {
	return "BeginGenerationTask", "FinishGenerationTask", model.CreditLogTypeAILateRecharge
}
