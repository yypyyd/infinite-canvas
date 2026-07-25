package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestRequestObservabilityPreservesValidRequestIDAndWritesStructuredLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	previousLogger := accessLogger
	accessLogger = slog.New(slog.NewJSONHandler(&output, nil))
	t.Cleanup(func() { accessLogger = previousLogger })
	var handlerRequestID string
	engine := gin.New()
	engine.Use(RequestObservability)
	engine.GET("/items/:id", func(c *gin.Context) {
		handlerRequestID = c.GetHeader(RequestIDHeader)
		c.Status(http.StatusCreated)
	})
	request := httptest.NewRequest(http.MethodGet, "/items/1", nil)
	request.Header.Set(RequestIDHeader, "request-valid_1")
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || response.Header().Get(RequestIDHeader) != "request-valid_1" || handlerRequestID != "request-valid_1" {
		t.Fatalf("unexpected request ID propagation: status=%d response=%q handler=%q", response.Code, response.Header().Get(RequestIDHeader), handlerRequestID)
	}
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &entry); err != nil {
		t.Fatalf("decode access log: %v", err)
	}
	if entry["msg"] != "http_request" || entry["request_id"] != "request-valid_1" || entry["path"] != "/items/:id" || entry["status"] != float64(http.StatusCreated) {
		t.Fatalf("unexpected access log: %#v", entry)
	}
}

func TestRequestObservabilityReplacesInvalidRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RequestObservability)
	engine.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Header.Set(RequestIDHeader, "invalid request id")
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, request)
	requestID := response.Header().Get(RequestIDHeader)
	if _, err := uuid.Parse(requestID); err != nil {
		t.Fatalf("generated request ID %q is not a UUID: %v", requestID, err)
	}
}
