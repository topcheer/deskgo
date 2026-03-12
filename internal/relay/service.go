package relay

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// DesktopMessage 远程桌面消息结构
type DesktopMessage struct {
	Type      string `json:"type"` // "init", "frame", "input", "resize", "close"
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id"`
	Message   string `json:"message,omitempty"`

	// 编码格式（新增）
	Codec string `json:"codec"` // "jpeg" 或 "h264"

	// JPEG 字段（保持兼容）
	Data []byte `json:"data"` // 图像数据或输入数据

	// H.264 字段（新增）
	H264Data   []byte `json:"h264_data,omitempty"` // H.264 编码数据
	IsKeyFrame bool   `json:"is_key_frame"`        // 是否关键帧
	SPS        []byte `json:"sps,omitempty"`       // Sequence Parameter Set
	PPS        []byte `json:"pps,omitempty"`       // Picture Parameter Set

	Width   int `json:"width"`   // 屏幕宽度
	Height  int `json:"height"`  // 屏幕高度
	Quality int `json:"quality"` // JPEG 质量

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
	Clients map[string]*managedConn

	// CLI客户端连接（最多一个）
	ClientConn *managedConn

	mu sync.RWMutex
}

// Service 中继服务
type Service struct {
	sessions map[string]*Session
	mu       sync.RWMutex

	// 心跳配置
	pingInterval time.Duration
	pingTimeout  time.Duration
}

// 全局服务实例
var globalService *Service
var serviceOnce sync.Once
var errCLIUnavailable = errors.New("cli client unavailable")

type managedConn struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func newManagedConn(conn *websocket.Conn) *managedConn {
	return &managedConn{conn: conn}
}

func (c *managedConn) ReadMessage() (int, []byte, error) {
	return c.conn.ReadMessage()
}

func (c *managedConn) SetPingHandler(h func(string) error) {
	c.conn.SetPingHandler(h)
}

func (c *managedConn) WriteMessage(messageType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(messageType, data)
}

func (c *managedConn) WriteControl(messageType int, data []byte, deadline time.Time) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteControl(messageType, data, deadline)
}

func (c *managedConn) Close() error {
	return c.conn.Close()
}

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
	sessionID := normalizeSessionID(c.Param("session_id"))
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
	rawConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	conn := newManagedConn(rawConn)

	// 获取或创建会话
	service := getServiceOrCreate(sessionID)
	service.mu.Lock()

	session, exists := service.sessions[sessionID]
	if !exists {
		session = &Session{
			ID:        sessionID,
			CreatedAt: time.Now(),
			LastPing:  time.Now(),
			Clients:   make(map[string]*managedConn),
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
		waitingWebClientCount := len(session.Clients)
		session.mu.Unlock()

		log.Printf("✅ CLI客户端已连接: %s (用户: %s)", sessionID, userID)
		if waitingWebClientCount > 0 {
			log.Printf("📢 检测到 %d 个等待中的 Web 客户端，通知 CLI 开始捕获", waitingWebClientCount)
			if err := sendStartCapture(session, sessionID, "relay"); err != nil {
				log.Printf("通知 CLI 开始捕获失败: %v", err)
				conn.Close()
				return
			}
		}

		// 处理客户端消息
		handleClientMessages(conn, session, service)
	} else {
		session.mu.Lock()
		session.Clients[userID] = conn
		session.LastPing = time.Now()
		hasCLIClient := session.ClientConn != nil
		session.mu.Unlock()

		if hasCLIClient {
			log.Printf("📢 通知CLI客户端开始捕获 (Web用户: %s)", userID)
			if err := sendStartCapture(session, sessionID, userID); err != nil {
				log.Printf("通知CLI客户端开始捕获失败: %v", err)
				removeWebClient(session, userID, conn)
				notifyAndCloseWebClient(conn, sessionID, "桌面客户端当前不可用，请重新启动 CLI 后再连接。")
				return
			}
		}

		log.Printf("✅ Web客户端已连接: %s (用户: %s)", sessionID, userID)

		// 处理Web客户端消息
		handleWebMessages(conn, session, userID, service)
	}
}

func handleClientMessages(conn *managedConn, session *Session, service *Service) {
	defer func() {
		session.mu.Lock()
		if session.ClientConn == conn {
			session.ClientConn = nil
		}
		session.mu.Unlock()

		closeWebClientsWithNotice(session, DesktopMessage{
			Type:      "session_ended",
			SessionID: session.ID,
			UserID:    "relay",
			Message:   "桌面客户端已断开，会话已结束。请重新启动 CLI 后重新连接。",
			Timestamp: float64(time.Now().UnixMilli()),
		})

		conn.Close()
		log.Printf("❌ CLI客户端断开: %s", session.ID)
	}()

	// 设置心跳
	conn.SetPingHandler(func(appData string) error {
		session.mu.Lock()
		session.LastPing = time.Now()
		session.mu.Unlock()
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(time.Second))
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

		clientCount := len(snapshotWebClients(session))
		if msg.Type == "frame" && clientCount > 0 {
			log.Printf("🔍 准备转发帧: codec=%s, h264_data_len=%d, data_len=%d, json_size=%d",
				msg.Codec, len(msg.H264Data), len(msg.Data), len(message))
		}
		broadcastToWebClients(session, message)

		if msg.Type == "frame" && clientCount > 0 {
			log.Printf("✅ 转发帧到 %d 个Web客户端", clientCount)
		}
	}
}

func snapshotWebClients(session *Session) map[string]*managedConn {
	session.mu.RLock()
	defer session.mu.RUnlock()

	clients := make(map[string]*managedConn, len(session.Clients))
	for userID, clientConn := range session.Clients {
		clients[userID] = clientConn
	}
	return clients
}

func removeWebClient(session *Session, userID string, clientConn *managedConn) {
	session.mu.Lock()
	defer session.mu.Unlock()

	if currentConn, ok := session.Clients[userID]; ok && currentConn == clientConn {
		delete(session.Clients, userID)
	}
}

func broadcastToWebClients(session *Session, message []byte) {
	for userID, clientConn := range snapshotWebClients(session) {
		if err := clientConn.WriteMessage(websocket.TextMessage, message); err != nil {
			log.Printf("转发到Web客户端失败 (%s): %v", userID, err)
			removeWebClient(session, userID, clientConn)
			clientConn.Close()
			stopCaptureIfIdle(session)
		}
	}
}

func sendStartCapture(session *Session, sessionID, userID string) error {
	msgBytes, err := json.Marshal(DesktopMessage{
		Type:      "start_capture",
		SessionID: sessionID,
		UserID:    userID,
	})
	if err != nil {
		return err
	}
	return writeToCLI(session, msgBytes)
}

func sendStopCapture(session *Session, sessionID, userID string) error {
	msgBytes, err := json.Marshal(DesktopMessage{
		Type:      "stop_capture",
		SessionID: sessionID,
		UserID:    userID,
	})
	if err != nil {
		return err
	}
	return writeToCLI(session, msgBytes)
}

func writeToCLI(session *Session, message []byte) error {
	session.mu.RLock()
	clientConn := session.ClientConn
	session.mu.RUnlock()

	if clientConn == nil {
		return errCLIUnavailable
	}

	if err := clientConn.WriteMessage(websocket.TextMessage, message); err != nil {
		session.mu.Lock()
		if session.ClientConn == clientConn {
			session.ClientConn = nil
		}
		session.mu.Unlock()
		clientConn.Close()
		return err
	}

	return nil
}

func stopCaptureIfIdle(session *Session) {
	session.mu.RLock()
	clientCount := len(session.Clients)
	hasCLI := session.ClientConn != nil
	sessionID := session.ID
	session.mu.RUnlock()

	if clientCount > 0 || !hasCLI {
		return
	}

	log.Printf("⏸️ 所有Web客户端已断开，通知CLI暂停捕获: %s", sessionID)
	if err := sendStopCapture(session, sessionID, "relay"); err != nil && !errors.Is(err, errCLIUnavailable) {
		log.Printf("通知CLI暂停捕获失败: %v", err)
	}
}

func closeWebClientsWithNotice(session *Session, notice DesktopMessage) {
	noticePayload, err := json.Marshal(notice)
	if err != nil {
		log.Printf("序列化会话结束通知失败: %v", err)
		return
	}

	closePayload := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "desktop client disconnected")
	for userID, clientConn := range snapshotWebClients(session) {
		if err := clientConn.WriteMessage(websocket.TextMessage, noticePayload); err != nil {
			log.Printf("发送会话结束通知失败 (%s): %v", userID, err)
		}
		if err := clientConn.WriteControl(websocket.CloseMessage, closePayload, time.Now().Add(time.Second)); err != nil {
			log.Printf("发送关闭帧失败 (%s): %v", userID, err)
		}
		removeWebClient(session, userID, clientConn)
		clientConn.Close()
	}
}

func handleWebMessages(conn *managedConn, session *Session, userID string, service *Service) {
	defer func() {
		removeWebClient(session, userID, conn)
		stopCaptureIfIdle(session)
		conn.Close()
		log.Printf("❌ Web客户端断开: %s (用户: %s)", session.ID, userID)
	}()

	// 设置心跳
	conn.SetPingHandler(func(appData string) error {
		session.mu.Lock()
		session.LastPing = time.Now()
		session.mu.Unlock()
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(time.Second))
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

		if msg.Type == "ping" {
			pongPayload, err := json.Marshal(DesktopMessage{
				Type:      "pong",
				SessionID: session.ID,
				UserID:    "relay",
				Timestamp: msg.Timestamp,
			})
			if err != nil {
				log.Printf("序列化 pong 消息失败: %v", err)
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, pongPayload); err != nil {
				log.Printf("发送 pong 消息失败: %v", err)
				return
			}
			continue
		}

		if err := writeToCLI(session, message); err != nil {
			if errors.Is(err, errCLIUnavailable) {
				continue
			}
			log.Printf("转发到CLI客户端失败: %v", err)
			notifyAndCloseWebClient(conn, session.ID, "桌面客户端当前不可用，请重新启动 CLI 后再连接。")
			return
		}
	}
}

func notifyAndCloseWebClient(conn *managedConn, sessionID, message string) {
	noticePayload, err := json.Marshal(DesktopMessage{
		Type:      "session_ended",
		SessionID: sessionID,
		UserID:    "relay",
		Message:   message,
		Timestamp: float64(time.Now().UnixMilli()),
	})
	if err == nil {
		if writeErr := conn.WriteMessage(websocket.TextMessage, noticePayload); writeErr != nil {
			log.Printf("发送单客户端断开通知失败: %v", writeErr)
		}
	}
	if err := conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "desktop client unavailable"), time.Now().Add(time.Second)); err != nil {
		log.Printf("发送单客户端关闭帧失败: %v", err)
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

func normalizeSessionID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
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
