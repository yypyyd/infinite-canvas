package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
)

// The standalone Nexa admin session (login, bootstrap token, CSRF cookie) was
// removed when the gateway was embedded into 幻图. Admin routes are now gated
// by 幻图's super-admin middleware. There is no external data plane: the pool
// is only reachable in-process, under a fixed built-in principal.

const currentAPICredentialKey = "api_credential"

// UseCredential attaches a fixed credential. It is only mounted on the
// in-process listener that 幻图 itself dials, which is unreachable from the
// network, so no bearer token is required there.
func UseCredential(resolve func() *model.APICredential) gin.HandlerFunc {
	return func(c *gin.Context) {
		credential := resolve()
		if credential == nil {
			writeOpenAIAuthError(c, http.StatusServiceUnavailable, "Built-in gateway credential unavailable.", "internal_error")
			c.Abort()
			return
		}
		c.Set(currentAPICredentialKey, credential)
		c.Next()
	}
}

func CurrentAPICredential(c *gin.Context) *model.APICredential {
	value, ok := c.Get(currentAPICredentialKey)
	if !ok {
		return nil
	}
	credential, _ := value.(*model.APICredential)
	return credential
}

func writeOpenAIAuthError(c *gin.Context, status int, message, code string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    "invalid_request_error",
			"param":   nil,
			"code":    code,
		},
	})
}
