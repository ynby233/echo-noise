package routers

import (
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/controllers"
)

// Managed backup handlers check the current actor and kind-specific capability
// before parsing a body, opening an archive, or accessing configured storage.
func registerManagedBackupRoutes(authRoutes *gin.RouterGroup) {
	backups := authRoutes.Group("/admin/backups")
	backups.GET("/status", controllers.HandleManagedBackupStatus)
	backups.PUT("/plans/:kind", controllers.HandleManagedBackupPlan)
	backups.POST("/:kind/create", controllers.HandleManagedBackupCreate)
	backups.GET("/:kind/history", controllers.HandleManagedBackupHistory)
	backups.GET("/:kind/history/:id/download", controllers.HandleManagedBackupDownload)
	backups.POST("/:kind/history/:id/restore", controllers.HandleManagedBackupRestoreRecord)
	backups.POST("/inspect", controllers.HandleManagedBackupInspect)
	backups.POST("/restore", controllers.HandleManagedBackupRestoreUpload)
}
