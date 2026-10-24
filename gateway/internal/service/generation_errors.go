package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/yypyyd/infinite-canvas/gateway/internal/provider/adobe"
	"github.com/yypyyd/infinite-canvas/gateway/internal/provider/byteplus"
	"github.com/yypyyd/infinite-canvas/gateway/internal/provider/chatgpt"
	"github.com/yypyyd/infinite-canvas/gateway/internal/provider/grok"
)

var ErrAdobeUpstreamBusy = fmt.Errorf("%w: Adobe 上游服务暂时繁忙，请稍后重试", ErrProviderTemporary)
var ErrAdobeTrialExhausted = fmt.Errorf("%w: Adobe GPT Image 2 今日免费试用次数已用完，请等待每日额度重置后重试", ErrProviderQuota)

// publicGenerationError collapses provider-specific failures into stable,
// credential-free API categories. Provider errors routinely contain reflected
// response bodies, URLs, paths, prompts, cookies, or bearer tokens and must not
// be persisted in EventLog or returned by asynchronous polling endpoints.
func publicGenerationError(cause error) error {
	if cause == nil {
		return nil
	}
	var paramErr *ParameterError
	if errors.As(cause, &paramErr) {
		return paramErr
	}
	var grokSubmitted *grok.VideoSubmissionError
	if errors.As(cause, &grokSubmitted) {
		switch {
		case errors.Is(grokSubmitted.Cause, grok.ErrQuotaExhausted):
			return ErrProviderQuota
		case errors.Is(grokSubmitted.Cause, grok.ErrVideoRejected), errors.Is(grokSubmitted.Cause, grok.ErrInvalidTaskHandle):
			return ErrProviderExecution
		}
	}
	switch {
	case errors.Is(cause, ErrAdobeTrialExhausted), errors.Is(cause, adobe.ErrTrialExhausted):
		return ErrAdobeTrialExhausted
	case errors.Is(cause, ErrAdobeUpstreamBusy), errors.Is(cause, adobe.ErrSystemUnderLoad):
		return ErrAdobeUpstreamBusy
	case errors.Is(cause, ErrGenerationOutcomeUnknown):
		return ErrGenerationOutcomeUnknown
	case errors.Is(cause, ErrIdempotencyConflict):
		return ErrIdempotencyConflict
	case errors.Is(cause, ErrUnknownModel):
		return ErrUnknownModel
	case errors.Is(cause, ErrReferenceTooLarge):
		return ErrReferenceTooLarge
	case errors.Is(cause, ErrReferenceVideoTooLarge):
		return ErrReferenceVideoTooLarge
	case errors.Is(cause, ErrReferenceAudioTooLarge):
		return ErrReferenceAudioTooLarge
	case errors.Is(cause, ErrBannedPrompt):
		return ErrBannedPrompt
	case errors.Is(cause, ErrUnsupportedParams), errors.Is(cause, byteplus.ErrInvalidParams):
		return ErrUnsupportedParams

	case errors.Is(cause, ErrContentRejected), errors.Is(cause, byteplus.ErrRiskControl),
		errors.Is(cause, chatgpt.ErrContentPolicy):
		return ErrContentRejected
	case errors.Is(cause, ErrNoProviderAccount):
		return ErrNoProviderAccount
	case errors.Is(cause, ErrProviderDisabled):
		return ErrProviderDisabled
	case errors.Is(cause, ErrConcurrencyBackendUnavailable):
		return ErrConcurrencyBackendUnavailable
	case errors.Is(cause, ErrUserConcurrencyFull):
		return ErrUserConcurrencyFull
	case errors.Is(cause, ErrConcurrencyFull):
		return ErrConcurrencyFull
	case errors.Is(cause, ErrProviderAuth), errors.Is(cause, adobe.ErrAuth), errors.Is(cause, adobe.ErrEntitlement),
		errors.Is(cause, byteplus.ErrAuth), errors.Is(cause, chatgpt.ErrAuth),
		errors.Is(cause, grok.ErrAuth):
		return ErrProviderAuth
	case errors.Is(cause, ErrProviderQuota), errors.Is(cause, adobe.ErrQuotaExhausted),
		errors.Is(cause, byteplus.ErrQuotaExhausted), errors.Is(cause, chatgpt.ErrQuotaExhausted),
		errors.Is(cause, grok.ErrQuotaExhausted):
		return ErrProviderQuota
	case errors.Is(cause, ErrProviderTemporary), errors.Is(cause, context.Canceled), errors.Is(cause, context.DeadlineExceeded),
		errors.Is(cause, chatgpt.ErrTaskAccepted), errors.Is(cause, chatgpt.ErrTaskSubmissionUnknown), errors.Is(cause, grok.ErrTaskAccepted),
		errors.Is(cause, adobe.ErrTemporaryUpstream), errors.Is(cause, byteplus.ErrTemporaryUpstream),
		errors.Is(cause, chatgpt.ErrTemporaryUpstream),
		errors.Is(cause, grok.ErrTemporaryUpstream):
		return ErrProviderTemporary
	case errors.Is(cause, ErrProviderUnsupported):
		return ErrProviderUnsupported
	default:
		return ErrProviderExecution
	}
}

func safeGenerationErrorText(cause error) string {
	if safe := publicGenerationError(cause); safe != nil {
		return safe.Error()
	}
	return ""
}

// safeStoredGenerationError protects polling/admin reads of rows created by an
// older build that may have persisted raw provider text. Only exact messages
// emitted by this service are allowed through; everything else is collapsed.
func safeStoredGenerationError(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	for _, safe := range []error{
		ErrGenerationOutcomeUnknown, ErrGenerationAbandoned,
		ErrUnknownModel, ErrIdempotencyConflict, ErrInsufficientFunds, ErrUnsupportedParams,
		ErrBannedPrompt, ErrContentRejected, ErrNoProviderAccount, ErrProviderDisabled,
		ErrProviderAuth, ErrProviderQuota, ErrProviderTemporary, ErrProviderExecution,
		ErrAdobeUpstreamBusy, ErrAdobeTrialExhausted,
		ErrProviderUnsupported,
		ErrConcurrencyBackendUnavailable, ErrConcurrencyFull, ErrUserConcurrencyFull,
		ErrReferenceTooLarge, ErrReferenceVideoTooLarge, ErrReferenceAudioTooLarge,
	} {
		if raw == safe.Error() {
			return raw
		}
	}
	return ErrProviderExecution.Error()
}

func recordBookkeepingError(operation string, err error) {
	if err != nil {
		// Database/transport errors can embed DSNs, signed URLs, provider bodies,
		// or credentials. The operation label is a fixed call-site constant and is
		// sufficient for alerting; never log the raw error here.
		log.Printf("generation bookkeeping %s failed", operation)
	}
}

func safeQuotaProbeError(data map[string]any) any {
	if data == nil || strings.TrimSpace(stringValue(data["error"])) == "" {
		return nil
	}
	return "provider quota probe failed"
}
