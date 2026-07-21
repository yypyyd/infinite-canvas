package middleware

import (
	"net/http"
	"strings"

	"github.com/basketikun/infinite-canvas/config"
	"github.com/basketikun/infinite-canvas/handler"
	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/service"
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
