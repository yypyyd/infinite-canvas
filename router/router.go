package router

import (
	"net/http"

	"github.com/basketikun/infinite-canvas/handler"
	"github.com/basketikun/infinite-canvas/middleware"
	"github.com/gin-gonic/gin"
)

func New() *gin.Engine {
	router := gin.Default()
	router.RedirectTrailingSlash = false
	_ = router.SetTrustedProxies(nil)
	api := router.Group("/api")
	api.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	api.POST("/auth/register", gin.WrapF(handler.Register))
	api.POST("/auth/email-code", gin.WrapF(handler.SendRegistrationEmailCode))
	api.POST("/auth/login", gin.WrapF(handler.Login))
	api.POST("/auth/logout", gin.WrapF(handler.Logout))
	api.GET("/auth/me", middleware.OptionalAuth, gin.WrapF(handler.CurrentUser))
	api.POST("/auth/profile", middleware.UserAuth, gin.WrapF(handler.UpdateProfile))
	api.POST("/auth/password", middleware.UserAuth, gin.WrapF(handler.ChangePassword))
	api.GET("/credit-logs", middleware.UserAuth, gin.WrapF(handler.UserCreditLogs))
	api.GET("/generation-tasks", middleware.UserAuth, middleware.OrganizationAuth, gin.WrapF(handler.UserGenerationTasks))
	api.GET("/workspace", middleware.UserAuth, middleware.OrganizationAuth, gin.WrapF(handler.UserWorkspace))
	api.POST("/workspace/changes", middleware.UserAuth, middleware.OrganizationAuth, gin.WrapF(handler.SaveUserWorkspace))
	api.POST("/workspace/files/upload-ticket", middleware.UserAuth, middleware.OrganizationAuth, gin.WrapF(handler.PrepareUserWorkspaceFileUpload))
	api.POST("/workspace/files/confirm", middleware.UserAuth, middleware.OrganizationAuth, gin.WrapF(handler.ConfirmUserWorkspaceFileUpload))
	api.GET("/workspace/files/:storageKey", middleware.UserAuth, middleware.OrganizationAuth, func(c *gin.Context) {
		handler.UserWorkspaceFile(c.Writer, c.Request, c.Param("storageKey"))
	})
	api.HEAD("/workspace/files/:storageKey", middleware.UserAuth, middleware.OrganizationAuth, func(c *gin.Context) {
		handler.UserWorkspaceFile(c.Writer, c.Request, c.Param("storageKey"))
	})
	api.GET("/workspace/status", middleware.UserAuth, middleware.OrganizationAuth, gin.WrapF(handler.UserStorageStatus))
	api.GET("/preferences", middleware.UserAuth, gin.WrapF(handler.UserPreferences))
	api.POST("/preferences", middleware.UserAuth, gin.WrapF(handler.SaveUserPreferences))
	commerce := api.Group("/commerce", middleware.UserAuth, middleware.OrganizationAuth)
	commerce.GET("/workspace", gin.WrapF(handler.CommerceWorkspace))
	commerce.POST("/organizations", gin.WrapF(handler.CreateOrganization))
	commerce.POST("/organizations/current", gin.WrapF(handler.UpdateOrganization))
	commerce.POST("/organizations/switch", gin.WrapF(handler.SwitchOrganization))
	commerce.GET("/invitations", gin.WrapF(handler.PendingOrganizationInvitations))
	commerce.GET("/organization-invitations", gin.WrapF(handler.CurrentOrganizationInvitations))
	commerce.POST("/invitations", gin.WrapF(handler.InviteOrganizationMember))
	commerce.POST("/invitations/:id/accept", func(c *gin.Context) { handler.AcceptOrganizationInvitation(c.Writer, c.Request, c.Param("id")) })
	commerce.DELETE("/invitations/:id", func(c *gin.Context) { handler.RevokeOrganizationInvitation(c.Writer, c.Request, c.Param("id")) })
	commerce.GET("/members", gin.WrapF(handler.CommerceOrganizationMembers))
	commerce.POST("/members/:id", func(c *gin.Context) { handler.UpdateOrganizationMember(c.Writer, c.Request, c.Param("id")) })
	commerce.DELETE("/members/:id", func(c *gin.Context) { handler.RemoveOrganizationMember(c.Writer, c.Request, c.Param("id")) })
	commerce.POST("/members/:id/transfer-owner", func(c *gin.Context) { handler.TransferOrganizationOwnership(c.Writer, c.Request, c.Param("id")) })
	commerce.GET("/brands", gin.WrapF(handler.CommerceBrands))
	commerce.POST("/brands", gin.WrapF(handler.SaveCommerceBrand))
	commerce.DELETE("/brands/:id", func(c *gin.Context) { handler.DeleteCommerceBrand(c.Writer, c.Request, c.Param("id")) })
	commerce.GET("/products", gin.WrapF(handler.CommerceProducts))
	commerce.POST("/products", gin.WrapF(handler.SaveCommerceProduct))
	commerce.DELETE("/products/:id", func(c *gin.Context) { handler.DeleteCommerceProduct(c.Writer, c.Request, c.Param("id")) })
	commerce.GET("/products/:id/skus", func(c *gin.Context) { handler.CommerceProductSKUs(c.Writer, c.Request, c.Param("id")) })
	commerce.POST("/skus", gin.WrapF(handler.SaveCommerceProductSKU))
	commerce.DELETE("/skus/:id", func(c *gin.Context) { handler.DeleteCommerceProductSKU(c.Writer, c.Request, c.Param("id")) })
	commerce.GET("/batch-jobs", gin.WrapF(handler.CommerceBatchJobs))
	commerce.POST("/batch-jobs", gin.WrapF(handler.CreateCommerceBatchJob))
	commerce.GET("/batch-jobs/:id/items", func(c *gin.Context) { handler.CommerceBatchItems(c.Writer, c.Request, c.Param("id")) })
	commerce.POST("/batch-jobs/:id/cancel", func(c *gin.Context) { handler.CancelCommerceBatchJob(c.Writer, c.Request, c.Param("id")) })
	commerce.POST("/batch-jobs/:id/retry", func(c *gin.Context) { handler.RetryCommerceBatchJob(c.Writer, c.Request, c.Param("id")) })
	commerce.GET("/audit-logs", gin.WrapF(handler.CommerceAuditLogs))
	api.GET("/settings", gin.WrapF(handler.Settings))
	api.GET("/check-in", middleware.UserAuth, gin.WrapF(handler.CheckInStatus))
	api.POST("/check-in", middleware.UserAuth, gin.WrapF(handler.CheckIn))
	api.POST("/redeem-codes/redeem", middleware.UserAuth, gin.WrapF(handler.RedeemCode))
	api.GET("/media/references/:id", func(c *gin.Context) {
		handler.ReferenceMedia(c.Writer, c.Request, c.Param("id"))
	})
	api.HEAD("/media/references/:id", func(c *gin.Context) {
		handler.ReferenceMedia(c.Writer, c.Request, c.Param("id"))
	})
	api.GET("/asset-files/:name", func(c *gin.Context) {
		handler.AssetFile(c.Writer, c.Request, c.Param("name"))
	})
	api.HEAD("/asset-files/:name", func(c *gin.Context) {
		handler.AssetFile(c.Writer, c.Request, c.Param("name"))
	})
	v1 := api.Group("/v1", middleware.UserAuth, middleware.OrganizationAuth)
	v1.POST("/images/generations", gin.WrapF(handler.AIImagesGenerations))
	v1.POST("/images/edits", gin.WrapF(handler.AIImagesEdits))
	v1.POST("/chat/completions", gin.WrapF(handler.AIChatCompletions))
	v1.POST("/audio/speech", gin.WrapF(handler.AIAudioSpeech))
	v1.POST("/videos", gin.WrapF(handler.AIVideos))
	v1.POST("/media/references", gin.WrapF(handler.UploadReferenceMedia))
	v1.GET("/videos/:id", func(c *gin.Context) {
		handler.AIVideo(c.Writer, c.Request, c.Param("id"))
	})
	v1.GET("/videos/:id/content", func(c *gin.Context) {
		handler.AIVideoContent(c.Writer, c.Request, c.Param("id"))
	})
	api.GET("/prompts", middleware.OptionalAuth, gin.WrapF(handler.Prompts))
	api.GET("/assets", middleware.OptionalAuth, gin.WrapF(handler.Assets))
	api.POST("/admin/login", gin.WrapF(handler.AdminLogin))

	admin := api.Group("/admin", middleware.AdminAuth)
	admin.GET("/users", gin.WrapF(handler.AdminUsers))
	admin.GET("/dashboard", gin.WrapF(handler.AdminDashboard))
	admin.GET("/generation-tasks", gin.WrapF(handler.AdminGenerationTasks))
	admin.POST("/users", gin.WrapF(handler.AdminSaveUser))
	admin.POST("/users/:id/credits", func(c *gin.Context) {
		handler.AdminAdjustUserCredits(c.Writer, c.Request, c.Param("id"))
	})
	admin.DELETE("/users/:id", func(c *gin.Context) {
		handler.AdminDeleteUser(c.Writer, c.Request, c.Param("id"))
	})
	admin.GET("/credit-logs", gin.WrapF(handler.AdminCreditLogs))
	admin.POST("/credit-logs", gin.WrapF(handler.AdminSaveCreditLog))
	admin.DELETE("/credit-logs/:id", func(c *gin.Context) {
		handler.AdminDeleteCreditLog(c.Writer, c.Request, c.Param("id"))
	})
	admin.GET("/redeem-codes", gin.WrapF(handler.AdminRedemptionCodes))
	admin.POST("/redeem-codes/generate", gin.WrapF(handler.AdminGenerateRedemptionCodes))
	admin.POST("/redeem-codes/batch-delete", gin.WrapF(handler.AdminDeleteRedemptionCodes))
	admin.DELETE("/redeem-codes/:id", func(c *gin.Context) {
		handler.AdminDeleteRedemptionCode(c.Writer, c.Request, c.Param("id"))
	})
	admin.GET("/settings", gin.WrapF(handler.AdminSettings))
	admin.POST("/settings", gin.WrapF(handler.AdminSaveSettings))
	admin.POST("/settings/channel-models", gin.WrapF(handler.AdminChannelModels))
	admin.POST("/settings/channel-test", gin.WrapF(handler.AdminTestChannelModel))
	admin.GET("/prompt-categories", gin.WrapF(handler.AdminPromptCategories))
	admin.POST("/prompt-categories/sync", gin.WrapF(handler.AdminSyncPromptCategories))
	admin.GET("/prompts", gin.WrapF(handler.AdminPrompts))
	admin.POST("/prompts", gin.WrapF(handler.AdminSavePrompt))
	admin.POST("/prompts/batch-delete", gin.WrapF(handler.AdminDeletePrompts))
	admin.DELETE("/prompts/:id", func(c *gin.Context) {
		handler.AdminDeletePrompt(c.Writer, c.Request, c.Param("id"))
	})
	admin.GET("/assets", gin.WrapF(handler.AdminAssets))
	admin.POST("/assets", gin.WrapF(handler.AdminSaveAsset))
	admin.POST("/asset-files", gin.WrapF(handler.AdminUploadAssetFile))
	admin.DELETE("/assets/:id", func(c *gin.Context) {
		handler.AdminDeleteAsset(c.Writer, c.Request, c.Param("id"))
	})

	router.NoRoute(middleware.NotFoundJSON)

	return router
}
