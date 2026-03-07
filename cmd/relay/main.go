package main

import (
	"log"
	"net/http"
	"os"

	"github.com/deskgo/deskgo/internal/relay"
	"github.com/gin-gonic/gin"
)

func main() {
	// 从环境变量读取配置
	host := getEnv("RELAY_HOST", "0.0.0.0")
	port := getEnv("RELAY_PORT", "8080")

	// 创建Gin路由
	router := gin.Default()

	// 加载HTML模板
	router.LoadHTMLGlob("web/*.html")

	// 静态文件服务
	router.Static("/lib", "./web/lib")
	router.StaticFile("/favicon.ico", "./web/favicon.ico")

	// 首页路由
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
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
				"status": "ok",
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
