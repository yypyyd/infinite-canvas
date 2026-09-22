package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// openAIResponseWriter marks API Key calls so handler.Fail writes an OpenAI error.
type openAIResponseWriter struct {
	gin.ResponseWriter
}

func (w *openAIResponseWriter) OpenAICompatible() bool { return true }

func (w *openAIResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
