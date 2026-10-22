package service

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/yypyyd/infinite-canvas/gateway/internal/provider/adobe"
)

func TestAdobeOverloadSurvivesSafeErrorMapping(t *testing.T) {
	err := publicGenerationError(fmt.Errorf("%w: private-secret-response", adobe.ErrSystemUnderLoad))
	if err != ErrAdobeUpstreamBusy || !errors.Is(err, ErrProviderTemporary) {
		t.Fatalf("lost Adobe busy classification: %v", err)
	}
	if safeStoredGenerationError(err.Error()) != err.Error() || strings.Contains(err.Error(), "private-secret") {
		t.Fatal("safe error mapping lost classification or leaked response")
	}
	if auth, quota, temporary, dead := adobeErrClass(adobe.ErrSystemUnderLoad); auth || quota || !temporary || dead {
		t.Fatal("overload must not invalidate the account")
	}
}

func TestAdobeDailyTrialLimitPreservesAccountAndEntitlement(t *testing.T) {
	cause := fmt.Errorf("%w: private-secret-response", adobe.ErrTrialExhausted)
	state, class := dispatchFailureClass(cause)
	if state != "failed" || class != "model_quota" || shouldMarkAccountQuota(cause) {
		t.Fatal("daily trial exhaustion must preserve the account and its renewable model entitlement")
	}
	err := publicGenerationError(cause)
	if err != ErrAdobeTrialExhausted || !errors.Is(err, ErrProviderQuota) || errors.Is(err, ErrProviderAuth) {
		t.Fatalf("trial limit must remain a quota error to API consumers: %v", err)
	}
	if safeStoredGenerationError(err.Error()) != err.Error() || strings.Contains(err.Error(), "private-secret") {
		t.Fatal("trial message was lost or exposed the upstream response")
	}
}
