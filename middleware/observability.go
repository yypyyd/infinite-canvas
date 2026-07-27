package middleware

import (
	"log/slog"
	"os"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const RequestIDHeader = "X-Request-ID"

var (
	requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	accessLogger     = slog.New(slog.NewJSONHandler(os.Stdout, nil))
)

func RequestObservability(c *gin.Context) {
	startedAt := time.Now()
	requestID := c.GetHeader(RequestIDHeader)
	if !requestIDPattern.MatchString(requestID) {
		requestID = uuid.NewString()
	}
	c.Request.Header.Set(RequestIDHeader, requestID)
	c.Header(RequestIDHeader, requestID)
	c.Set("request_id", requestID)
	c.Next()

	path := c.FullPath()
	if path == "" {
		path = c.Request.URL.Path
	}
	accessLogger.Info("http_request",
		"request_id", requestID,
		"method", c.Request.Method,
		"path", path,
		"status", c.Writer.Status(),
		"latency_ms", time.Since(startedAt).Milliseconds(),
		"response_bytes", c.Writer.Size(),
		"client_ip", c.ClientIP(),
	)
}
