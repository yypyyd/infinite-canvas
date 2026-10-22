package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/yypyyd/infinite-canvas/gateway/internal/service"
)

func TestV1ParameterErrorIsActionableWithoutEchoingInput(t *testing.T) {
	err := &service.ParameterError{Param: "n", Message: "n must be 1.", Supported: []string{"1"}}
	status, body := v1ErrorResponse(err, nil)
	data, _ := json.Marshal(body)
	for _, value := range []string{`"param":"n"`, `"code":"unsupported_parameter"`, `"supported_values":["1"]`} {
		if !strings.Contains(string(data), value) {
			t.Fatalf("missing %s", value)
		}
	}
	if status != 400 {
		t.Fatalf("status=%d", status)
	}
	_, body = v1ErrorResponse(errors.Join(service.ErrUnsupportedParams, errors.New("secret-token")), nil)
	data, _ = json.Marshal(body)
	if strings.Contains(string(data), "secret-token") {
		t.Fatal("raw failure leaked")
	}
}

func TestAdminProviderAuthErrorIsNotReportedAsOriginFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	adminTestError(ctx, service.ErrProviderAuth)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "所选账号凭据失效") {
		t.Fatalf("body=%s", recorder.Body.String())
	}
}

func TestAdminUpstreamBusyReturnsActionableJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, err := range []error{service.ErrProviderTemporary, errors.Join(service.ErrAdobeUpstreamBusy, errors.New("private-secret-response"))} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		adminTestError(ctx, err)
		if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("upstream failure returned origin error: %d", recorder.Code)
		}
		if strings.Contains(recorder.Body.String(), "private-secret") {
			t.Fatal("raw upstream response leaked")
		}
		if errors.Is(err, service.ErrAdobeUpstreamBusy) && !strings.Contains(recorder.Body.String(), "adobe_upstream_busy") {
			t.Fatal("Adobe busy code missing")
		}
	}
	status, body := v1ErrorResponse(service.ErrAdobeUpstreamBusy, nil)
	data, _ := json.Marshal(body)
	if status != http.StatusServiceUnavailable || !strings.Contains(string(data), "adobe_upstream_busy") {
		t.Fatal("public API lost Adobe busy classification")
	}
}

func TestAdobeTrialExhaustionExplainsModelLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	err := errors.Join(service.ErrAdobeTrialExhausted, errors.New("private-secret-response"))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	adminTestError(ctx, err)
	if recorder.Code != http.StatusTooManyRequests || !strings.Contains(recorder.Body.String(), "adobe_trial_exhausted") || !strings.Contains(recorder.Body.String(), "免费试用") {
		t.Fatalf("trial limit lost its explanation: %d %s", recorder.Code, recorder.Body.String())
	}
	status, body := v1ErrorResponse(err, nil)
	data, _ := json.Marshal(body)
	if status != http.StatusTooManyRequests || !strings.Contains(string(data), "adobe_trial_exhausted") || strings.Contains(string(data)+recorder.Body.String(), "private-secret") {
		t.Fatal("trial limit lost its classification or exposed the response")
	}
	if !strings.Contains(recorder.Body.String(), "每日额度重置") || !strings.Contains(string(data), "daily allowance resets") || strings.Contains(string(data), "Upgrade") {
		t.Fatal("daily trial limit must explain automatic renewal")
	}
}
