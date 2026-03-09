package main

/*
配置文件支持

配置文件查找优先级：
1. ./deskgo.json (当前目录)
2. ~/.deskgo.json (用户主目录)

如果都不存在，创建 ~/.deskgo.json 并设置默认值
*/

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Config 配置结构
type Config struct {
	Server  string `json:"server"`
	Proxy   string `json:"proxy,omitempty"`
	Display int    `json:"display"`
	FPS     int    `json:"fps"`
	Quality int    `json:"quality"`
	Session string `json:"session,omitempty"`

	// H.264 编码配置
	Codec           string `json:"codec,omitempty"`             // "jpeg" 或 "h264"，默认按平台选择
	H264Bitrate     int    `json:"h264_bitrate,omitempty"`      // H.264 比特率 (Kbps)，默认 2000
	H264KeyInterval int    `json:"h264_key_interval,omitempty"` // 关键帧间隔，默认 60
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		Server:          "wss://deskgo.zty8.cn/api/desktop",
		Proxy:           "",
		Display:         0,                      // 主显示器
		FPS:             15,                     // 15 fps
		Quality:         75,                     // JPEG 质量 75%
		Session:         "",                     // 自动生成
		Codec:           defaultPlatformCodec(), // 默认按平台选择
		H264Bitrate:     2000,                   // H.264 2 Mbps
		H264KeyInterval: 60,                     // 每60帧一个关键帧
	}
}

func defaultPlatformCodec() string {
	switch runtime.GOOS {
	case "darwin", "linux":
		return "h264"
	default:
		return "jpeg"
	}
}

func normalizeConfig(config Config) Config {
	defaults := DefaultConfig()

	if config.Server == "" {
		config.Server = defaults.Server
	}
	if config.FPS == 0 {
		config.FPS = defaults.FPS
	}
	if config.Proxy == "" {
		config.Proxy = defaults.Proxy
	}
	if config.Quality == 0 {
		config.Quality = defaults.Quality
	}
	if config.Codec == "" {
		config.Codec = defaults.Codec
	}
	if config.H264Bitrate == 0 {
		config.H264Bitrate = defaults.H264Bitrate
	}
	if config.H264KeyInterval == 0 {
		config.H264KeyInterval = defaults.H264KeyInterval
	}

	return config
}

// LoadConfig 加载配置文件
// 查找优先级：./deskgo.json -> ~/.deskgo.json
// 如果都不存在，创建 ~/.deskgo.json 并返回默认配置
func LoadConfig() (Config, string, error) {
	// 1. 尝试当前目录的配置文件
	localConfigPath := "deskgo.json"
	if config, err := loadConfigFile(localConfigPath); err == nil {
		return config, localConfigPath, nil
	} else if !os.IsNotExist(err) {
		return Config{}, "", fmt.Errorf("读取 %s 失败: %w", localConfigPath, err)
	}

	// 2. 尝试用户主目录的配置文件
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Config{}, "", fmt.Errorf("获取用户主目录失败: %w", err)
	}

	homeConfigPath := filepath.Join(homeDir, ".deskgo.json")
	if config, err := loadConfigFile(homeConfigPath); err == nil {
		return config, homeConfigPath, nil
	} else if !os.IsNotExist(err) {
		return Config{}, "", fmt.Errorf("读取 %s 失败: %w", homeConfigPath, err)
	}

	// 3. 配置文件不存在，创建默认配置
	defaultConfig := DefaultConfig()
	if err := saveConfigFile(homeConfigPath, defaultConfig); err != nil {
		return Config{}, "", fmt.Errorf("创建默认配置文件失败: %w", err)
	}

	return defaultConfig, homeConfigPath, nil
}

// loadConfigFile 从指定路径加载配置文件
func loadConfigFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return normalizeConfig(config), nil
}

// saveConfigFile 保存配置到指定路径
func saveConfigFile(path string, config Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

// MergeWithFlags 合并配置文件和命令行参数
// 命令行参数优先级高于配置文件
func MergeWithFlags(config Config, server *string, proxy *string, display *int, fps *int, quality *int, session *string, codec *string, h264Bitrate *int) Config {
	result := config

	if server != nil && *server != "" {
		result.Server = *server
	}
	if proxy != nil && *proxy != "" {
		result.Proxy = *proxy
	}
	if display != nil && *display != 0 {
		result.Display = *display
	}
	if fps != nil && *fps != 0 {
		result.FPS = *fps
	}
	if quality != nil && *quality != 0 {
		result.Quality = *quality
	}
	if session != nil && *session != "" {
		result.Session = *session
	}
	if codec != nil && *codec != "" {
		result.Codec = *codec
	}
	if h264Bitrate != nil && *h264Bitrate != 0 {
		result.H264Bitrate = *h264Bitrate
	}

	return result
}
