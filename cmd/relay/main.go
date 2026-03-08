package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/deskgo/deskgo/internal/relay"
	"github.com/gin-gonic/gin"
)

func main() {
	// 从环境变量读取配置
	host := getEnv("RELAY_HOST", "0.0.0.0")
	port := getEnv("RELAY_PORT", "8080")

	// 获取项目目录
	projectDir := getProjectDir()
	webDir := filepath.Join(projectDir, "web")
	downloadsDir := filepath.Join(projectDir, "downloads")

	log.Printf("📂 项目目录: %s", projectDir)
	log.Printf("🌐 Web 目录: %s", webDir)

	// 创建Gin路由
	router := gin.Default()

	// 加载HTML模板
	templatePath := filepath.Join(webDir, "*.html")
	router.LoadHTMLGlob(templatePath)

	// 静态文件服务
	router.Static("/lib", filepath.Join(webDir, "lib"))
	router.StaticFile("/favicon.ico", filepath.Join(webDir, "favicon.ico"))
	if _, err := os.Stat(downloadsDir); err == nil {
		router.Static("/downloads", downloadsDir)
		log.Printf("📦 Downloads 目录: %s", downloadsDir)
	}

	// 首页路由
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	// 调试页面路由
	router.GET("/debug", func(c *gin.Context) {
		c.HTML(http.StatusOK, "debug.html", nil)
	})

	// 诊断页面路由
	router.GET("/diagnose", func(c *gin.Context) {
		c.HTML(http.StatusOK, "diagnose.html", nil)
	})

	// 会话页面路由
	router.GET("/session/:session_id", func(c *gin.Context) {
		sessionID := c.Param("session_id")
		c.HTML(http.StatusOK, "desktop.html", gin.H{
			"sessionID": sessionID,
		})
	})

	// API路由
	api := router.Group("/api")
	{
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status":  "ok",
				"service": "deskgo-relay",
			})
		})

		// WebSocket连接端点
		api.GET("/desktop/:session_id", relay.HandleDesktopConnection)
	}

	// 启动服务
	addr := host + ":" + port
	log.Printf("Starting DeskGo relay server on %s", addr)

	if err := router.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getProjectDir 获取项目目录
func getProjectDir() string {
	// 优先使用环境变量
	if dir := os.Getenv("DESKGO_PROJECT_DIR"); dir != "" {
		return dir
	}

	// 获取可执行文件路径
	execPath, err := os.Executable()
	if err != nil {
		// 如果失败，使用当前目录
		return "."
	}

	// 获取可执行文件的目录
	execDir := filepath.Dir(execPath)

	// 如果在 bin/ 目录下，返回上一级
	if filepath.Base(execDir) == "bin" {
		return filepath.Dir(execDir)
	}

	// 否则检查当前目录是否有 web 目录
	if _, err := os.Stat("web"); err == nil {
		return "."
	}

	// 检查父目录
	parentDir := filepath.Dir(execDir)
	if _, err := os.Stat(filepath.Join(parentDir, "web")); err == nil {
		return parentDir
	}

	// 默认返回当前目录
	return "."
}
