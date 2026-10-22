package router

import (
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/yypyyd/infinite-canvas/gateway/internal/http/handler"
)

// Handlers is the gateway data plane plus its admin control plane. The admin
// plane has no authentication of its own: the caller mounts it behind 幻图's
// super-admin middleware.
type Handlers struct {
	Health      *handler.HealthHandler
	V1          *handler.V1Handler
	Admin       *handler.AdminConsoleHandler
	BannedWords *handler.BannedWordsHandler
}

// MountV1 registers the OpenAI-compatible routes on group. It is only used on
// the in-process listener; auth attaches the built-in principal.
func MountV1(v1 *gin.RouterGroup, auth gin.HandlerFunc, h Handlers) {
	v1.Use(cors.New(cors.Config{
		AllowOriginFunc:  func(origin string) bool { return strings.TrimSpace(origin) != "" },
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type", "Idempotency-Key", "Prefer", "Range", "X-Request-Id"},
		ExposeHeaders:    []string{"Accept-Ranges", "Content-Length", "Content-Range", "Idempotency-Key", "Location", "Preference-Applied", "Retry-After", "X-Request-Id"},
		AllowCredentials: false,
	}))
	v1.OPTIONS("/*path", preflight)
	v1.Use(auth)
	v1.GET("/models", h.V1.Models)
	v1.POST("/chat/completions", h.V1.ChatCompletions)
	v1.POST("/images/generations", h.V1.ImageGenerations)
	v1.POST("/images/edits", h.V1.ImageEdits)
	v1.GET("/images/tasks", h.V1.GetImageTask)
	v1.GET("/images/:id/content", h.V1.GetImageContent)
	v1.POST("/videos", h.V1.CreateVideo)
	v1.GET("/videos/:id", h.V1.GetVideo)
	v1.GET("/videos/:id/content", h.V1.GetVideoContent)
}

// MountAdmin registers the control plane (formerly /admin/api/*) on group.
func MountAdmin(admin *gin.RouterGroup, h Handlers) {
	admin.GET("/health/live", h.Health.Live)
	admin.GET("/health/ready", h.Health.Ready)

	admin.GET("/logical-models", h.Admin.LogicalModels)
	admin.PATCH("/logical-models/:id", h.Admin.UpdateLogicalModel)
	admin.PATCH("/logical-models/:id/routes/:route_id", h.Admin.UpdateLogicalRoute)
	admin.GET("/overview", h.Admin.Overview)

	admin.GET("/accounts", h.Admin.Accounts)
	admin.POST("/accounts/import", h.Admin.ImportAccount)
	admin.POST("/accounts/delete-dead", h.Admin.DeleteDeadAccounts)
	admin.POST("/accounts/batch-status", h.Admin.BatchUpdateAccounts)
	admin.POST("/providers/:provider/probe", h.Admin.ProbeProvider)
	admin.PATCH("/accounts/:id", h.Admin.UpdateAccount)
	admin.DELETE("/accounts/:id", h.Admin.DeleteAccount)
	admin.PATCH("/accounts/:id/routes/:binding_id", h.Admin.SetAccountRoute)
	admin.POST("/accounts/:id/refresh-quota", h.Admin.RefreshAccountQuota)
	admin.POST("/test", h.V1.AdminTest)
	admin.GET("/test/events/:id", h.V1.AdminTestStatus)
	admin.GET("/test/artifacts/:id", h.V1.AdminTestArtifact)

	admin.GET("/logs", h.Admin.Logs)
	admin.GET("/artifacts", h.Admin.Artifacts)
	admin.GET("/artifacts/:id/content", h.V1.AdminArtifact)
	admin.GET("/settings", h.Admin.Settings)
	admin.PUT("/settings", h.Admin.UpdateSettings)

	admin.GET("/banned-words", h.BannedWords.List)
	admin.POST("/banned-words", h.BannedWords.Create)
	admin.POST("/banned-words/import", h.BannedWords.Import)
	admin.DELETE("/banned-words/:id", h.BannedWords.Delete)
	admin.GET("/banned-word-hits", h.BannedWords.Hits)
}

func preflight(c *gin.Context) {
	c.Status(204)
}
