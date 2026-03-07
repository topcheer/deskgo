package vnc

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"net"
	"time"

	"github.com/mitchellh/go-vnc"
)

// Client VNC客户端
type Client struct {
	conn         net.Conn
	server       *vnc.ClientConn
	password     string
	width        int
	height       int
	frameBuffer  *image.RGBA
	encodings    []int32
}

// Connect 连接到VNC服务器
func Connect(host, password string) (*Client, error) {
	// 连接到VNC服务器
	conn, err := net.DialTimeout("tcp", host, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("连接VNC服务器失败: %w", err)
	}

	// 创建VNC客户端连接
	vncClient, err := vnc.Client(conn, &vnc.ClientConfig{
		Password: password,
	})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("VNC握手失败: %w", err)
	}

	client := &Client{
		conn:     conn,
		server:   vncClient,
		password: password,
	}

	// 获取屏幕尺寸
	width, height := vncClient.Width(), vncClient.Height()
	client.width = int(width)
	client.height = int(height)
	client.frameBuffer = image.NewRGBA(image.Rect(0, 0, client.width, client.height))

	// 设置支持的编码格式
	client.encodings = []int32{
		0,  // Raw
		1,  // CopyRect
		2,  // RRE
		// -239, // Tight (如果支持)
	}

	log.Printf("📺 VNC屏幕尺寸: %dx%d", client.width, client.height)

	return client, nil
}

// Close 关闭连接
func (c *Client) Close() error {
	if c.server != nil {
		c.server.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// StartFrameCapture 启动帧捕获
func (c *Client) StartFrameCapture(writeChan chan<- []byte, sessionID string) {
	// 发送像素格式请求
	c.server.SetEncodings(c.encodings)
	c.server.RequestFramebufferUpdate(0, 0, uint16(c.width), uint16(c.height))

	// 持续接收帧更新
	go func() {
		for {
			msg, err := c.server.ReadMessage()
			if err != nil {
				if err != io.EOF {
					log.Printf("❌ VNC消息读取失败: %v", err)
				}
				return
			}

			switch msg := msg.(type) {
			case *vnc.FramebufferUpdateMessage:
				c.handleFramebufferUpdate(msg, writeChan, sessionID)

				// 请求下一帧
				c.server.RequestFramebufferUpdate(0, 0, uint16(c.width), uint16(c.height))
			}
		}
	}()
}

// handleFramebufferUpdate 处理帧缓冲更新
func (c *Client) handleFramebufferUpdate(msg *vnc.FramebufferUpdate, writeChan chan<- []byte, sessionID string) {
	for _, rect := range msg.Rectangles {
		c.handleRectangle(rect)
	}

	// 将帧缓冲转换为PNG并发送
	// 注意：实际实现应该优化为增量更新和压缩
	// 这里简化为发送完整帧

	// TODO: 实现高效的图像编码和传输
	// 1. 使用增量更新
	// 2. 使用更好的压缩算法（如WebP）
	// 3. 添加帧率控制
}

// handleRectangle 处理矩形更新
func (c *Client) handleRectangle(rect *vnc.FramebufferRectangle) {
	switch rect.Type() {
	case 0: // Raw编码
		c.handleRawRect(rect)
	// 可以添加其他编码格式的处理
	}
}

// handleRawRect 处理Raw编码的矩形
func (c *Client) handleRawRect(rect *vnc.FramebufferRectangle) {
	bytesPerPixel := int(c.server.PixelFormat().BitsPerPixel / 8)
	stride := rect.Width * bytesPerPixel

	data := make([]byte, stride*int(rect.Height))
	copy(data, rect.Data.([]byte))

	// 更新帧缓冲
	yStart := int(rect.Y)
	for y := 0; y < int(rect.Height); y++ {
		for x := 0; x < int(rect.Width); x++ {
			offset := y*stride + x*bytesPerPixel

			var r, g, b, a uint8

			// 根据像素格式解析颜色
			// 这里简化处理，实际应该根据PixelFormat正确解析
			if bytesPerPixel == 4 {
				b = data[offset]
				g = data[offset+1]
				r = data[offset+2]
				a = data[offset+3]
			} else if bytesPerPixel == 3 {
				b = data[offset]
				g = data[offset+1]
				r = data[offset+2]
				a = 255
			}

			c.frameBuffer.SetRGBA(int(rect.X)+x, yStart+y, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}
}

// SendKeyEvent 发送键盘事件
func (c *Client) SendKeyEvent(keyCode int, down bool) {
	// VNC键码映射
	// 这里简化处理，实际需要完整的键码映射表
	key := uint32(keyCode)

	var err error
	if down {
		err = c.server.KeyEvent(key, true)
	} else {
		err = c.server.KeyEvent(key, false)
	}

	if err != nil {
		log.Printf("❌ 发送键盘事件失败: %v", err)
	}
}

// SendMouseEvent 发送鼠标事件
func (c *Client) SendMouseEvent(x, y, mask int) {
	err := c.server.PointerEvent(uint8(mask), uint16(x), uint16(y))
	if err != nil {
		log.Printf("❌ 发送鼠标事件失败: %v", err)
	}
}

// GetFrameBuffer 获取当前帧缓冲
func (c *Client) GetFrameBuffer() *image.RGBA {
	return c.frameBuffer
}

// GetSize 获取屏幕尺寸
func (c *Client) GetSize() (width, height int) {
	return c.width, c.height
}
