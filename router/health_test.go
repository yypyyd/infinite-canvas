package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHealthEndpointsExposeLivenessAndDatabaseReadiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := New()
	for _, path := range []string{"/api/health", "/api/health/live", "/api/health/ready"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, response.Code)
		}
		if response.Header().Get("X-Request-ID") == "" {
			t.Fatalf("GET %s did not return a request ID", path)
		}
		var body struct {
			Code int `json:"code"`
			Data struct {
				Status string `json:"status"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Code != 0 || body.Data.Status != "ok" {
			t.Fatalf("GET %s response = %s, err=%v", path, response.Body.String(), err)
		}
	}
}
