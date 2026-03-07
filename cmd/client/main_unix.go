// +build !windows

package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"golang.org/x/net/proxy"
	"golang.org/x/term"

	"github.com/deskgo/deskgo/internal/vnc"
)

type DesktopMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id"`
	Data      []byte `json:"data"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	EventType string `json:"event_type"`
	KeyCode   int    `json:"key_code"`
	MouseX    int    `json:"mouse_x"`
	MouseY    int    `json:"mouse_y"`
	MouseMask int    `json:"mouse_mask"`
}

func main() {
	// 命令行参数
	serverURL := flag.String("server", "http://localhost:8082", "中继服务器URL")
	vncHost := flag.String("host", "", "VNC服务器地址 (例如: 192.168.1.100:5900)")
	vncPassword := flag.String("password", "", "VNC密码（可选）")
	proxyURL := flag.String("proxy", "", "代理服务器地址")
	sessionID := flag.String("session", "", "会话ID（留空自动生成）")
	flag.Parse()

	if *vncHost == "" {
		log.Fatal("❌ 请使用 -host 参数指定VNC服务器地址")
	}

	// 生成或使用提供的会话ID
	sid := *sessionID
	if sid == "" {
		sid = uuid.New().String()
	}

	// 显示连接信息
	printHeader(sid, *serverURL, *vncHost, *proxyURL)

	// 启动桌面会话
	if err := runDesktopSession(*serverURL, *proxyURL, sid, *vncHost, *vncPassword); err != nil {
		log.Fatalf("桌面会话失败: %v", err)
	}
}

func printHeader(sessionID, serverURL, vncHost, proxyURL string) {
	cleanURL := strings.TrimSuffix(serverURL, "/")
	webURL := fmt.Sprintf("%s/session/%s", cleanURL, sessionID)

	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║                    🚀 DeskGo 远程桌面                     ║")
	fmt.Println("╠═══════════════════════════════════════════════════════════╣")
	fmt.Printf("║ 📋 会话ID: %-43s ║\n", sessionID)
	fmt.Printf("║ 🖥️  VNC服务器: %-39s ║\n", vncHost)
	fmt.Printf("║ 🌐 Web访问: %-43s ║\n", webURL)
	if proxyURL != "" {
		fmt.Printf("║ 🔧 代理服务器: %-39s ║\n", proxyURL)
	}
	fmt.Printf("║ 🔗 连接状态: %-43s ║\n", "🟡 连接中...")
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func runDesktopSession(serverURL, proxyURL, sessionID, vncHost, vncPassword string) error {
	// 建立WebSocket连接
	wsURL, _ := buildWebSocketURL(serverURL, sessionID)

	// 创建拨号器
	dialer := websocket.DefaultDialer
	dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	// 如果配置了代理
	if proxyURL != "" {
		proxyDialer, err := createProxyDialer(proxyURL)
		if err != nil {
			return fmt.Errorf("创建代理拨号器失败: %w", err)
		}
		dialer.NetDial = proxyDialer
		log.Printf("✅ 已配置代理: %s", proxyURL)
	}

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("WebSocket连接失败: %w", err)
	}
	defer conn.Close()

	log.Println("✅ WebSocket已连接")
	fmt.Println("✅ 已连接到中继服务器")

	// 创建WebSocket写入channel
	wsWriteChan := make(chan []byte, 100)

	// 启动WebSocket写入goroutine
	go func() {
		for data := range wsWriteChan {
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Printf("❌ WebSocket写入失败: %v", err)
				return
			}
		}
	}()

	// 设置心跳
	setupHeartbeat(conn)

	// 发送初始化消息
	initMsg := DesktopMessage{
		Type:      "init",
		SessionID: sessionID,
		UserID:    "client",
	}
	jsonData, _ := json.Marshal(initMsg)
	wsWriteChan <- jsonData

	// 连接到VNC服务器
	vncClient, err := vnc.Connect(vncHost, vncPassword)
	if err != nil {
		return fmt.Errorf("连接VNC服务器失败: %w", err)
	}
	defer vncClient.Close()

	log.Printf("✅ 已连接到VNC服务器: %s", vncHost)
	fmt.Println("💡 现在可以在浏览器中访问远程桌面了")
	fmt.Println()

	// 处理中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("👋 收到中断信号，正在关闭...")
		vncClient.Close()
		conn.Close()
		os.Exit(0)
	}()

	// 启动VNC帧捕获
	vncClient.StartFrameCapture(wsWriteChan, sessionID)

	// 处理来自WebSocket的输入事件
	go func() {
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("WebSocket读取失败: %v", err)
				return
			}

			var msg DesktopMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				continue
			}

			// 处理输入事件
			if msg.Type == "input" {
				if msg.EventType == "key" {
					vncClient.SendKeyEvent(msg.KeyCode, true)
					vncClient.SendKeyEvent(msg.KeyCode, false)
				} else if msg.EventType == "mouse" {
					vncClient.SendMouseEvent(msg.MouseX, msg.MouseY, msg.MouseMask)
				}
			}
		}
	}()

	// 保持运行
	select {}
}

func buildWebSocketURL(serverURL, sessionID string) (string, error) {
	parsedURL, err := url.Parse(serverURL)
	if err != nil {
		return "", err
	}

	scheme := "ws"
	if parsedURL.Scheme == "https" {
		scheme = "wss"
	}

	wsURL := fmt.Sprintf("%s://%s/api/desktop/%s?type=client&user_id=client",
		scheme, parsedURL.Host, sessionID)
	return wsURL, nil
}

func setupHeartbeat(conn *websocket.Conn) {
	conn.SetPingHandler(func(appData string) error {
		return conn.WriteMessage(websocket.PongMessage, []byte(appData))
	})

	conn.SetPongHandler(func(appData string) error {
		return nil
	})

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, []byte("heartbeat")); err != nil {
					log.Printf("❌ 发送ping失败: %v", err)
					return
				}
			}
		}
	}()
}

func createProxyDialer(proxyURL string) (func(network, addr string) (net.Conn, error), error) {
	proxy, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("解析代理URL失败: %w", err)
	}

	switch proxy.Scheme {
	case "http", "https":
		return createHTTPProxyDialer(proxyURL)
	case "socks5":
		return createSocks5Dialer(proxy.Host)
	default:
		return nil, fmt.Errorf("不支持的代理类型: %s", proxy.Scheme)
	}
}

func createHTTPProxyDialer(proxyURL string) (func(network, addr string) (net.Conn, error), error) {
	proxyParsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("解析代理URL失败: %w", err)
	}

	proxyAddr := proxyParsed.Host
	dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("创建HTTP代理拨号器失败: %w", err)
	}

	return dialer.Dial, nil
}

func createSocks5Dialer(proxyAddr string) (func(network, addr string) (net.Conn, error), error) {
	dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("创建SOCKS5拨号器失败: %w", err)
	}

	return dialer.Dial, nil
}
