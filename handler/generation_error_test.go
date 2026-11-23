package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type apiErrorRecorder struct{ *httptest.ResponseRecorder }

func (apiErrorRecorder) OpenAICompatible() bool { return true }

func TestGenerationFailureResponse(t *testing.T) {
	for _, tc := range []struct {
		name, body, want    string
		upstream, apiStatus int
	}{
		{"reason", `{"error":{"code":"quota_exceeded","message":"Daily Limit Reached"}}`, "Daily Limit Reached", 429, 429},
		{"string error", `{"error":"Reference image is too large"}`, "Reference image is too large", 400, 400},
		{"no reason", `<html>edge failure</html>`, "HTTP 403 Forbidden", 403, 502},
		{"credential", `{"error":{"message":"invalid secret-fixture"}}`, "HTTP 401 Unauthorized", 401, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/v1/images/edits", nil)
			req.Header.Set("Authorization", "Bearer secret-fixture")
			for _, api := range []bool{false, true} {
				recorder := httptest.NewRecorder()
				var w http.ResponseWriter = recorder
				if api {
					w = apiErrorRecorder{recorder}
					writeAIKeepAlive(w)
				}
				response := &http.Response{StatusCode: tc.upstream, Body: io.NopCloser(strings.NewReader(tc.body))}
				var saved string
				writeAIResponse(w, req, response, []byte(tc.body), nil, func(message string) { saved = message }, nil)
				var result struct {
					Msg   string
					Error struct{ Message string }
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				message, status := result.Msg, 200
				if api {
					message, status = result.Error.Message, tc.apiStatus
				}
				if message != tc.want || saved != message || recorder.Code != status {
					t.Fatalf("api=%t status=%d message=%q saved=%q", api, recorder.Code, message, saved)
				}
			}
		})
	}
}

func TestImageRecoveryForwardsFailure(t *testing.T) {
	for _, tc := range []struct {
		body       string
		httpStatus int
		want       string
	}{
		{`{"detail":"Access Denied for Image"}`, 403, "Access Denied for Image"},
		{`{"status":"failed","error":{"code":"rejected","message":"图片超过大小限制"}}`, 200, "图片超过大小限制"},
		{`{"status":"failed","error":"HTTP 401 Unauthorized","error_status":401}`, 200, "HTTP 401 Unauthorized"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.httpStatus); io.WriteString(w, tc.body) }))
		defer server.Close()
		req := httptest.NewRequest("POST", server.URL+"/images/generations", nil)
		req.Header.Set("Idempotency-Key", "test")
		_, failure, disposition := recoverImageTask(httptest.NewRecorder(), req, true)
		if disposition != imageTaskRecoveryHandled || failure.message != tc.want {
			t.Fatalf("got %v %q", disposition, failure.message)
		}
	}
}
