package routers

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/config"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/controllers"
	"github.com/rcy1314/echo-noise/internal/middleware"
	"github.com/rcy1314/echo-noise/pkg"
)

func registerAdminAuthorizationRoutes(authRoutes *gin.RouterGroup) {
	authRoutes.GET("/admin/authorization/me", middleware.AdminAuthMiddleware(), controllers.AuthorizationMe)
	authRoutes.GET("/admin/audit-config", middleware.RequireCapability(authorization.CapabilityAuthorizationManage), controllers.GetAdminAuditConfig)
	authRoutes.PUT("/admin/audit-config", middleware.RequireCapability(authorization.CapabilityAuthorizationManage), controllers.UpdateAdminAuditConfig)

	authorizationRoutes := authRoutes.Group("/admin/authorization")
	authorizationRoutes.Use(middleware.RequireCapability(authorization.CapabilityAuthorizationManage))
	{
		authorizationRoutes.GET("/catalog", controllers.AuthorizationCatalog)
		authorizationRoutes.GET("/admins", controllers.ListAuthorizationAdmins)
		authorizationRoutes.GET("/admins/:id", controllers.GetAuthorizationAdmin)
		authorizationRoutes.PUT("/admins/:id", controllers.ReplaceAuthorizationAdminGrants)
	}

	auditRoutes := authRoutes.Group("/admin/audit-logs")
	auditRoutes.Use(middleware.RequireCapability(authorization.CapabilityAuditView))
	{
		auditRoutes.GET("", controllers.ListAdminAuditLogs)
		auditRoutes.GET("/export", controllers.ExportAdminAuditLogs)
		auditRoutes.GET("/:id", controllers.GetAdminAuditLog)
	}
}

func registerLocalAttachmentRoute(r *gin.Engine, route string, kind string, dir string) {
	handler := controllers.ServeLocalAttachment(kind, dir)
	r.GET(route+"/*name", handler)
	r.HEAD(route+"/*name", handler)
}

func registerAttachmentManagementRoutes(authRoutes *gin.RouterGroup) {
	attachments := authRoutes.Group("/attachments")
	attachments.GET("/images", middleware.RequireCapability(authorization.CapabilityAttachmentsView), controllers.ListImageAttachments)
	attachments.GET("/images/", middleware.RequireCapability(authorization.CapabilityAttachmentsView), controllers.ListImageAttachments)
	attachments.GET("/video", middleware.RequireCapability(authorization.CapabilityAttachmentsView), controllers.ListVideoAttachments)
	attachments.GET("/video/", middleware.RequireCapability(authorization.CapabilityAttachmentsView), controllers.ListVideoAttachments)
	attachments.GET("/audio", middleware.RequireCapability(authorization.CapabilityAttachmentsView), controllers.ListAudioAttachments)
	attachments.GET("/audio/", middleware.RequireCapability(authorization.CapabilityAttachmentsView), controllers.ListAudioAttachments)
	attachments.GET("/other", middleware.RequireCapability(authorization.CapabilityAttachmentsView), controllers.ListOtherAttachments)
	attachments.GET("/other/", middleware.RequireCapability(authorization.CapabilityAttachmentsView), controllers.ListOtherAttachments)
	attachments.POST("/download-zip", middleware.RequireCapability(authorization.CapabilityAttachmentsDownload), controllers.DownloadAttachmentZip)
	attachments.DELETE("/references/:id", middleware.RequireCapability(authorization.CapabilityAttachmentsDeleteReference), controllers.DeleteAttachmentReference)
	attachments.POST("/references/batch-delete", middleware.RequireCapability(authorization.CapabilityAttachmentsDeleteReference), controllers.DeleteAttachmentReferencesBatch)
	attachments.POST("/references/batch-purge", middleware.RequireCapability(authorization.CapabilityAttachmentsPurgeBlob), controllers.PurgeAttachmentBlobsBatch)
	attachments.DELETE("/images/*name", middleware.RequireCapability(authorization.CapabilityAttachmentsDeleteReference), controllers.DeleteImageAttachment)
	attachments.DELETE("/video/*name", middleware.RequireCapability(authorization.CapabilityAttachmentsDeleteReference), controllers.DeleteVideoAttachment)
	attachments.DELETE("/audio/*name", middleware.RequireCapability(authorization.CapabilityAttachmentsDeleteReference), controllers.DeleteAudioAttachment)
	attachments.DELETE("/other/*name", middleware.RequireCapability(authorization.CapabilityAttachmentsDeleteReference), controllers.DeleteOtherAttachment)
}

func SetupRouter() *gin.Engine {
	r := gin.New()
	configureTrustedProxies(r)
	r.Use(gin.Recovery())
	r.Use(middleware.SecurityHeadersMiddleware())
	if enableAccessLog() {
		r.Use(gin.Logger())
	}
	// 支持大文件上传（视频压缩/直传云端可能超过 200MB）
	r.MaxMultipartMemory = 1024 << 20
	basePath := normalizeProxyPrefix(os.Getenv("BASE_PATH"))
	r.Use(func(c *gin.Context) {
		prefix := basePath
		if h := normalizeProxyPrefix(c.GetHeader("X-Forwarded-Prefix")); h != "" {
			prefix = h
		}
		if newPath, ok := stripProxyPrefix(c.Request.URL.Path, prefix); ok {
			c.Request.URL.Path = newPath
			c.Request.URL.RawPath = newPath
			if c.Request.RequestURI != "" {
				if q := c.Request.URL.RawQuery; q != "" {
					c.Request.RequestURI = newPath + "?" + q
				} else {
					c.Request.RequestURI = newPath
				}
			}
		}
		c.Next()
	})
	r.Use(staticResponseHeadersMiddleware())
	r.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{
		"/api/images/",
		"/api/video/",
		"/video/",
		"/api/audio/",
		"/api/files/",
		"/api/cloud-attachments/",
	})))

	// 安全防护：拦截敏感路径扫描（不影响正常 API/静态资源/MCP）
	r.Use(middleware.SecurityMiddleware())

	// 使用 pkg 中的 session 初始化
	pkg.InitSession(r)
	r.Use(middleware.AccessLogMiddleware())
	// 配置 CORS
	corsConfig := cors.DefaultConfig()
	// 动态按环境变量或放通所有来源（支持反代与跨域小组件）
	var allowList []string
	if origins := os.Getenv("CORS_ORIGINS"); origins != "" {
		for _, o := range strings.Split(origins, ",") {
			s := strings.TrimSpace(o)
			if s != "" {
				allowList = append(allowList, s)
			}
		}
	}
	if len(allowList) == 0 {
		corsConfig.AllowOriginFunc = func(origin string) bool { return true }
	} else {
		corsConfig.AllowOriginFunc = func(origin string) bool {
			for _, o := range allowList {
				if origin == o {
					return true
				}
			}
			return false
		}
	}
	corsConfig.AllowHeaders = []string{
		"Origin",
		"Content-Type",
		"X-Requested-With",
		"Accept",
		"Device-Type",
		"Authorization", // 新增授权头
		"Cache-Control",
		"Pragma",
		"Referer",
	}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	corsConfig.AllowCredentials = true
	corsConfig.MaxAge = 86400

	// 不再使用 AllowOrigins 列表，统一使用 AllowOriginFunc 做灵活匹配

	r.Use(cors.New(corsConfig))

	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	sp := strings.TrimRight(config.Config.Upload.SavePath, "/")
	imgDir := pickDir([]string{
		sp,
		"./" + sp,
		filepath.Join(wd, sp),
		filepath.Join(exeDir, sp),
		"./data/images",
		filepath.Join(wd, "data/images"),
		filepath.Join(exeDir, "data/images"),
		"/data/images",
		"/app/data/images",
	}, "./data/images")

	// 确定视频目录，优先查找存在的目录
	vidDir := pickDir([]string{
		"./data/video",
		filepath.Join(wd, "data/video"),
		filepath.Join(exeDir, "data/video"),
		"/data/video",
		"/app/data/video",
	}, "./data/video")

	// 确保目录存在（如果都找不到，就用默认的 ./data/video 并创建）
	if _, err := os.Stat(vidDir); os.IsNotExist(err) {
		os.MkdirAll(vidDir, 0755)
	}

	// 确定音频目录，优先查找存在的目录
	audioDir := pickDir([]string{
		"./data/audio",
		filepath.Join(wd, "data/audio"),
		filepath.Join(exeDir, "data/audio"),
		"/data/audio",
		"/app/data/audio",
	}, "./data/audio")
	if _, err := os.Stat(audioDir); os.IsNotExist(err) {
		os.MkdirAll(audioDir, 0755)
	}

	attachmentDir := pickDir([]string{
		"./data/attachments",
		filepath.Join(wd, "data/attachments"),
		filepath.Join(exeDir, "data/attachments"),
		"/data/attachments",
		"/app/data/attachments",
	}, "./data/attachments")
	if _, err := os.Stat(attachmentDir); os.IsNotExist(err) {
		os.MkdirAll(attachmentDir, 0755)
	}

	registerLocalAttachmentRoute(r, "/api/images", "image", imgDir)
	// 同时支持 /api/video 和 /video，兼容旧版路径和 API 规范
	registerLocalAttachmentRoute(r, "/api/video", "video", vidDir)
	registerLocalAttachmentRoute(r, "/video", "video", vidDir)
	registerLocalAttachmentRoute(r, "/api/audio", "audio", audioDir)
	registerLocalAttachmentRoute(r, "/api/files", "file", attachmentDir)
	r.GET("/api/cloud-attachments/:id/*name", controllers.ServeCloudAttachment)
	r.HEAD("/api/cloud-attachments/:id/*name", controllers.ServeCloudAttachment)
	// 常用静态文件已在上方映射

	// API 路由组
	api := r.Group("/api")

	// 消息详情页路由（移到 API 组外）
	r.GET("/m/:id", controllers.GetMessagePage)

	// RSS 路由保留在 API 组外，禁用时仍返回显式 404，避免旧链接落入前端兜底页。
	r.GET("/rss", controllers.GenerateRSS)
	api.POST("/rss/refresh", middleware.SessionAuthMiddleware(), controllers.RefreshRSS)

	// 公共路由
	api.GET("", controllers.GetStatus)
	api.GET("/frontend/config", controllers.GetFrontendConfig)
	api.GET("/settings", controllers.GetFrontendConfig)
	api.GET("/announcements", controllers.ListAnnouncements)
	api.GET("/announcements/unread", controllers.GetUnreadAnnouncements)
	api.PUT("/announcements/read-all", controllers.MarkAllAnnouncementsRead)
	api.PUT("/announcements/:id/read", controllers.MarkAnnouncementRead)
	api.GET("/feed/items", controllers.GetInfoFeedItems)
	api.POST("/feed/refresh", controllers.RefreshPublicInfoFeedItems)
	api.POST("/login", controllers.Login)
	api.POST("/register", controllers.Register)
	api.GET("/status", controllers.GetStatus)
	api.GET("/users/profile", controllers.GetUserProfile)
	api.GET("/captcha", controllers.GetCaptcha)
	// api.GET("/config", controllers.GetFrontendConfig)
	api.GET("/messages", controllers.GetMessages)
	api.POST("/messages/locate", controllers.LocateMessagePage)
	api.GET("/messages/locate", controllers.LocateMessagePage)
	api.POST("/messages/page", controllers.GetMessagesByPage)
	api.GET("/messages/page", controllers.GetMessagesByPage)
	api.GET("/messages/:id", controllers.GetMessage)
	api.GET("/messages/calendar", controllers.GetMessagesCalendar) // 新增热力图专用路由
	api.GET("/messages/search", controllers.SearchMessages)        // 新增搜索消息路由
	api.GET("/version/check", controllers.CheckVersion)            // 添加版本检查路由
	api.GET("/version", controllers.GetVersion)                    // 当前运行版本（镜像标签/环境变量）
	api.GET("/version/runtime", controllers.GetRuntimeEnv)
	// 版本更新（管理员）将在下方统一 authRoutes 组中注册
	// GitHub OAuth
	api.GET("/oauth/github/login", controllers.GithubLogin)
	r.GET("/oauth/github/callback", controllers.GithubCallback)
	api.POST("/password/forgot", controllers.PasswordForgot)

	// 添加标签和图像相关路由
	api.GET("/messages/tags/:tag", controllers.GetMessagesByTag)         // 获取指定标签的消息
	api.GET("/messages/tags", controllers.GetAllTags)                    // 获取所有标签列表
	api.GET("/messages/images", controllers.GetAllImages)                // 获取所有图片列表
	api.POST("/messages/:id/like", controllers.IncrementMessageLike)     // 点赞接口
	api.POST("/messages/:id/like/toggle", controllers.ToggleMessageLike) // 点赞切换
	api.GET("/guestbook/message", controllers.GetGuestbookMessageID)     // 获取留言板消息ID
	// 友链申请（公开）
	api.POST("/friend-links/apply", controllers.SubmitFriendLinkApply)
	api.GET("/douyin/resolve", controllers.ResolveDouyinShortURL)
	api.GET("/douyin/play", controllers.ProxyDouyinVideo)

	// 需要鉴权的路由
	authRoutes := api.Group("")
	authRoutes.Use(middleware.SessionAuthMiddleware())
	registerAdminAuthorizationRoutes(authRoutes)
	authRoutes.GET("/users/me/stats", controllers.GetCurrentUserHomeStats)
	// 版本更新（管理员）
	authRoutes.POST("/version/update", middleware.RequireCapability(authorization.CapabilityVersionUpdate), controllers.UpdateVersion)
	authRoutes.GET("/version/update/stream", middleware.RequireCapability(authorization.CapabilityVersionUpdate), controllers.UpdateVersionStream)
	authRoutes.POST("/version/static-sync", middleware.RequireCapability(authorization.CapabilityVersionUpdate), controllers.SyncStatic)
	// 静态资源同步已移除，版本升级统一走容器镜像

	// 添加 token 认证的路由组
	tokenAuth := api.Group("/token")
	tokenAuth.Use(middleware.TokenAuthMiddleware()) // 使用 TokenAuthMiddleware
	{
		tokenAuth.POST("/messages", controllers.PostMessage)
		tokenAuth.PUT("/messages/:id", controllers.UpdateMessage)
		tokenAuth.PUT("/messages/:id/pin", controllers.UpdateMessagePinned)
		tokenAuth.PUT("/messages/:id/pin/global", controllers.UpdateMessageGlobalPin)
		tokenAuth.PUT("/messages/:id/pin/personal", controllers.UpdateMessagePersonalPin)
		tokenAuth.DELETE("/messages/:id", controllers.DeleteMessage)
		tokenAuth.PUT("/settings", middleware.RequireCapability(authorization.CapabilitySiteSettingsManage), controllers.UpdateSetting)
	}
	// 需要鉴权的消息操作路由
	messages := authRoutes.Group("/messages")
	{
		messages.POST("", controllers.PostMessage)
		messages.PUT("/:id", controllers.UpdateMessage)
		messages.PUT("/:id/pin", controllers.UpdateMessagePinned)
		messages.PUT("/:id/pin/global", controllers.UpdateMessageGlobalPin)
		messages.PUT("/:id/pin/personal", controllers.UpdateMessagePersonalPin)
		messages.DELETE("/:id", controllers.DeleteMessage)
	}

	// 评论系统（内置）公共路由
	api.GET("/messages/:id/comments", controllers.GetComments)
	api.POST("/messages/comments/counts", controllers.GetCommentCounts)
	api.POST("/messages/:id/comments", controllers.PostComment)
	// 管理员评论列表（提供公共路径，附加会话中间件以注入用户上下文）
	api.GET("/comments", middleware.SessionAuthMiddleware(), middleware.RequireCapability(authorization.CapabilityCommentsView), controllers.ListComments)
	// 评论更新/删除：管理员可管理全部，普通用户可管理自己发布的内容
	authRoutes.PUT("/messages/:id/comments/:cid", controllers.UpdateComment)
	authRoutes.DELETE("/messages/:id/comments/:cid", controllers.DeleteComment)
	// 管理员评论列表管理（已在公共组注册路径，函数内部鉴权）
	// 添加推送配置路由
	notify := authRoutes.Group("/notify")
	{
		notify.POST("/test", middleware.RequireCapability(authorization.CapabilityNotificationsManage), controllers.TestNotify)
		notify.POST("/send", middleware.RequireCapability(authorization.CapabilityNotificationsManage), controllers.SendNotify)
		notify.GET("/config", middleware.RequireCapability(authorization.CapabilityNotificationsView), controllers.GetNotifyConfig)
		notify.PUT("/config", middleware.RequireCapability(authorization.CapabilityNotificationsManage), controllers.SaveNotifyConfig)
	}

	// 站内通知路由
	notifications := authRoutes.Group("/notifications")
	{
		notifications.GET("", controllers.ListUserNotifications)
		notifications.GET("/unread-count", controllers.GetUserNotificationUnreadCount)
		notifications.PUT("/read-all", controllers.MarkAllUserNotificationsRead)
		notifications.PUT("/read/:id", controllers.MarkUserNotificationRead)
	}

	announcementAdmin := authRoutes.Group("/admin/announcements")
	announcementAdmin.Use(middleware.RequireCapability(authorization.CapabilityAnnouncementsView))
	{
		announcementAdmin.GET("", controllers.ListAdminAnnouncements)
		announcementAdmin.POST("", middleware.RequireCapability(authorization.CapabilityAnnouncementsManage), controllers.CreateAnnouncement)
		announcementAdmin.PUT("/:id", middleware.RequireCapability(authorization.CapabilityAnnouncementsManage), controllers.UpdateAnnouncement)
		announcementAdmin.POST("/:id/publish", middleware.RequireCapability(authorization.CapabilityAnnouncementsPush), controllers.PublishAnnouncement)
		announcementAdmin.POST("/:id/withdraw", middleware.RequireCapability(authorization.CapabilityAnnouncementsManage), controllers.WithdrawAnnouncement)
		announcementAdmin.DELETE("/:id", middleware.RequireCapability(authorization.CapabilityAnnouncementsManage), controllers.DeleteAnnouncement)
		announcementAdmin.POST("/batch-delete", middleware.RequireCapability(authorization.CapabilityAnnouncementsManage), controllers.BatchDeleteAnnouncements)
		announcementAdmin.GET("/:id/push-summary", controllers.GetAnnouncementPushSummary)
		announcementAdmin.POST("/:id/retry-push", controllers.RetryAnnouncementPush)
	}

	email := authRoutes.Group("/email")
	{
		email.POST("/test", middleware.RequireCapability(authorization.CapabilityEmailManage), controllers.EmailTest)
	}

	// 数据库备份相关路由
	backup := authRoutes.Group("/backup")
	{
		backup.GET("/download", middleware.RequireCapability(authorization.CapabilityDatabaseBackup), controllers.HandleBackupDownload)
		backup.POST("/restore", middleware.RequireCapability(authorization.CapabilityDatabaseRestore), controllers.HandleBackupRestore)
		backup.POST("/storage/upload", middleware.RequireCapability(authorization.CapabilityDatabaseBackup), controllers.HandleBackupUploadToURL)
		backup.POST("/storage/restore", middleware.RequireCapability(authorization.CapabilityDatabaseRestore), controllers.HandleBackupRestoreFromURL)
		backup.POST("/storage/presign/upload", middleware.RequireCapability(authorization.CapabilityDatabaseBackup), controllers.HandleBackupPresignUpload)
		backup.POST("/storage/presign/download", middleware.RequireCapability(authorization.CapabilityDatabaseRestore), controllers.HandleBackupPresignDownload)
		backup.POST("/storage/sync-confirm", middleware.RequireCapability(authorization.CapabilityStorageManage), controllers.HandleBackupSyncConfirm)
		backup.POST("/storage/sync-now", middleware.RequireCapability(authorization.CapabilityStorageManage), controllers.HandleBackupSyncNow)
	}

	// 系统设置相关路由
	settings := authRoutes.Group("/settings")
	{
		settings.POST("/reset-defaults", middleware.RequireCapability(authorization.CapabilitySiteSettingsManage), controllers.ResetDefaultData)
	}

	// 安全记录（管理员）
	security := authRoutes.Group("/security")
	{
		security.GET("/attacks", middleware.RequireCapability(authorization.CapabilitySecurityView), controllers.GetAttackRecords)
		security.DELETE("/attacks", middleware.RequireCapability(authorization.CapabilitySecurityClearLogs), controllers.ClearAttackRecords)
		security.GET("/access-logs", middleware.RequireCapability(authorization.CapabilityAccessLogsView), controllers.GetAccessLogs)
		security.DELETE("/access-logs", middleware.RequireCapability(authorization.CapabilityAccessLogsClear), controllers.ClearAccessLogs)
		security.GET("/site-visits", middleware.RequireCapability(authorization.CapabilitySiteVisitsView), controllers.GetSiteVisits)
		security.DELETE("/site-visits", middleware.RequireCapability(authorization.CapabilitySiteVisitsClear), controllers.ClearSiteVisits)
		security.GET("/login-audits", middleware.RequireCapability(authorization.CapabilityLoginAuditsView), controllers.GetLoginAudits)
		security.GET("/bans", middleware.RequireCapability(authorization.CapabilitySecurityView), controllers.GetIPBans)
		security.POST("/bans", middleware.RequireCapability(authorization.CapabilitySecurityManage), controllers.AddIPBan)
		security.DELETE("/bans", middleware.RequireCapability(authorization.CapabilitySecurityManage), controllers.RemoveIPBan)
		security.GET("/config", middleware.RequireCapability(authorization.CapabilitySecurityView), controllers.GetSecurityConfig)
		security.PUT("/config", middleware.RequireCapability(authorization.CapabilitySecurityManage), controllers.UpdateSecurityConfig)
	}

	// 图片上传路由
	authRoutes.POST("/images/upload", controllers.UploadImage) // 上传图片
	// 新增：视频上传路由（改为单数 video）
	authRoutes.POST("/video/upload", controllers.UploadVideo)            // 上传视频
	authRoutes.POST("/audio/upload", controllers.UploadAudio)            // 上传音频
	authRoutes.POST("/attachments/upload", controllers.UploadAttachment) // 上传通用附件

	// 附件管理路由
	registerAttachmentManagementRoutes(authRoutes)

	// 用户相关路由
	user := authRoutes.Group("/user")
	{
		user.GET("", controllers.GetUserInfo)
		user.PUT("/change_password", controllers.ChangePassword)
		user.PUT("/update", controllers.UpdateUser)
		user.PUT("/admin", middleware.RequireCapability(authorization.CapabilityAdminRolesManage), controllers.UpdateUserAdmin)
		user.DELETE("", middleware.RequireCapability(authorization.CapabilityUsersDelete), controllers.DeleteUser)
		user.POST("/logout", controllers.Logout) // 添加退出登录路由
		user.POST("/reset_password", middleware.RequireCapability(authorization.CapabilityUsersResetPassword), controllers.AdminResetPassword)
		// 添加 Token 相关路由
		user.GET("/token", controllers.GetUserToken)
		user.POST("/token/regenerate", controllers.RegenerateUserToken)
		user.POST("/email/bind", controllers.BindEmail)
		user.POST("/email/verify", controllers.VerifyEmail)
		user.POST("/email/change/send_code", controllers.SendChangeEmailCode)
		user.POST("/email/change", controllers.ChangeEmail)
	}

	// 注册申请管理（管理员）
	registration := authRoutes.Group("/registration")
	{
		registration.GET("/applications", middleware.RequireCapability(authorization.CapabilityRegistrationView), controllers.ListRegistrationApplications)
		registration.PUT("/applications/:id/approve", middleware.RequireCapability(authorization.CapabilityRegistrationReview), controllers.ApproveRegistrationApplication)
		registration.PUT("/applications/:id/reject", middleware.RequireCapability(authorization.CapabilityRegistrationReview), controllers.RejectRegistrationApplication)
	}

	// 设置路由
	authRoutes.PUT("/settings", middleware.RequireCapability(authorization.CapabilitySiteSettingsManage), controllers.UpdateSetting)
	authRoutes.POST("/settings/vocechat/health", middleware.RequireCapability(authorization.CapabilityAuthorizationManage), controllers.CheckVoceChatHealth)
	// 友链申请管理（管理员）
	authRoutes.GET("/friend-links/apply", middleware.RequireCapability(authorization.CapabilitySiteSettingsView), controllers.ListFriendLinkApplications)
	authRoutes.DELETE("/friend-links/apply", middleware.RequireCapability(authorization.CapabilitySiteSettingsManage), controllers.ClearFriendLinkApplications)
	authRoutes.DELETE("/friend-links/apply/:id", middleware.RequireCapability(authorization.CapabilitySiteSettingsManage), controllers.DeleteFriendLinkApplication)
	authRoutes.PUT("/friend-links/:id/audit", middleware.RequireCapability(authorization.CapabilitySiteSettingsManage), controllers.AuditFriendLink)

	// 显式 /status 返回 SPA 入口，避免目录重定向影响
	r.GET("/status", func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.File("./public/index.html")
	})

	// 先映射关键静态文件，避免被 SPA Fallback 覆盖
	// Service Worker 文件路由 - 必须注册以支持 PWA
	r.StaticFile("/sw.js", "./public/sw.js")
	// manifest 路由（提供 API 版本以避免静态中间件干扰）
	r.GET("/manifest.json", controllers.GetWebManifest)
	r.GET("/manifest.webmanifest", controllers.GetWebManifest)
	r.GET("/api/manifest", controllers.GetWebManifest)

	// 显式映射 Nuxt 资源目录和常用静态
	r.Static("/_nuxt", "./public/_nuxt")
	r.Static("/assets", "./public/assets")

	r.Use(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}
		p := c.Request.URL.Path
		if p == "/" || p == "/api" || strings.HasPrefix(p, "/api/") {
			c.Next()
			return
		}
		cleanPath := filepath.Clean("/" + p)
		if cleanPath == "/" || strings.Contains(cleanPath, "..") {
			c.Next()
			return
		}
		filePath := filepath.Join("./public", strings.TrimPrefix(cleanPath, "/"))
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			c.File(filePath)
			c.Abort()
			return
		}
		c.Next()
	})

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/m/") ||
			strings.HasPrefix(path, "/messages/") ||
			path == "/" ||
			!strings.HasPrefix(path, "/api") {
			// 禁止入口页缓存，确保最新静态资源被加载
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
			c.File("./public/index.html")
		} else {
			c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "接口不存在"})
		}
	})

	return r
}

func staticResponseHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		switch {
		case path == "/manifest.webmanifest" || path == "/manifest.json":
			c.Header("Content-Type", "application/manifest+json; charset=utf-8")
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		case path == "/sw.js":
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		case strings.HasPrefix(path, "/_nuxt/") || strings.HasPrefix(path, "/assets/") ||
			strings.HasPrefix(path, "/favicon") || strings.HasPrefix(path, "/android-chrome"):
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		}
		c.Next()
	}
}

func configureTrustedProxies(r *gin.Engine) {
	raw := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES"))
	if raw == "" {
		_ = r.SetTrustedProxies(nil)
		return
	}

	proxies := make([]string, 0)
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			proxies = append(proxies, value)
		}
	}
	if len(proxies) == 0 {
		_ = r.SetTrustedProxies(nil)
		return
	}
	if err := r.SetTrustedProxies(proxies); err != nil {
		_ = r.SetTrustedProxies(nil)
		log.Printf("TRUSTED_PROXIES 配置无效，已禁用代理转发头信任: %v", err)
	}
}

func enableAccessLog() bool {
	v := strings.TrimSpace(os.Getenv("ACCESS_LOG"))
	if v != "" {
		v = strings.ToLower(v)
		return v == "1" || v == "true" || v == "yes" || v == "on"
	}
	mode := strings.ToLower(strings.TrimSpace(gin.Mode()))
	// 生产默认关闭访问日志；开发环境默认开启，便于调试
	return mode != gin.ReleaseMode
}

func pickDir(candidates []string, fallback string) string {
	for _, d := range candidates {
		if d == "" {
			continue
		}
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			return d
		}
	}
	return fallback
}

func normalizeProxyPrefix(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.Index(v, ","); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	if v == "" || v == "/" {
		return ""
	}
	if !strings.HasPrefix(v, "/") {
		v = "/" + v
	}
	v = strings.TrimRight(v, "/")
	if v == "" || v == "/" {
		return ""
	}
	return v
}

func stripProxyPrefix(path, prefix string) (string, bool) {
	if prefix == "" || path == "" {
		return path, false
	}
	if path == prefix {
		return "/", true
	}
	mark := prefix + "/"
	if strings.HasPrefix(path, mark) {
		trimmed := strings.TrimPrefix(path, prefix)
		if trimmed == "" {
			return "/", true
		}
		return trimmed, true
	}
	return path, false
}
