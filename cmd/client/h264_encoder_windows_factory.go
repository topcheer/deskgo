//go:build desktop && windows
// +build desktop,windows

package main

// NewH264Encoder 创建 Windows 平台的原生 Media Foundation H.264 编码器。
func NewH264Encoder() H264Encoder {
	return newWindowsMediaFoundationH264Encoder()
}
