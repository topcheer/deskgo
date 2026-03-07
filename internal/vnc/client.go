package vnc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"net"
	"sync"
	"time"
)

// DesktopMessage 桌面消息结构
type DesktopMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id"`
	Data      []byte `json:"data"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
}

// Client VNC客户端
type Client struct {
	conn        net.Conn
	password    string
	width       int
	height      int
	frameBuffer *image.RGBA
	mu          sync.Mutex
	lastFrame   time.Time
	frameRate   time.Duration
	connected   bool
}

// Connect 连接到VNC服务器
func Connect(host, password string) (*Client, error) {
	log.Printf("🔌 正在连接VNC服务器: %s", host)

	// TODO: 实现真正的VNC连接
	// 暂时使用模拟模式
	log.Printf("⚠️  使用模拟模式（未实现真正的VNC协议）")

	client := &Client{
		password:  password,
		width:    1024,
		height:   768,
		frameRate: 33 * time.Millisecond, // ~30 FPS
		connected: true,
	}

	client.frameBuffer = image.NewRGBA(image.Rect(0, 0, client.width, client.height))

	// 填充一个简单的测试画面
	fillTestPattern(client.frameBuffer)

	log.Printf("✅ VNC连接成功(模拟): %dx%d", client.width, client.height)

	return client, nil
}

// fillTestPattern 填充测试图案
func fillTestPattern(buf *image.RGBA) {
	bounds := buf.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			// 创建渐变背景
			r := uint8((x * 255) / bounds.Max.X)
			g := uint8((y * 255) / bounds.Max.Y)
			b := uint8(128)

			// 添加一些文本区域
			if x > 100 && x < 500 && y > 100 && y < 200 {
				r = 70
				g = 130
				b = 180
			}

			buf.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
}

// Close 关闭连接
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = false
	log.Printf("👋 VNC连接已关闭")
	return nil
}

// StartFrameCapture 启动帧捕获
func (c *Client) StartFrameCapture(writeChan chan<- []byte, sessionID string) {
	log.Printf("📺 开始捕获VNC帧...")

	// 持续发送帧
	go func() {
		frameCount := 0
		for c.connected {
			c.mu.Lock()

			// 更新测试图案（模拟屏幕变化）
			updateTestPattern(c.frameBuffer, frameCount)
			frameCount++

			// 发送帧
			if err := c.sendFrame(writeChan, sessionID); err != nil {
				log.Printf("❌ 发送帧失败: %v", err)
			}
			c.mu.Unlock()

			// 帧率控制
			time.Sleep(c.frameRate)
		}
	}()
}

// updateTestPattern 更新测试图案
func updateTestPattern(buf *image.RGBA, frame int) {
	bounds := buf.Bounds()
	t := float64(frame) / 100.0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			// 动态渐变背景
			r := uint8(((x + int(t*50)) * 255) / bounds.Max.X)
			g := uint8(((y + int(t*30)) * 255) / bounds.Max.Y)
			b := uint8((128 + int(t*20)) % 256)

			// 移动的文本区域
			textX := (100 + int(t*100)) % (bounds.Max.X - 500)
			if x > textX && x < textX+400 && y > 100 && y < 200 {
				r = 70
				g = 130
				b = 180
			}

			// 添加当前帧号显示
			if x > 10 && x < 200 && y > 10 && y < 40 {
				r = 50
				g = 50
				b = 50
			}

			buf.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
}

// sendFrame 发送帧数据
func (c *Client) sendFrame(writeChan chan<- []byte, sessionID string) error {
	// 编码为PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, c.frameBuffer); err != nil {
		return fmt.Errorf("PNG编码失败: %w", err)
	}

	// 创建消息
	msg := DesktopMessage{
		Type:      "frame",
		SessionID: sessionID,
		UserID:    "client",
		Data:      buf.Bytes(),
		Width:     c.width,
		Height:    c.height,
	}

	// 序列化为JSON
	jsonData, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("JSON序列化失败: %w", err)
	}

	// 发送到WebSocket
	select {
	case writeChan <- jsonData:
		return nil
	default:
		// Channel满了，跳过此帧
		return nil
	}
}

// SendKeyEvent 发送键盘事件
func (c *Client) SendKeyEvent(keySym uint32, down bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	log.Printf("⌨️  键盘事件: keySym=0x%x, down=%v", keySym, down)
	return nil
}

// SendMouseEvent 发送鼠标事件
func (c *Client) SendMouseEvent(x, y int, buttonMask uint8) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	log.Printf("🖱️  鼠标事件: x=%d, y=%d, mask=%d", x, y, buttonMask)
	return nil
}

// GetFrameBuffer 获取当前帧缓冲
func (c *Client) GetFrameBuffer() *image.RGBA {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.frameBuffer
}

// GetSize 获取屏幕尺寸
func (c *Client) GetSize() (width, height int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.width, c.height
}

// SetFrameRate 设置帧率
func (c *Client) SetFrameRate(fps int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if fps > 0 {
		c.frameRate = time.Second / time.Duration(fps)
		log.Printf("📹 帧率设置为: %d FPS", fps)
	}
}
