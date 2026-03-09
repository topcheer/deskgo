//go:build desktop
// +build desktop

package main

/*
H.264 硬件编码器接口

支持平台：
- macOS: VideoToolbox (硬件加速)
- Linux: ffmpeg/libx264（软件编码，要求系统安装 ffmpeg）
- Windows: 空实现（回退到 JPEG）
*/

import (
	"image"
)

// H264Encoder H.264 编码器接口
type H264Encoder interface {
	// Initialize 初始化编码器
	Initialize(width, height int, fps int, bitrateKbps int) error

	// Encode 编码一帧图像
	// forceKeyFrame: 是否强制关键帧（用于新客户端连接时初始化解码器）
	// 返回: H.264数据 (AVCC/长度前缀格式), 是否关键帧, SPS, PPS, 错误
	Encode(img image.Image, forceKeyFrame bool) ([]byte, bool, []byte, []byte, error)

	// Close 关闭编码器
	Close() error

	// IsHardwareAccelerated 是否使用硬件加速
	IsHardwareAccelerated() bool
}

// H264Config H.264 编码配置
type H264Config struct {
	Bitrate     int    // 比特率 (Kbps)
	KeyInterval int    // 关键帧间隔 (帧数)
	Profile     string // H.264 Profile (baseline, main, high)
	Level       string // H.264 Level
}

// DefaultH264Config 默认 H.264 配置
func DefaultH264Config() H264Config {
	return H264Config{
		Bitrate:     2000, // 2 Mbps
		KeyInterval: 60,   // 每60帧一个关键帧 (4秒@15fps)
		Profile:     "baseline",
		Level:       "3.1",
	}
}
