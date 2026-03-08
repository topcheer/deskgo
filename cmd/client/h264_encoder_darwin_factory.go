// +build darwin

package main

// NewH264Encoder 创建 macOS 特定的 H.264 编码器
func NewH264Encoder() *H264EncoderDarwin {
	return new(H264EncoderDarwin)
}
