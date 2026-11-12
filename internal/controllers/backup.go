package controllers

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/config"
	backupservice "github.com/rcy1314/echo-noise/internal/backup"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/syncmanager"
)

// 通过预签名URL上传备份到云存储（R2/S3）
func HandleBackupUploadToURL(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"code": 0, "msg": "请使用已配置的备份通道生成托管备份"})
}

// 通过预签名URL从云存储恢复
func HandleBackupRestoreFromURL(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"code": 0, "msg": "请从备份历史选择或上传备份，检查后明确确认恢复"})
}

func HandleBackupDownload(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"code": 0, "msg": "请分别生成数据、附件或服务器配置备份，再从备份记录下载"})
}

func createBackupArchive(tempDir, dbType string) (string, error) {
	zipFile := filepath.Join(tempDir, "backup.zip")
	if dbType == "sqlite" {
		if err := backupservice.CreateArchive(zipFile, database.DB, backupservice.DefaultLayout()); err != nil {
			return "", fmt.Errorf("创建 SQLite 备份失败: %w", err)
		}
		return zipFile, nil
	}
	if err := backupImages(tempDir); err != nil {
		return "", fmt.Errorf("备份图片失败: %w", err)
	}
	if err := backupVideos(tempDir); err != nil {
		return "", fmt.Errorf("备份视频失败: %w", err)
	}
	switch dbType {
	case "postgres":
		if err := backupPostgres(tempDir); err != nil {
			return "", fmt.Errorf("PostgreSQL备份失败: %w", err)
		}
	case "mysql":
		if err := backupMySQL(tempDir); err != nil {
			return "", fmt.Errorf("MySQL备份失败: %w", err)
		}
	default:
		if err := backupSQLite(tempDir); err != nil {
			return "", fmt.Errorf("SQLite备份失败: %w", err)
		}
	}
	if err := createBackupZip(tempDir, zipFile); err != nil {
		return "", fmt.Errorf("创建备份文件失败: %w", err)
	}
	return zipFile, nil
}

func HandleBackupRestore(c *gin.Context) {
	if _, err := requirePrimaryAdmin(c); err != nil {
		managedBackupError(c, http.StatusForbidden, err)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, managedBackupUploadLimit+(1<<20))
	if c.PostForm("confirm") != "true" {
		managedBackupError(c, http.StatusBadRequest, errors.New("恢复前必须明确确认"))
		return
	}
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	uploaded, err := c.FormFile("database")
	if err != nil {
		managedBackupError(c, http.StatusBadRequest, err)
		return
	}
	reader, err := uploaded.Open()
	if err != nil {
		managedBackupError(c, http.StatusBadRequest, err)
		return
	}
	defer reader.Close()
	path, cleanup, err := managedBackupSpool(c.Request.Context(), reader)
	if err != nil {
		managedBackupError(c, http.StatusBadRequest, err)
		return
	}
	defer cleanup()
	release := syncmanager.LockOperation()
	defer release()
	managedBackupStage(c, path, "original", "data")
}

func HandleBackupPresignUpload(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"code": 0, "msg": "备份通道不再签发任意对象上传链接"})
}

func HandleBackupPresignDownload(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"code": 0, "msg": "请使用具有独立下载权限的备份历史入口"})
}

// 立即执行云端同步（备份到 R2/S3）
func HandleBackupSyncNow(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"code": 0, "msg": "请使用独立备份模块执行云备份；旧双向同步已停用"})
}

func stagedRestoreResponse(message string, result backupservice.StageResult) gin.H {
	response := gin.H{"code": 1, "msg": message, "pendingRestart": true}
	if result.Warning != "" {
		response["warning"] = result.Warning
		response["completeMigration"] = false
	}
	return response
}

// 确认云同步（首次启用/首次启动门禁）
func HandleBackupSyncConfirm(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"code": 0, "msg": "请由站长配置云备份通道；旧同步确认已停用"})
}
func backupPostgres(tempDir string) error {
	dumpFile := filepath.Join(tempDir, "database.sql")

	args := []string{
		"-h", os.Getenv("DB_HOST"),
		"-p", os.Getenv("DB_PORT"),
		"-U", os.Getenv("DB_USER"),
		"-d", os.Getenv("DB_NAME"),
		"-f", dumpFile,
		"--no-owner",
		"--no-privileges",
		"--no-password",
		"--clean",
		"--if-exists",
		"--no-tablespaces",
		"--schema=public",      // 只备份 public schema
		"--no-comments",        // 跳过注释
		"--no-publications",    // 跳过发布
		"--no-subscriptions",   // 跳过订阅
		"--no-security-labels", // 跳过安全标签
	}

	cmd := exec.Command("pg_dump", args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", os.Getenv("DB_PASSWORD")))

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_dump 执行失败: %v, 错误输出: %s", err, stderr.String())
	}

	return nil
}
func backupMySQL(tempDir string) error {
	dumpFile := filepath.Join(tempDir, "database.sql")

	args := []string{
		"-h", os.Getenv("DB_HOST"),
		"-P", os.Getenv("DB_PORT"),
		"-u", os.Getenv("DB_USER"),
		fmt.Sprintf("-p%s", os.Getenv("DB_PASSWORD")),
		"--skip-opt",             // 禁用所有优化选项
		"--skip-comments",        // 跳过注释
		"--skip-triggers",        // 跳过触发器
		"--skip-extended-insert", // 单行插入
		"--compact",              // 最简输出
		"--skip-ssl",
		os.Getenv("DB_NAME"),
	}

	// 先尝试使用 mariadb-dump
	cmd := exec.Command("mariadb-dump", args...)
	outFile, err := os.Create(dumpFile)
	if err != nil {
		return err
	}
	defer outFile.Close()

	cmd.Stdout = outFile
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// 如果失败，尝试最基础的备份方式
		args = []string{
			"-h", os.Getenv("DB_HOST"),
			"-P", os.Getenv("DB_PORT"),
			"-u", os.Getenv("DB_USER"),
			fmt.Sprintf("-p%s", os.Getenv("DB_PASSWORD")),
			"--skip-opt",
			"--compact",
			"--skip-ssl",
			os.Getenv("DB_NAME"),
		}
		cmd = exec.Command("mariadb-dump", args...)
		outFile.Seek(0, 0)
		outFile.Truncate(0)
		cmd.Stdout = outFile
		cmd.Stderr = &stderr
		err = cmd.Run()

		if err != nil {
			// 最后尝试使用 mysqldump
			cmd = exec.Command("mysqldump", args...)
			outFile.Seek(0, 0)
			outFile.Truncate(0)
			cmd.Stdout = outFile
			cmd.Stderr = &stderr
			err = cmd.Run()

			if err != nil {
				return fmt.Errorf("数据库备份失败: %v, 错误输出: %s", err, stderr.String())
			}
		}
	}

	return nil
}
func backupSQLite(tempDir string) error {
	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	cfgPath := strings.TrimSpace(config.Config.Database.Path)
	if cfgPath == "" {
		cfgPath = "data/noise.db"
	}
	dbEnv := strings.TrimSpace(os.Getenv("DB_PATH"))
	candidates := []string{
		dbEnv,
		cfgPath,
		filepath.Join(wd, cfgPath),
		filepath.Join(exeDir, cfgPath),
		"/app/" + cfgPath,
		"/app/data/noise.db",
	}
	var dbPath string
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			dbPath = p
			break
		}
	}
	if dbPath == "" {
		dbPath = filepath.Join(wd, cfgPath)
	}
	return copyFile(dbPath, filepath.Join(tempDir, "database.db"))
}
func restorePostgres(tempDir string) error {
	dumpFile := filepath.Join(tempDir, "database.sql")

	// 先清理现有连接并重建数据库 - 分成单独的命令执行
	terminateCmd := exec.Command("psql",
		"-h", os.Getenv("DB_HOST"),
		"-p", os.Getenv("DB_PORT"),
		"-U", os.Getenv("DB_USER"),
		"-d", "postgres",
		"-c", fmt.Sprintf(`
            SELECT pg_terminate_backend(pg_stat_activity.pid) 
            FROM pg_stat_activity 
            WHERE pg_stat_activity.datname = '%s' 
            AND pg_stat_activity.pid <> pg_backend_pid();
        `, os.Getenv("DB_NAME")),
	)
	terminateCmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", os.Getenv("DB_PASSWORD")))

	var terminateStderr bytes.Buffer
	terminateCmd.Stderr = &terminateStderr

	if err := terminateCmd.Run(); err != nil {
		return fmt.Errorf("终止数据库连接失败: %v, 错误输出: %s", err, terminateStderr.String())
	}

	// 单独执行DROP和CREATE命令
	recreateCmd := exec.Command("psql",
		"-h", os.Getenv("DB_HOST"),
		"-p", os.Getenv("DB_PORT"),
		"-U", os.Getenv("DB_USER"),
		"-d", "postgres",
		"-c", fmt.Sprintf("DROP DATABASE IF EXISTS %s; CREATE DATABASE %s WITH ENCODING='UTF8';",
			os.Getenv("DB_NAME"), os.Getenv("DB_NAME")),
	)
	recreateCmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", os.Getenv("DB_PASSWORD")))

	var recreateStderr bytes.Buffer
	recreateCmd.Stderr = &recreateStderr

	if err := recreateCmd.Run(); err != nil {
		return fmt.Errorf("重建数据库失败: %v, 错误输出: %s", err, recreateStderr.String())
	}

	// 恢复数据
	args := []string{
		"-h", os.Getenv("DB_HOST"),
		"-p", os.Getenv("DB_PORT"),
		"-U", os.Getenv("DB_USER"),
		"-d", os.Getenv("DB_NAME"),
		"-f", dumpFile,
		"--single-transaction",
		"--no-owner",
		"--no-privileges",
	}

	cmd := exec.Command("psql", args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", os.Getenv("DB_PASSWORD")))

	var restoreStderr bytes.Buffer
	cmd.Stderr = &restoreStderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psql 执行失败: %v, 错误输出: %s", err, restoreStderr.String())
	}

	return nil
}
func restoreMySQL(tempDir string) error {
	dumpFile := filepath.Join(tempDir, "database.sql")

	// 先尝试使用 mariadb
	client := "mariadb"
	args := []string{
		"-h", os.Getenv("DB_HOST"),
		"-P", os.Getenv("DB_PORT"),
		"-u", os.Getenv("DB_USER"),
		fmt.Sprintf("-p%s", os.Getenv("DB_PASSWORD")),
		"--skip-ssl",
	}

	// 测试连接
	testCmd := exec.Command(client, append(args, "-e", "SELECT 1")...)
	if err := testCmd.Run(); err != nil {
		// 如果 mariadb 失败，尝试 mysql
		client = "mysql"
		testCmd = exec.Command(client, append(args, "-e", "SELECT 1")...)
		if err := testCmd.Run(); err != nil {
			return fmt.Errorf("数据库连接失败: %v", err)
		}
	}

	// 重置数据库
	resetCmd := exec.Command(client, append(args,
		"-e", fmt.Sprintf("DROP DATABASE IF EXISTS %s; CREATE DATABASE %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;",
			os.Getenv("DB_NAME"), os.Getenv("DB_NAME")))...)
	if err := resetCmd.Run(); err != nil {
		return fmt.Errorf("重置数据库失败: %v", err)
	}

	// 恢复数据
	restoreArgs := append(args, os.Getenv("DB_NAME"))
	cmd := exec.Command(client, restoreArgs...)
	inFile, err := os.Open(dumpFile)
	if err != nil {
		return fmt.Errorf("打开备份文件失败: %v", err)
	}
	defer inFile.Close()

	cmd.Stdin = inFile
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("数据库恢复失败: %v, 错误输出: %s", err, stderr.String())
	}

	return nil
}
func restoreSQLite(tempDir string) error {
	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	cfgPath := strings.TrimSpace(config.Config.Database.Path)
	if cfgPath == "" {
		cfgPath = "data/noise.db"
	}
	dbEnv := strings.TrimSpace(os.Getenv("DB_PATH"))

	candidates := []string{
		dbEnv,
		cfgPath,
		filepath.Join(wd, cfgPath),
		filepath.Join(exeDir, cfgPath),
		"/app/" + cfgPath,
		"/app/data/noise.db",
	}
	var dbPath string
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			dbPath = p
			break
		}
	}
	if dbPath == "" {
		dbPath = filepath.Join(wd, cfgPath)
		_ = os.MkdirAll(filepath.Dir(dbPath), 0755)
	}

	// 支持递归查找，兼容不同压缩结构
	fileCandidates := []string{"database.db", "noise.db", "backup.db", "sqlite.db", "db.sqlite", "database.sqlite"}
	var backupFile string
	// 先尝试顶层
	for _, name := range fileCandidates {
		p := filepath.Join(tempDir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			backupFile = p
			break
		}
	}
	// 若顶层未找到则递归搜索
	if backupFile == "" {
		_ = filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			base := strings.ToLower(filepath.Base(path))
			// 仅匹配可能的 sqlite 文件名或 .db/.sqlite 扩展
			if base == "database.db" || base == "noise.db" || base == "backup.db" ||
				strings.HasSuffix(base, ".db") || strings.HasSuffix(base, ".sqlite") {
				backupFile = path
				return io.EOF // 终止遍历
			}
			return nil
		})
	}
	if backupFile == "" {
		return fmt.Errorf("找不到有效的 SQLite 备份文件")
	}

	backupPath := dbPath + ".bak"
	_ = os.MkdirAll(filepath.Dir(backupPath), 0755)
	if err := copyFile(dbPath, backupPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("创建当前数据库备份失败: %v", err)
		}
	}
	if err := copyFile(backupFile, dbPath); err != nil {
		if _, statErr := os.Stat(backupPath); statErr == nil {
			_ = copyFile(backupPath, dbPath)
			_ = os.Remove(backupPath)
		}
		return fmt.Errorf("恢复数据库失败: %v", err)
	}
	_ = os.Remove(backupPath)
	return nil
}

func backupImages(tempDir string) error {
	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	sp := strings.TrimRight(config.Config.Upload.SavePath, "/")
	if sp == "" {
		sp = "data/images"
	}
	candidates := []string{
		sp,
		"./" + sp,
		filepath.Join(wd, sp),
		filepath.Join(exeDir, sp),
		"./data/images",
		filepath.Join(wd, "data/images"),
		filepath.Join(exeDir, "data/images"),
		"/data/images",
		"/app/data/images",
	}
	var imagesDir string
	for _, d := range candidates {
		if d == "" {
			continue
		}
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			imagesDir = d
			break
		}
	}
	if imagesDir == "" {
		return nil
	}
	destDir := filepath.Join(tempDir, "images")
	return copyDir(imagesDir, destDir)
}

func backupCurrentImages() error {
	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	sp := strings.TrimRight(config.Config.Upload.SavePath, "/")
	if sp == "" {
		sp = "data/images"
	}
	candidates := []string{
		sp,
		"./" + sp,
		filepath.Join(wd, sp),
		filepath.Join(exeDir, sp),
		"./data/images",
		filepath.Join(wd, "data/images"),
		filepath.Join(exeDir, "data/images"),
		"/data/images",
		"/app/data/images",
	}
	var imagesDir string
	for _, d := range candidates {
		if d == "" {
			continue
		}
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			imagesDir = d
			break
		}
	}
	if imagesDir == "" {
		return nil
	}
	backupDir := imagesDir + "_backup"
	return copyDir(imagesDir, backupDir)
}

func restoreImages(tempDir string) error {
	srcDir := filepath.Join(tempDir, "images")
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return nil
	}
	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	sp := strings.TrimRight(config.Config.Upload.SavePath, "/")
	if sp == "" {
		sp = "data/images"
	}
	candidates := []string{
		sp,
		"./" + sp,
		filepath.Join(wd, sp),
		filepath.Join(exeDir, sp),
		"./data/images",
		filepath.Join(wd, "data/images"),
		filepath.Join(exeDir, "data/images"),
		"/data/images",
		"/app/data/images",
	}
	var destDir string
	for _, d := range candidates {
		if d == "" {
			continue
		}
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			destDir = d
			break
		}
	}
	if destDir == "" {
		destDir = filepath.Join(wd, sp)
	}
	_ = os.MkdirAll(destDir, 0755)
	if err := os.RemoveAll(destDir); err != nil {
		return err
	}
	return copyDir(srcDir, destDir)
}

func restoreCurrentImages() error {
	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	sp := strings.TrimRight(config.Config.Upload.SavePath, "/")
	if sp == "" {
		sp = "data/images"
	}
	candidates := []string{
		sp,
		"./" + sp,
		filepath.Join(wd, sp),
		filepath.Join(exeDir, sp),
		"./data/images",
		filepath.Join(wd, "data/images"),
		filepath.Join(exeDir, "data/images"),
		"/data/images",
		"/app/data/images",
	}
	var imagesDir string
	for _, d := range candidates {
		if d == "" {
			continue
		}
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			imagesDir = d
			break
		}
	}
	if imagesDir == "" {
		imagesDir = filepath.Join(wd, sp)
	}
	backupDir := imagesDir + "_backup"
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		return nil
	}
	if err := os.RemoveAll(imagesDir); err != nil {
		return err
	}
	return copyDir(backupDir, imagesDir)
}

func cleanupImageBackup() {
	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	sp := strings.TrimRight(config.Config.Upload.SavePath, "/")
	if sp == "" {
		sp = "data/images"
	}
	candidates := []string{
		sp + "_backup",
		"./" + sp + "_backup",
		filepath.Join(wd, sp+"_backup"),
		filepath.Join(exeDir, sp+"_backup"),
		"./data/images_backup",
		filepath.Join(wd, "data/images_backup"),
		filepath.Join(exeDir, "data/images_backup"),
		"/data/images_backup",
		"/app/data/images_backup",
	}
	for _, d := range candidates {
		_ = os.RemoveAll(d)
	}
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func createBackupZip(sourceDir, zipPath string) (err error) {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(zipFile)
	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == zipPath {
			return nil
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		header.Name = relPath

		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if closeErr := archive.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if closeErr := zipFile.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	return err
}

func unzipBackup(zipPath, destDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		path := filepath.Join(destDir, file.Name)

		// 安全检查：防止 zip slip 漏洞
		if !strings.HasPrefix(path, destDir) {
			return fmt.Errorf("非法的文件路径: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			os.MkdirAll(path, file.Mode())
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return err
		}

		rc, err := file.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// 添加视频备份相关辅助函数
func backupCurrentVideos() error {
	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	candidates := []string{
		"./data/video",
		filepath.Join(wd, "data/video"),
		filepath.Join(exeDir, "data/video"),
		"/data/video",
		"/app/data/video",
	}
	var videoDir string
	for _, d := range candidates {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			videoDir = d
			break
		}
	}
	if videoDir == "" {
		return nil
	}
	backupDir := videoDir + "_backup"
	return copyDir(videoDir, backupDir)
}

func restoreCurrentVideos() error {
	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	candidates := []string{
		"./data/video",
		filepath.Join(wd, "data/video"),
		filepath.Join(exeDir, "data/video"),
		"/data/video",
		"/app/data/video",
	}
	var videoDir string
	for _, d := range candidates {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			videoDir = d
			break
		}
	}
	if videoDir == "" {
		videoDir = filepath.Join(wd, "data/video")
	}
	backupDir := videoDir + "_backup"
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		return nil
	}
	if err := os.RemoveAll(videoDir); err != nil {
		return err
	}
	return copyDir(backupDir, videoDir)
}

func cleanupVideoBackup() {
	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	candidates := []string{
		"./data/video_backup",
		filepath.Join(wd, "data/video_backup"),
		filepath.Join(exeDir, "data/video_backup"),
		"/data/video_backup",
		"/app/data/video_backup",
	}
	for _, d := range candidates {
		_ = os.RemoveAll(d)
	}
}
func backupVideos(tempDir string) error {
	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	candidates := []string{
		"./data/video",
		filepath.Join(wd, "data/video"),
		filepath.Join(exeDir, "data/video"),
		"/data/video",
		"/app/data/video",
	}
	var videoDir string
	for _, d := range candidates {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			videoDir = d
			break
		}
	}
	if videoDir == "" {
		return nil
	}
	destDir := filepath.Join(tempDir, "video")
	return copyDir(videoDir, destDir)
}

func restoreVideos(tempDir string) error {
	srcDir := filepath.Join(tempDir, "video")
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return nil
	}
	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	candidates := []string{
		"./data/video",
		filepath.Join(wd, "data/video"),
		filepath.Join(exeDir, "data/video"),
		"/data/video",
		"/app/data/video",
	}
	var destDir string
	for _, d := range candidates {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			destDir = d
			break
		}
	}
	if destDir == "" {
		destDir = filepath.Join(wd, "data/video")
	}
	_ = os.MkdirAll(destDir, 0755)
	if err := os.RemoveAll(destDir); err != nil {
		return err
	}
	return copyDir(srcDir, destDir)
}
