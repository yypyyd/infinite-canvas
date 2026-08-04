package middleware

import (
	"net/http"
	"strings"

	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/handler"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/service"
	"github.com/gin-gonic/gin"
)

func AdminAuth(c *gin.Context) {
	user, ok := authUser(c)
	if !ok || user.Role != model.UserRoleAdmin {
		handler.Fail(c.Writer, "未登录或权限不足")
		c.Abort()
		return
	}
	c.Request = c.Request.WithContext(service.WithUser(c.Request.Context(), user))
	c.Next()
}

func UserAuth(c *gin.Context) {
	user, ok := authUser(c)
	if !ok || user.Role == model.UserRoleGuest {
		handler.Fail(c.Writer, "未登录或权限不足")
		c.Abort()
		return
	}
	if organizationID := strings.TrimSpace(c.GetHeader("X-Organization-ID")); organizationID != "" {
		organization, _, err := service.ResolveOrganizationAccess(user, organizationID)
		if err != nil {
			handler.FailError(c.Writer, err)
			c.Abort()
			return
		}
		user.OrganizationID = organization.ID
	}
	c.Request = c.Request.WithContext(service.WithUser(c.Request.Context(), user))
	c.Next()
}

// APIAuth 允许登录会话或绑定企业的用户 API Key 访问 AI 接口。
func APIAuth(c *gin.Context) {
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	scheme, token, hasScheme := strings.Cut(authorization, " ")
	token = strings.TrimSpace(token)
	if !hasScheme || !strings.EqualFold(scheme, "Bearer") || !strings.HasPrefix(token, service.UserAPIKeyTokenPrefix) {
		UserAuth(c)
		return
	}
	user, apiKey, err := service.AuthenticateUserAPIKey(token)
	if err != nil {
		handler.FailError(c.Writer, err)
		c.Abort()
		return
	}
	requestedOrganizationID := strings.TrimSpace(c.GetHeader("X-Organization-ID"))
	if requestedOrganizationID == "" {
		requestedOrganizationID = strings.TrimSpace(c.Query("organization"))
	}
	if requestedOrganizationID != "" && requestedOrganizationID != apiKey.OrganizationID {
		handler.Fail(c.Writer, "API Key 不能访问其他企业")
		c.Abort()
		return
	}
	c.Request.Header.Set("X-Organization-ID", apiKey.OrganizationID)
	c.Request = c.Request.WithContext(service.WithUser(c.Request.Context(), user))
	c.Set("user_api_key_id", apiKey.ID)
	c.Next()
}

func OptionalAuth(c *gin.Context) {
	if user, ok := authUser(c); ok {
		c.Request = c.Request.WithContext(service.WithUser(c.Request.Context(), user))
	}
	c.Next()
}

func NotFoundJSON(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"code": 1, "data": nil, "msg": "接口不存在"})
}

func authUser(c *gin.Context) (model.AuthUser, bool) {
	token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	if strings.TrimSpace(token) == "" {
		token, _ = c.Cookie("infinite_canvas_session")
	}
	if strings.TrimSpace(token) == "" {
		return model.AuthUser{}, false
	}
	user, ok := service.CurrentAuthUser(token)
	if ok && strings.TrimSpace(c.GetHeader("Authorization")) != "" {
		maxAge := config.Cfg.JWTExpireHours * 3600
		if maxAge <= 0 {
			maxAge = 7 * 24 * 3600
		}
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie("infinite_canvas_session", token, maxAge, "/", "", c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https", true)
	}
	return user, ok
}

func OrganizationAuth(c *gin.Context) {
	user, ok := service.UserFromContext(c.Request.Context())
	if !ok {
		handler.Fail(c.Writer, "未登录或权限不足")
		c.Abort()
		return
	}
	organizationID := strings.TrimSpace(c.GetHeader("X-Organization-ID"))
	if organizationID == "" { organizationID = strings.TrimSpace(c.Query("organization")) }
	if organizationID != "" && user.OrganizationID == organizationID {
		c.Next()
		return
	}
	organization, _, err := service.ResolveOrganizationAccess(user, organizationID)
	if err != nil {
		handler.FailError(c.Writer, err)
		c.Abort()
		return
	}
	user.OrganizationID = organization.ID
	c.Request = c.Request.WithContext(service.WithUser(c.Request.Context(), user))
	c.Next()
}
