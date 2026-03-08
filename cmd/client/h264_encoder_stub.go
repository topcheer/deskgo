// +build desktop,!darwin

package main

/*
H.264 编码器空实现（非 macOS 平台）

用于 Windows/Linux 平台，自动回退到 JPEG 编码
*/

import (
	"errors"
	"image"
)

// H264EncoderStub 空实现编码器（不支持硬件编码）
type H264EncoderStub struct{}

// Initialize 初始化（失败）
func (e *H264EncoderStub) Initialize(width, height int, fps int, bitrateKbps int) error {
	return errors.New("H.264 编码器在当前平台不支持")
}

// Encode 编码（失败）
func (e *H264EncoderStub) Encode(img image.Image, forceKeyFrame bool) ([]byte, bool, []byte, []byte, error) {
	return nil, false, nil, nil, errors.New("H.264 编码器在当前平台不支持")
}

// Close 关闭
func (e *H264EncoderStub) Close() error {
	return nil
}

// IsHardwareAccelerated 是否硬件加速
func (e *H264EncoderStub) IsHardwareAccelerated() bool {
	return false
}

// NewH264Encoder 创建非 macOS 平台的 H.264 编码器
func NewH264Encoder() H264Encoder {
	return new(H264EncoderStub)
}
