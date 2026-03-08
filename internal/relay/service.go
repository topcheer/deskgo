package relay

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// DesktopMessage 远程桌面消息结构
type DesktopMessage struct {
	Type      string `json:"type"`       // "init", "frame", "input", "resize", "close"
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id"`

	// 编码格式（新增）
	Codec     string `json:"codec"`       // "jpeg" 或 "h264"

	// JPEG 字段（保持兼容）
	Data      []byte `json:"data"`       // 图像数据或输入数据

	// H.264 字段（新增）
	H264Data  []byte `json:"h264_data,omitempty"`  // H.264 编码数据
	IsKeyFrame bool  `json:"is_key_frame"`         // 是否关键帧
	SPS       []byte `json:"sps,omitempty"`        // Sequence Parameter Set
	PPS       []byte `json:"pps,omitempty"`        // Picture Parameter Set

	Width     int    `json:"width"`      // 屏幕宽度
	Height    int    `json:"height"`     // 屏幕高度
	Quality   int    `json:"quality"`    // JPEG 质量

	// 控制事件字段
	EventType string `json:"event_type"` // "key", "mouse", "canvas_size"
	KeyCode   int    `json:"key_code"`   // 键码
	MouseX    int    `json:"mouse_x"`    // 鼠标X坐标
	MouseY    int    `json:"mouse_y"`    // 鼠标Y坐标
	MouseMask int    `json:"mouse_mask"` // 鼠标按钮掩码

	// Canvas 尺寸（用于坐标映射）
	CanvasWidth  int `json:"canvas_width,omitempty"`
	CanvasHeight int `json:"canvas_height,omitempty"`

	Timestamp float64 `json:"timestamp"` // 时间戳（支持浮点数）
}

// Session 会话结构
type Session struct {
	ID        string
	CreatedAt time.Time
	LastPing  time.Time

	// Web客户端连接（可以有多个）
	Clients map[string]*websocket.Conn

	// CLI客户端连接（最多一个）
	ClientConn *websocket.Conn

	mu sync.RWMutex
}

// Service 中继服务
type Service struct {
	sessions map[string]*Session
	mu       sync.RWMutex

	// 心跳配置
	pingInterval  time.Duration
	pingTimeout   time.Duration
}

// 全局服务实例
var globalService *Service
var serviceOnce sync.Once

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源
	},
}

// NewService 创建中继服务
func NewService() *Service {
	return &Service{
		sessions:     make(map[string]*Session),
		pingInterval: 30 * time.Second,
		pingTimeout:  90 * time.Second,
	}
}

// HandleDesktopConnection 处理WebSocket连接
func HandleDesktopConnection(c *gin.Context) {
	sessionID := c.Param("session_id")
	userID := c.Query("user_id")
	connectionType := c.Query("type") // "client" 或 "web"

	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}

	if userID == "" {
		userID = uuid.New().String()
	}

	// 升级HTTP连接到WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	// 获取或创建会话
	service := getServiceOrCreate(sessionID)
	service.mu.Lock()

	session, exists := service.sessions[sessionID]
	if !exists {
		session = &Session{
			ID:        sessionID,
			CreatedAt: time.Now(),
			LastPing:  time.Now(),
			Clients:   make(map[string]*websocket.Conn),
		}
		service.sessions[sessionID] = session
		log.Printf("✅ 新会话创建: %s", sessionID)
	}

	service.mu.Unlock()

	// 根据连接类型处理
	if connectionType == "client" {
		session.mu.Lock()
		session.ClientConn = conn
		session.LastPing = time.Now()
		session.mu.Unlock()

		log.Printf("✅ CLI客户端已连接: %s (用户: %s)", sessionID, userID)

		// 处理客户端消息
		handleClientMessages(conn, session, service)
	} else {
		session.mu.Lock()
		session.Clients[userID] = conn
		session.LastPing = time.Now()

		// 检查是否有CLI客户端连接，如果有则通知开始捕获
		if session.ClientConn != nil {
			log.Printf("📢 通知CLI客户端开始捕获 (Web用户: %s)", userID)

			// 发送 start_capture 消息给CLI客户端
			startMsg := DesktopMessage{
				Type:      "start_capture",
				SessionID: sessionID,
				UserID:    userID,
			}
			msgBytes, _ := json.Marshal(startMsg)
			session.ClientConn.WriteMessage(websocket.TextMessage, msgBytes)
		}

		session.mu.Unlock()

		log.Printf("✅ Web客户端已连接: %s (用户: %s)", sessionID, userID)

		// 处理Web客户端消息
		handleWebMessages(conn, session, userID, service)
	}
}

func handleClientMessages(conn *websocket.Conn, session *Session, service *Service) {
	defer func() {
		session.mu.Lock()
		session.ClientConn = nil
		session.mu.Unlock()
		conn.Close()
		log.Printf("❌ CLI客户端断开: %s", session.ID)
	}()

	// 设置心跳
	conn.SetPingHandler(func(appData string) error {
		session.mu.Lock()
		session.LastPing = time.Now()
		session.mu.Unlock()
		return conn.WriteMessage(websocket.PongMessage, []byte(appData))
	})

	// 接收客户端消息
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("客户端读取失败: %v", err)
			return
		}

		var msg DesktopMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("消息解析失败: %v", err)
			continue
		}

		// 记录收到的消息类型
		if msg.Type == "frame" {
			// 计算实际数据大小（JPEG 或 H.264）
			dataSize := 0
			if msg.Codec == "h264" && len(msg.H264Data) > 0 {
				dataSize = len(msg.H264Data)
			} else if len(msg.Data) > 0 {
				dataSize = len(msg.Data)
			}
			log.Printf("📨 收到帧消息: %dx%d, 编码=%s, 大小=%d字节",
				msg.Width, msg.Height, msg.Codec, dataSize)
		} else if msg.Type == "init" {
			log.Printf("🎬 收到初始化消息")
		}

		// 转发消息到所有Web客户端
		session.mu.RLock()
		clientCount := len(session.Clients)
		if msg.Type == "frame" && clientCount > 0 {
			log.Printf("🔍 准备转发帧: codec=%s, h264_data_len=%d, data_len=%d, json_size=%d",
				msg.Codec, len(msg.H264Data), len(msg.Data), len(message))
		}
		for userID, clientConn := range session.Clients {
			if err := clientConn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("转发到Web客户端失败 (%s): %v", userID, err)
				delete(session.Clients, userID)
				clientConn.Close()
			}
		}
		session.mu.RUnlock()

		if msg.Type == "frame" && clientCount > 0 {
			log.Printf("✅ 转发帧到 %d 个Web客户端", clientCount)
		}
	}
}

func handleWebMessages(conn *websocket.Conn, session *Session, userID string, service *Service) {
	defer func() {
		session.mu.Lock()
		delete(session.Clients, userID)
		session.mu.Unlock()
		conn.Close()
		log.Printf("❌ Web客户端断开: %s (用户: %s)", session.ID, userID)
	}()

	// 设置心跳
	conn.SetPingHandler(func(appData string) error {
		session.mu.Lock()
		session.LastPing = time.Now()
		session.mu.Unlock()
		return conn.WriteMessage(websocket.PongMessage, []byte(appData))
	})

	// 接收Web客户端消息
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Web客户端读取失败: %v", err)
			return
		}

		var msg DesktopMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("消息解析失败: %v", err)
			continue
		}

		// 转发消息到CLI客户端
		session.mu.RLock()
		clientConn := session.ClientConn
		session.mu.RUnlock()

		if clientConn != nil {
			if err := clientConn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("转发到CLI客户端失败: %v", err)
			}
		}
	}
}

// getServiceOrCreate 获取或创建服务（使用全局单例）
func getServiceOrCreate(sessionID string) *Service {
	serviceOnce.Do(func() {
		globalService = NewService()
		log.Println("🚀 全局中继服务已创建")
	})
	return globalService
}

// StartCleanupRoutine 启动清理超时会话的协程
func (s *Service) StartCleanupRoutine() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		for sessionID, session := range s.sessions {
			session.mu.RLock()
			isActive := time.Since(session.LastPing) < s.pingTimeout
			hasClients := len(session.Clients) > 0 || session.ClientConn != nil
			session.mu.RUnlock()

			if !isActive && !hasClients {
				log.Printf("🧹 清理超时会话: %s", sessionID)
				delete(s.sessions, sessionID)
			}
		}
		s.mu.Unlock()
	}
}
