package routers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/config"
	"github.com/rcy1314/echo-noise/internal/controllers"
	"github.com/rcy1314/echo-noise/internal/middleware"
	"github.com/rcy1314/echo-noise/pkg"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()
	// 支持大文件上传（视频压缩/直传云端可能超过 200MB）
	r.MaxMultipartMemory = 1024 << 20

	// 安全防护：拦截敏感路径扫描（不影响正常 API/静态资源/MCP）
	r.Use(middleware.SecurityMiddleware())

	// 使用 pkg 中的 session 初始化
	pkg.InitSession(r)
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

	r.Static("/api/images", imgDir)
	// 同时支持 /api/video 和 /video，兼容旧版路径和 API 规范
	r.Static("/api/video", vidDir)
	r.Static("/video", vidDir)
	// 常用静态文件已在上方映射

	// API 路由组
	api := r.Group("/api")

	// 消息详情页路由（移到 API 组外）
	r.GET("/m/:id", controllers.GetMessagePage)

	// RSS 路由
	r.GET("/rss", controllers.GenerateRSS)                                               // 保持原有的 RSS 订阅链接
	api.POST("/rss/refresh", middleware.SessionAuthMiddleware(), controllers.RefreshRSS) // 添加刷新 RSS 的路由

	// 公共路由
	api.GET("", controllers.GetStatus)
	api.GET("/frontend/config", controllers.GetFrontendConfig)
	api.GET("/settings", controllers.GetFrontendConfig)
	api.POST("/login", controllers.Login)
	api.POST("/register", controllers.Register)
	api.GET("/status", controllers.GetStatus)
	api.GET("/users/profile", controllers.GetUserProfile)
	api.GET("/captcha", controllers.GetCaptcha)
	// api.GET("/config", controllers.GetFrontendConfig)
	api.GET("/messages", controllers.GetMessages)
	api.GET("/messages/:id", controllers.GetMessage)
	api.POST("/messages/page", controllers.GetMessagesByPage)
	api.GET("/messages/page", controllers.GetMessagesByPage)
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

	// 需要鉴权的路由
	authRoutes := api.Group("")
	authRoutes.Use(middleware.SessionAuthMiddleware())
	// 版本更新（管理员）
	authRoutes.POST("/version/update", controllers.UpdateVersion)
	authRoutes.GET("/version/update/stream", controllers.UpdateVersionStream)
	authRoutes.POST("/version/static-sync", controllers.SyncStatic)
	// 静态资源同步已移除，版本升级统一走容器镜像

	// 添加 token 认证的路由组
	tokenAuth := api.Group("/token")
	tokenAuth.Use(middleware.TokenAuthMiddleware()) // 使用 TokenAuthMiddleware
	{
		tokenAuth.POST("/messages", controllers.PostMessage)
		tokenAuth.PUT("/messages/:id", controllers.UpdateMessage)
		tokenAuth.PUT("/messages/:id/pin", controllers.UpdateMessagePinned)
		tokenAuth.DELETE("/messages/:id", controllers.DeleteMessage)
		tokenAuth.PUT("/settings", controllers.UpdateSetting)
	}
	// 需要鉴权的消息操作路由
	messages := authRoutes.Group("/messages")
	{
		messages.POST("", controllers.PostMessage)
		messages.PUT("/:id", controllers.UpdateMessage)
		messages.PUT("/:id/pin", controllers.UpdateMessagePinned)
		messages.DELETE("/:id", controllers.DeleteMessage)
	}

	// 评论系统（内置）公共路由
	api.GET("/messages/:id/comments", controllers.GetComments)
	api.POST("/messages/comments/counts", controllers.GetCommentCounts)
	api.POST("/messages/:id/comments", controllers.PostComment)
	// 管理员评论列表（提供公共路径，附加会话中间件以注入用户上下文）
	api.GET("/comments", middleware.SessionAuthMiddleware(), controllers.ListComments)
	// 评论删除（管理员）
	authRoutes.DELETE("/messages/:id/comments/:cid", controllers.DeleteComment)
	// 一次性回填评论 parent_id（管理员）
	authRoutes.POST("/comments/backfill", controllers.BackfillCommentParents)
	// 管理员评论列表管理（已在公共组注册路径，函数内部鉴权）
	// 添加推送配置路由
	notify := authRoutes.Group("/notify")
	{
		notify.POST("/test", controllers.TestNotify)        // 测试推送
		notify.POST("/send", controllers.SendNotify)        // 新增：实际推送路由
		notify.GET("/config", controllers.GetNotifyConfig)  // 获取配置
		notify.PUT("/config", controllers.SaveNotifyConfig) // 保存配置
	}

	email := authRoutes.Group("/email")
	{
		email.POST("/test", controllers.EmailTest)
	}

	// 数据库备份相关路由
	backup := authRoutes.Group("/backup")
	{
		backup.GET("/download", controllers.HandleBackupDownload)
		backup.POST("/restore", controllers.HandleBackupRestore)
		backup.POST("/storage/upload", controllers.HandleBackupUploadToURL)
		backup.POST("/storage/restore", controllers.HandleBackupRestoreFromURL)
		backup.POST("/storage/presign/upload", controllers.HandleBackupPresignUpload)
		backup.POST("/storage/presign/download", controllers.HandleBackupPresignDownload)
		backup.POST("/storage/sync-confirm", controllers.HandleBackupSyncConfirm)
		backup.POST("/storage/sync-now", controllers.HandleBackupSyncNow)
	}

	// 系统设置相关路由
	settings := authRoutes.Group("/settings")
	{
		settings.POST("/reset-defaults", controllers.ResetDefaultData)
	}

	// 安全记录（管理员）
	security := authRoutes.Group("/security")
	{
		security.GET("/attacks", middleware.AdminAuthMiddleware(), controllers.GetAttackRecords)
		security.DELETE("/attacks", middleware.AdminAuthMiddleware(), controllers.ClearAttackRecords)
		security.GET("/bans", middleware.AdminAuthMiddleware(), controllers.GetIPBans)
		security.POST("/bans", middleware.AdminAuthMiddleware(), controllers.AddIPBan)
		security.DELETE("/bans", middleware.AdminAuthMiddleware(), controllers.RemoveIPBan)
		security.GET("/config", middleware.AdminAuthMiddleware(), controllers.GetSecurityConfig)
		security.PUT("/config", middleware.AdminAuthMiddleware(), controllers.UpdateSecurityConfig)
	}

	// 图片上传路由
	authRoutes.POST("/images/upload", controllers.UploadImage) // 上传图片
	// 新增：视频上传路由（改为单数 video）
	authRoutes.POST("/video/upload", controllers.UploadVideo) // 上传视频

	// 附件管理路由
	attachments := authRoutes.Group("/attachments")
	{
		attachments.GET("/images", controllers.ListImageAttachments)
		attachments.GET("/images/", controllers.ListImageAttachments)
		attachments.GET("/video", controllers.ListVideoAttachments)
		attachments.GET("/video/", controllers.ListVideoAttachments)
		attachments.DELETE("/images/*name", middleware.AdminAuthMiddleware(), controllers.DeleteImageAttachment)
		attachments.DELETE("/video/*name", middleware.AdminAuthMiddleware(), controllers.DeleteVideoAttachment)
	}

	// 用户相关路由
	user := authRoutes.Group("/user")
	{
		user.GET("", controllers.GetUserInfo)
		user.PUT("/change_password", controllers.ChangePassword)
		user.PUT("/update", controllers.UpdateUser)
		user.PUT("/admin", controllers.UpdateUserAdmin)
		user.DELETE("", controllers.DeleteUser)
		user.POST("/logout", controllers.Logout) // 添加退出登录路由
		user.POST("/reset_password", controllers.AdminResetPassword)
		// 添加 Token 相关路由
		user.GET("/token", controllers.GetUserToken)
		user.POST("/token/regenerate", controllers.RegenerateUserToken)
		user.POST("/email/bind", controllers.BindEmail)
		user.POST("/email/verify", controllers.VerifyEmail)
		user.POST("/email/change/send_code", controllers.SendChangeEmailCode)
		user.POST("/email/change", controllers.ChangeEmail)
	}

	// 设置路由
	authRoutes.PUT("/settings", controllers.UpdateSetting)
	// 友链申请管理（管理员）
	authRoutes.GET("/friend-links/apply", controllers.ListFriendLinkApplications)
	authRoutes.DELETE("/friend-links/apply", controllers.ClearFriendLinkApplications)
	authRoutes.DELETE("/friend-links/apply/:id", controllers.DeleteFriendLinkApplication)
	authRoutes.PUT("/friend-links/:id/audit", controllers.AuditFriendLink)

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

	// 使用静态中间件托管根目录，支持 SPA Fallback
	r.Use(static.Serve("/", static.LocalFile("./public", true)))
	// 显式映射 Nuxt 资源目录和常用静态
	r.Static("/_nuxt", "./public/_nuxt")
	r.Static("/assets", "./public/assets")

	// 静态文件与 Manifest 头部处理
	r.Use(func(c *gin.Context) {
		p := c.Request.URL.Path
		// 先设置 manifest 的 MIME，静态中间件将复用已有头部
		if p == "/manifest.webmanifest" || p == "/manifest.json" {
			c.Header("Content-Type", "application/manifest+json; charset=utf-8")
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		}
		c.Next()
		// 对指纹化静态资源启用长缓存
		if strings.HasPrefix(p, "/_nuxt/") || strings.HasPrefix(p, "/assets/") ||
			strings.HasPrefix(p, "/favicon") || strings.HasPrefix(p, "/android-chrome") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		}
		if p == "/sw.js" {
			// Service Worker 需避免长缓存，确保更新及时生效
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		}
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
