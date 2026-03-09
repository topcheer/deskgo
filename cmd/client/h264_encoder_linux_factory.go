//go:build desktop && linux
// +build desktop,linux

package main

// NewH264Encoder 创建 Linux 平台的 ffmpeg H.264 编码器。
func NewH264Encoder() H264Encoder {
	return newFFmpegH264Encoder()
}
