//go:build desktop
// +build desktop

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/kbinani/screenshot"
)

// DesktopMessage 桌面消息
type DesktopMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id"`

	// 编码格式（新增）
	Codec string `json:"codec"` // "jpeg" 或 "h264"

	// JPEG 字段（保持兼容）
	Data []byte `json:"data,omitempty"`

	// H.264 字段（新增）
	H264Data   []byte `json:"h264_data,omitempty"`
	IsKeyFrame bool   `json:"is_key_frame"`
	SPS        []byte `json:"sps,omitempty"`
	PPS        []byte `json:"pps,omitempty"`

	Width     int   `json:"width"`
	Height    int   `json:"height"`
	Quality   int   `json:"quality"`
	Timestamp int64 `json:"timestamp"`
}

// ControlEvent 控制事件
type ControlEvent struct {
	Type         string `json:"type"`
	EventType    string `json:"event_type"` // mouse, keyboard, canvas_size
	KeyCode      int    `json:"key_code"`
	MouseX       int    `json:"mouse_x"`
	MouseY       int    `json:"mouse_y"`
	MouseMask    int    `json:"mouse_mask"`
	CanvasWidth  int    `json:"canvas_width"`
	CanvasHeight int    `json:"canvas_height"`

	// 编解码器支持（新增）
	H264Supported bool `json:"h264"`
	JPEPSupported bool `json:"jpeg"`
}

type DesktopCapture struct {
	mu             sync.Mutex
	conn           *websocket.Conn
	sessionID      string
	running        bool
	captureStarted bool // 捕获循环是否已启动
	width          int
	height         int
	displayIndex   int
	frameRate      time.Duration
	jpegQuality    int
	frameCount     int
	lastFrameTime  time.Time

	// H.264 编码器
	h264Encoder     H264Encoder
	h264Config      H264Config
	useH264         bool
	h264Initialized bool // H.264 编码器是否已初始化
	forceKeyFrame   bool // 强制发送关键帧
	pendingKeyFrame bool // 编码器初始化完成后是否需要强制关键帧

	// 鼠标状态追踪
	mouseLeftDown   bool
	mouseRightDown  bool
	mouseMiddleDown bool

	// Web端Canvas尺寸（用于坐标映射）
	canvasWidth  int
	canvasHeight int
}

func main() {
	// 加载配置文件
	config, configPath, err := LoadConfig()
	if err != nil {
		log.Fatalf("❌ 加载配置文件失败: %v", err)
	}

	// 命令行参数（优先级高于配置文件）
	serverURL := flag.String("server", "", "Relay WebSocket URL")
	display := flag.Int("display", 0, "显示器索引 (0=主显示器，使用配置文件默认值)")
	fps := flag.Int("fps", 0, "帧率 (0=使用配置文件默认值)")
	quality := flag.Int("quality", 0, "JPEG 质量 (1-100, 0=使用配置文件默认值)")
	sessionID := flag.String("session", "", "会话ID（留空自动生成或使用配置文件）")
	codec := flag.String("codec", "", "编码格式 (jpeg/h264, 留空使用配置文件默认值)")
	h264Bitrate := flag.Int("h264-bitrate", 0, "H.264 比特率 (Kbps, 0=使用配置文件默认值)")
	flag.Parse()

	// 合并配置文件和命令行参数
	config = MergeWithFlags(config, serverURL, display, fps, quality, sessionID, codec, h264Bitrate)

	// 生成会话ID
	sid := config.Session
	if sid == "" {
		sid = uuid.New().String()
	}

	// 显示信息
	printHeader(sid, configPath, config.Server, config.Display, config.FPS, config.Quality)

	// 创建桌面捕获
	capture := &DesktopCapture{
		sessionID:      sid,
		displayIndex:   config.Display,
		frameRate:      time.Second / time.Duration(config.FPS),
		jpegQuality:    config.Quality,
		running:        false, // 初始为false，等待start_capture消息
		captureStarted: false,
		useH264:        config.Codec == "h264",
		h264Config: H264Config{
			Bitrate:     config.H264Bitrate,
			KeyInterval: config.H264KeyInterval,
		},
	}

	// 初始化 H.264 编码器（如果需要）
	if capture.useH264 {
		log.Printf("🎬 初始化 H.264 编码器...")
		capture.h264Encoder = NewH264Encoder()

		if capture.h264Encoder.IsHardwareAccelerated() {
			log.Printf("✅ 使用硬件 H.264 编码器")
		} else {
			log.Printf("ℹ️  使用软件/平台自适应 H.264 编码器")
		}
	}

	// 连接到 Relay
	log.Printf("🔗 正在连接到 Relay...")
	if err := capture.connect(config.Server); err != nil {
		log.Fatalf("❌ 连接失败: %v", err)
	}
	defer capture.conn.Close()

	log.Printf("✅ 已连接到 Relay")
	log.Printf("🌐 Web 访问: %s/session/%s", getWebURL(config.Server), capture.sessionID)
	log.Printf("")

	// 显示二维码提示
	log.Printf("💡 提示: 在浏览器中打开上述地址即可远程控制桌面")
	log.Printf("")
	log.Printf("⏳ 等待Web客户端连接...（捕获循环将在Web客户端连接后自动启动）")
	log.Printf("")

	// 启动控制事件接收（包含start_capture消息处理）
	capture.receiveControlLoop()

	// 等待退出
	select {}
}

func printHeader(sessionID, configPath, serverURL string, display, fps, quality int) {
	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║              🚀 DeskGo 桌面捕获客户端                    ║")
	fmt.Println("╠═══════════════════════════════════════════════════════════╣")
	fmt.Printf("║ 📋 会话ID: %-43s ║\n", sessionID)
	fmt.Printf("║ 📁 配置文件: %-39s ║\n", configPath)
	fmt.Printf("║ 🖥️  显示器: %-43d ║\n", display)
	fmt.Printf("║ 📐 帧率: %-47d ║\n", fps)
	fmt.Printf("║ 🎨 质量: %-47d ║\n", quality)
	fmt.Printf("║ 🌐 Relay: %-43s ║\n", serverURL)
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")
	fmt.Println()
}

// getWebURL 从 WebSocket URL 生成 Web URL
// 例如: wss://deskgo.zty8.cn/api/desktop -> https://deskgo.zty8.cn
func getWebURL(wsURL string) string {
	// 提取主机名部分
	// wss://deskgo.zty8.cn/api/desktop -> deskgo.zty8.cn
	// ws://localhost:8082/api/desktop -> localhost:8082

	if len(wsURL) >= 3 {
		var protocol string
		var hostAndPath string

		if wsURL[:3] == "wss" {
			protocol = "https://"
			hostAndPath = wsURL[6:] // 去掉 wss://
		} else if wsURL[:2] == "ws" {
			protocol = "http://"
			hostAndPath = wsURL[5:] // 去掉 ws://
		} else {
			return wsURL
		}

		// 找到第一个 / 的位置，只保留主机名
		slashIndex := 0
		for i, c := range hostAndPath {
			if c == '/' {
				slashIndex = i
				break
			}
		}

		if slashIndex > 0 {
			return protocol + hostAndPath[:slashIndex]
		}
		return protocol + hostAndPath
	}
	return wsURL
}

func (c *DesktopCapture) connect(serverURL string) error {
	// 解析 URL
	u, err := url.Parse(serverURL)
	if err != nil {
		return fmt.Errorf("解析URL失败: %w", err)
	}

	// 构建正确的路径：/api/desktop/<session_id>
	// 而不是：/api/desktop?session_id=<session_id>
	path := u.Path
	if path == "" || path == "/" || path == "/api/desktop" {
		// 移除尾部斜杠（如果有）
		path = "/api/desktop"
	}

	// 将 session_id 添加到路径中
	u.Path = fmt.Sprintf("%s/%s", path, c.sessionID)

	// 添加查询参数
	// 注意：必须使用 "client" 而不是 "desktop"，因为 Relay 检查的是 "client"
	q := u.Query()
	q.Set("type", "client")
	u.RawQuery = q.Encode()

	log.Printf("🔗 连接到: %s", u.String())

	// 连接 WebSocket（使用 gorilla/websocket）
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("WebSocket 连接失败: %w", err)
	}

	c.conn = conn
	return nil
}

func (c *DesktopCapture) captureLoop() {
	log.Printf("🔄 捕获循环已启动")
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	ticker := time.NewTicker(c.frameRate)
	defer ticker.Stop()

	defer func() {
		if r := recover(); r != nil {
			log.Printf("💥 捕获循环 panic: %v", r)
		}
	}()

	for c.running {
		<-ticker.C

		if err := c.captureAndSend(); err != nil {
			log.Printf("❌ 捕获失败: %v", err)
			continue
		}
	}
}

func (c *DesktopCapture) captureAndSend() error {
	start := time.Now()

	// 捕获屏幕
	img, err := screenshot.CaptureRect(c.getBounds())
	if err != nil {
		return fmt.Errorf("捕获失败: %w", err)
	}
	log.Printf("📸 屏幕捕获成功: %dx%d", img.Bounds().Dx(), img.Bounds().Dy())

	// 更新帧信息
	c.mu.Lock()
	c.width = img.Bounds().Dx()
	c.height = img.Bounds().Dy()
	c.frameCount++
	log.Printf("📊 帧 #%d 准备编码 (尺寸: %dx%d)", c.frameCount, c.width, c.height)

	// 首次初始化 H.264 编码器（需要知道实际分辨率）
	if c.useH264 && c.h264Encoder != nil && !c.h264Initialized {
		c.mu.Unlock()

		// 计算帧率
		fps := int(1.0 / c.frameRate.Seconds())

		// 初始化编码器
		log.Printf("🎬 初始化 H.264 编码器 (%dx%d @ %dfps)...", c.width, c.height, fps)
		err := c.h264Encoder.Initialize(c.width, c.height, fps, c.h264Config.Bitrate)
		if err != nil {
			log.Printf("⚠️  H.264 编码器初始化失败: %v，回退到 JPEG", err)
			c.mu.Lock()
			c.useH264 = false
			c.mu.Unlock()
		} else {
			log.Printf("✅ H.264 编码器初始化成功")
			c.mu.Lock()
			c.h264Initialized = true

			// 检查是否有待处理的关键帧请求
			if c.pendingKeyFrame {
				log.Printf("🎬 检测到待处理的关键帧请求，立即强制关键帧")
				c.forceKeyFrame = true
				c.pendingKeyFrame = false
				log.Printf("🔍 [DEBUG] pendingKeyFrame -> forceKeyFrame")
			}

			c.mu.Unlock()
		}

		c.mu.Lock()
	}

	currentUseH264 := c.useH264 && c.h264Initialized

	// 检查是否需要强制发送关键帧
	var forceKeyFrame bool
	if c.forceKeyFrame && c.h264Initialized {
		forceKeyFrame = true
		c.forceKeyFrame = false
		log.Printf("🎬 强制关键帧模式已激活 (forceKeyFrame=%v)", forceKeyFrame)
	}

	c.mu.Unlock()

	var msg DesktopMessage

	if currentUseH264 {
		// H.264 编码路径
		log.Printf("🔹 调用 H.264 编码器: forceKeyFrame=%v", forceKeyFrame)
		h264Data, _, sps, pps, err := c.h264Encoder.Encode(img, forceKeyFrame)
		if err != nil {
			// 编码失败，回退到 JPEG
			log.Printf("⚠️  H.264 编码失败: %v，回退到 JPEG", err)
			c.mu.Lock()
			c.useH264 = false
			c.h264Initialized = false
			c.mu.Unlock()
		} else {
			// H.264 编码成功
			// 过滤 SEI NALU（类型6），只保留视频 NALU
			filteredH264Data := filterSEINALUs(h264Data)

			// 重新检测过滤后的数据是否是关键帧
			actualIsKeyFrame := detectIDRFrame(filteredH264Data)

			// 浏览器通过独立的 SPS/PPS 字段配置解码器，帧数据保持纯 AVCC 视频负载。
			finalH264Data := filteredH264Data
			if actualIsKeyFrame {
				if len(sps) > 0 && len(pps) > 0 {
					log.Printf("🔍 [关键帧] 使用独立 SPS/PPS 配置: SPS=%d字节, PPS=%d字节, 视频=%d字节",
						len(sps), len(pps), len(finalH264Data))
				} else {
					log.Printf("⚠️  [关键帧] 缺少 SPS/PPS: SPS=%d字节, PPS=%d字节", len(sps), len(pps))
				}
			}

			// 打印最终数据的前32字节用于调试
			if len(finalH264Data) >= 32 {
				log.Printf("🔍 最终数据前32字节:")
				for i := 0; i < 32; i += 16 {
					start := i
					end := i + 16
					if end > len(finalH264Data) {
						end = len(finalH264Data)
					}
					log.Printf("   %04d: %s", start,
						strings.Replace(strings.ToUpper(fmt.Sprintf("% x", finalH264Data[start:end])), " ", " ", -1))
				}
			}

			log.Printf("✅ H.264 编码成功: 原始=%d字节, 最终=%d字节, IDR=%v, 已提供SPS/PPS=%v",
				len(h264Data), len(finalH264Data), actualIsKeyFrame, len(sps) > 0 && len(pps) > 0)

			msg = DesktopMessage{
				Type:       "frame",
				SessionID:  c.sessionID,
				UserID:     "desktop",
				Codec:      "h264",
				H264Data:   finalH264Data, // 发送纯 AVCC 视频数据，SPS/PPS 通过独立字段传输
				IsKeyFrame: actualIsKeyFrame,
				SPS:        sps, // 保留SPS/PPS字段用于配置解码器
				PPS:        pps,
				Width:      c.width,
				Height:     c.height,
				Timestamp:  time.Now().UnixNano(),
			}
		}
	}

	// JPEG 编码路径（默认或 H.264 回退）
	if msg.Codec == "" || msg.Codec == "jpeg" {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: c.jpegQuality}); err != nil {
			return fmt.Errorf("JPEG 编码失败: %w", err)
		}

		msg = DesktopMessage{
			Type:      "frame",
			SessionID: c.sessionID,
			UserID:    "desktop",
			Codec:     "jpeg",
			Data:      buf.Bytes(),
			Width:     c.width,
			Height:    c.height,
			Quality:   c.jpegQuality,
			Timestamp: time.Now().UnixNano(),
		}
	}

	// 序列化 JSON
	jsonData, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("JSON 序列化失败: %w", err)
	}

	// 发送（使用 gorilla/websocket）
	if err := c.conn.WriteMessage(websocket.TextMessage, jsonData); err != nil {
		return fmt.Errorf("发送失败: %w", err)
	}

	// 确认发送成功
	if c.frameCount%10 == 0 {
		codecType := "JPEG"
		if msg.Codec == "h264" {
			codecType = "H.264"
		}
		log.Printf("📤 已发送帧 #%d: %s, %d字节 JSON", c.frameCount, codecType, len(jsonData))
	}

	// 性能日志
	elapsed := time.Since(start)
	fps := 1.0 / elapsed.Seconds()
	sizeKB := 0
	if msg.Codec == "jpeg" {
		sizeKB = len(msg.Data) / 1024
	} else if msg.Codec == "h264" {
		sizeKB = len(msg.H264Data) / 1024
	}

	if c.frameCount%30 == 0 {
		codecInfo := "JPEG"
		if msg.Codec == "h264" {
			codecInfo = "H.264"
		}
		log.Printf("📊 帧 #%d: %dx%d, %s, %d KB, %.2f ms (%.1f fps)",
			c.frameCount, c.width, c.height, codecInfo, sizeKB, elapsed.Milliseconds(), fps)
	}

	return nil
}

func (c *DesktopCapture) receiveControlLoop() {
	for {
		// 接收消息（使用 gorilla/websocket）
		messageType, msg, err := c.conn.ReadMessage()
		if err != nil {
			if err != io.EOF {
				log.Printf("❌ 接收控制事件失败: %v", err)
			}
			return
		}

		// 忽略非文本消息（ping/pong 控制帧在协议层面自动处理）
		if messageType != websocket.TextMessage {
			continue
		}

		// 跳过空消息
		if len(msg) == 0 {
			continue
		}

		// 解析控制事件
		var event ControlEvent
		if err := json.Unmarshal(msg, &event); err != nil {
			continue // 静默忽略无法解析的消息
		}

		// 处理控制事件
		c.handleControlEvent(&event)
	}
}

func (c *DesktopCapture) handleControlEvent(event *ControlEvent) {
	// 根据消息类型处理（使用 Type 字段区分不同消息）
	// Type: "codec_support", "request_keyframe", "input" 等
	// EventType: 仅用于 input 消息的子类型（"mouse", "keyboard", "canvas_size"）

	// 调试：打印收到的所有消息
	log.Printf("🔍 [DEBUG] 收到控制事件: Type=%s, EventType=%s", event.Type, event.EventType)

	switch event.Type {
	case "start_capture":
		// Relay通知开始捕获（当有Web客户端连接时）
		c.mu.Lock()
		if !c.captureStarted {
			log.Printf("🚀 收到start_capture消息，启动捕获循环")
			c.captureStarted = true
			c.running = true
			go c.captureLoop()
		} else {
			log.Printf("ℹ️  捕获循环已经在运行，忽略重复的start_capture消息")
		}
		c.mu.Unlock()

	case "input":
		// 输入事件（鼠标、键盘、Canvas尺寸）
		// 使用 EventType 区分子类型
		switch event.EventType {
		case "mouse":
			c.handleMouseEvent(event)
		case "keyboard":
			c.handleKeyboardEvent(event)
		case "canvas_size":
			// 更新Canvas尺寸
			if event.CanvasWidth > 0 && event.CanvasHeight > 0 {
				c.mu.Lock()
				c.canvasWidth = event.CanvasWidth
				c.canvasHeight = event.CanvasHeight
				c.mu.Unlock()
				log.Printf("📐 Canvas尺寸更新: %dx%d", event.CanvasWidth, event.CanvasHeight)
			}
		default:
			log.Printf("⚠️  未知输入事件类型: %s", event.EventType)
		}

	case "codec_support":
		// 浏览器编解码器支持
		log.Printf("🔍 [DEBUG] 收到 codec_support 消息: H.264=%v, JPEG=%v", event.H264Supported, event.JPEPSupported)
		c.mu.Lock()
		previousUseH264 := c.useH264
		// 只有在配置中指定了 H.264 且浏览器支持时才使用 H.264
		if event.H264Supported && c.useH264 {
			log.Printf("✅ 浏览器支持 H.264，继续使用 H.264 编码")
			log.Printf("🔍 [DEBUG] h264Initialized=%v", c.h264Initialized)

			// 强制发送一个关键帧，确保新连接的浏览器能解码
			if c.h264Initialized {
				log.Printf("🎬 H.264 已初始化，立即强制发送关键帧")
				c.forceKeyFrame = true
				log.Printf("🔍 [DEBUG] forceKeyFrame 已设置为 true")
			} else {
				log.Printf("⏳ H.264 编码器未初始化，将在初始化后强制关键帧")
				c.pendingKeyFrame = true
				log.Printf("🔍 [DEBUG] pendingKeyFrame 已设置为 true")
			}
		} else if c.useH264 && !event.H264Supported {
			log.Printf("⚠️  浏览器不支持 H.264，回退到 JPEG")
			c.useH264 = false
			c.h264Initialized = false
		} else {
			log.Printf("ℹ️  浏览器编解码器支持: H.264=%v, JPEG=%v", event.H264Supported, event.JPEPSupported)
		}
		c.mu.Unlock()

		// 如果编码格式改变，记录日志
		if previousUseH264 != c.useH264 {
			if c.useH264 {
				log.Printf("🎬 切换到 H.264 编码")
			} else {
				log.Printf("🖼️  切换到 JPEG 编码")
			}
		}

	case "request_keyframe":
		// 浏览器请求立即发送关键帧
		log.Printf("🔍 [DEBUG] 收到 request_keyframe 消息")
		c.mu.Lock()
		log.Printf("🔍 [DEBUG] h264Initialized=%v, useH264=%v", c.h264Initialized, c.useH264)
		if c.h264Initialized && c.useH264 {
			log.Printf("🎬 收到浏览器关键帧请求，强制发送关键帧")
			c.forceKeyFrame = true
			log.Printf("🔍 [DEBUG] forceKeyFrame 已设置为 true")
		} else {
			log.Printf("⚠️  无法强制关键帧: h264Initialized=%v, useH264=%v", c.h264Initialized, c.useH264)
		}
		c.mu.Unlock()

	default:
		log.Printf("⚠️  未知消息类型: %s", event.Type)
	}
}

func (c *DesktopCapture) handleMouseEvent(event *ControlEvent) {
	if event.CanvasWidth > 0 && event.CanvasHeight > 0 {
		c.mu.Lock()
		c.canvasWidth = event.CanvasWidth
		c.canvasHeight = event.CanvasHeight
		c.mu.Unlock()
	}

	// 映射坐标：Canvas → 屏幕
	screenX, screenY := c.mapCoordsToScreen(event.MouseX, event.MouseY, event.CanvasWidth, event.CanvasHeight)

	// 移动鼠标
	if err := c.moveMouse(screenX, screenY); err != nil {
		log.Printf("❌ 鼠标移动失败: %v", err)
	}

	// 处理鼠标按钮
	if err := c.handleMouseButton(event.MouseMask, screenX, screenY); err != nil {
		log.Printf("❌ 鼠标点击失败: %v", err)
	}
}

func (c *DesktopCapture) handleKeyboardEvent(event *ControlEvent) {
	jsKeyCode := event.KeyCode
	if jsKeyCode < 0 {
		jsKeyCode = -jsKeyCode
	}

	platformKeyCode := mapJSKeyCodeToPlatformKeyCode(jsKeyCode)
	if platformKeyCode == -1 {
		log.Printf("⚠️  未映射的按键或当前平台输入不可用: JS keyCode=%d", jsKeyCode)
		return
	}

	// 处理按键释放（负数表示释放）
	if event.KeyCode < 0 {
		c.keyToggle(platformKeyCode, false)
		return
	}

	// 按下键
	c.keyTap(platformKeyCode)
}

// mapCoordsToScreen 将Canvas坐标映射到屏幕坐标
func (c *DesktopCapture) mapCoordsToScreen(canvasX, canvasY, canvasW, canvasH int) (int, int) {
	c.mu.Lock()
	screenW := c.width
	screenH := c.height
	if canvasW <= 0 {
		canvasW = c.canvasWidth
	}
	if canvasH <= 0 {
		canvasH = c.canvasHeight
	}
	c.mu.Unlock()

	// 如果没有Canvas尺寸，直接返回原坐标（旧版本兼容）
	if canvasW == 0 || canvasH == 0 {
		return canvasX, canvasY
	}

	// 计算映射比例
	scaleX := float64(screenW) / float64(canvasW)
	scaleY := float64(screenH) / float64(canvasH)

	// 映射坐标
	screenX := int(float64(canvasX) * scaleX)
	screenY := int(float64(canvasY) * scaleY)

	// 边界检查
	if screenX < 0 {
		screenX = 0
	} else if screenX >= screenW {
		screenX = screenW - 1
	}

	if screenY < 0 {
		screenY = 0
	} else if screenY >= screenH {
		screenY = screenH - 1
	}

	return screenX, screenY
}

func (c *DesktopCapture) moveMouse(x, y int) error {
	return platformMouseMove(x, y)
}

// handleMouseButton 处理鼠标按钮
func (c *DesktopCapture) handleMouseButton(mask int, x, y int) error {
	if err := c.syncMouseButton("left", mask&1 != 0, &c.mouseLeftDown, x, y); err != nil {
		return err
	}
	if err := c.syncMouseButton("right", mask&2 != 0, &c.mouseRightDown, x, y); err != nil {
		return err
	}
	if err := c.syncMouseButton("middle", mask&4 != 0, &c.mouseMiddleDown, x, y); err != nil {
		return err
	}
	return nil
}

func (c *DesktopCapture) syncMouseButton(button string, down bool, state *bool, x, y int) error {
	if down == *state {
		return nil
	}
	if err := platformMouseButton(button, down, x, y); err != nil {
		return err
	}
	*state = down
	return nil
}

// keyTap 按键
func (c *DesktopCapture) keyTap(keycode int) error {
	return platformKeyTap(keycode)
}

// keyToggle 按键切换
func (c *DesktopCapture) keyToggle(keycode int, down bool) error {
	return platformKeyToggle(keycode, down)
}

func (c *DesktopCapture) getBounds() image.Rectangle {
	bounds := screenshot.GetDisplayBounds(c.displayIndex)
	if bounds.Dx() == 0 || bounds.Dy() == 0 {
		// 如果失败，使用默认值
		log.Printf("⚠️  无法获取显示器边界，使用默认值 1920x1080")
		return image.Rect(0, 0, 1920, 1080)
	}
	return bounds
}

func (c *DesktopCapture) Stop() {
	c.running = false

	// 关闭 H.264 编码器
	if c.h264Encoder != nil {
		c.h264Encoder.Close()
	}

	if c.conn != nil {
		c.conn.Close()
	}
}

// filterSEINALUs 过滤掉 SEI NALU（类型6）和 AUD（类型9），只保留视频数据
func filterSEINALUs(data []byte) []byte {
	if len(data) < 4 {
		return data
	}

	var filteredNALUs [][]byte
	pos := 0

	for pos < len(data) {
		if pos+4 > len(data) {
			break
		}

		// 读取 NALU 长度（大端序）
		naluLength := (uint32(data[pos]) << 24) | (uint32(data[pos+1]) << 16) |
			(uint32(data[pos+2]) << 8) | uint32(data[pos+3])
		pos += 4

		if naluLength == 0 || pos+int(naluLength) > len(data) {
			break
		}

		// 检查 NALU 类型
		naluType := data[pos] & 0x1F

		// 跳过 SEI（6）和 AUD（9）
		if naluType == 6 || naluType == 9 {
			pos += int(naluLength)
			continue
		}

		// 保留这个 NALU（包括长度前缀）
		filteredNALUs = append(filteredNALUs, data[pos-4:pos+int(naluLength)])
		pos += int(naluLength)
	}

	if len(filteredNALUs) == 0 {
		return data
	}

	// 合并所有 NALU
	totalLength := 0
	for _, nalu := range filteredNALUs {
		totalLength += len(nalu)
	}

	result := make([]byte, totalLength)
	offset := 0
	for _, nalu := range filteredNALUs {
		copy(result[offset:], nalu)
		offset += len(nalu)
	}

	return result
}

// detectIDRFrame 检测过滤后的数据中是否包含 IDR 帧（类型5）
func detectIDRFrame(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	pos := 0
	for pos < len(data) {
		if pos+4 > len(data) {
			break
		}

		// 读取 NALU 长度
		naluLength := (uint32(data[pos]) << 24) | (uint32(data[pos+1]) << 16) |
			(uint32(data[pos+2]) << 8) | uint32(data[pos+3])
		pos += 4

		if naluLength == 0 || pos+int(naluLength) > len(data) {
			break
		}

		// 检查 NALU 类型
		naluType := data[pos] & 0x1F

		// 找到第一个视频 NALU（1-5）
		if naluType >= 1 && naluType <= 5 {
			return naluType == 5 // 是 IDR（5）吗？
		}

		pos += int(naluLength)
	}

	return false
}
