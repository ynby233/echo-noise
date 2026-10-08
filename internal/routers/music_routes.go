package routers

import (
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	backupservice "github.com/rcy1314/echo-noise/internal/backup"
	"github.com/rcy1314/echo-noise/internal/controllers"
	"github.com/rcy1314/echo-noise/internal/middleware"
)

func musicCacheExcludedRoots() []string {
	roots := make([]string, 0)
	for _, root := range backupservice.DefaultLayout().Roots {
		roots = append(roots, root.Path)
	}
	return roots
}

func registerMusicManagementRoutes(group *gin.RouterGroup, controller *controllers.MusicController) {
	view := middleware.RequireCapability(authorization.CapabilityMusicView)
	manage := middleware.RequireCapability(authorization.CapabilityMusicManage)
	group.GET("/music/config", view, controller.GetConfig)
	group.PUT("/music/config", manage, controller.SaveConfig)
	group.GET("/music/library", view, controller.ListLibrary)
	group.POST("/music/library/refresh", manage, controller.Refresh)
	group.GET("/music/library/refresh-status", view, controller.ScanStatus)
	group.GET("/music/playlist", view, controller.GetPlaylist)
	group.PUT("/music/playlist", manage, controller.SavePlaylist)
	group.GET("/music/library/:trackID/cover", view, controller.AdminCover)
	group.HEAD("/music/library/:trackID/cover", view, controller.AdminCover)
}

func registerPublicMusicRoutes(group *gin.RouterGroup, controller *controllers.MusicController) {
	group.GET("/music/public", controller.Public)
	group.GET("/music/stream/:trackID", controller.Stream)
	group.HEAD("/music/stream/:trackID", controller.Stream)
	group.GET("/music/cover/:trackID", controller.Cover)
	group.HEAD("/music/cover/:trackID", controller.Cover)
	group.GET("/music/lyrics/:trackID", controller.Lyrics)
}
