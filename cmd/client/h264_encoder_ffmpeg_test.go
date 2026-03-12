//go:build desktop
// +build desktop

package main

import (
	"image"
	"image/color"
	"testing"
)

func TestParseFFmpegPacketStatsLine(t *testing.T) {
	testCases := []struct {
		name       string
		line       string
		wantSize   int
		wantKey    bool
		wantParsed bool
	}{
		{name: "keyframe", line: "1234 K", wantSize: 1234, wantKey: true, wantParsed: true},
		{name: "non-keyframe", line: "567 N", wantSize: 567, wantKey: false, wantParsed: true},
		{name: "stderr line", line: "[libx264 @ 0x123] using cpu capabilities", wantParsed: false},
		{name: "invalid marker", line: "88 X", wantParsed: false},
		{name: "negative size", line: "-1 K", wantParsed: false},
	}

	for _, tc := range testCases {
		size, isKeyFrame, ok := parseFFmpegPacketStatsLine(tc.line)
		if size != tc.wantSize || isKeyFrame != tc.wantKey || ok != tc.wantParsed {
			t.Fatalf("%s: parseFFmpegPacketStatsLine(%q) = (%d, %v, %v), want (%d, %v, %v)", tc.name, tc.line, size, isKeyFrame, ok, tc.wantSize, tc.wantKey, tc.wantParsed)
		}
	}
}

func TestFFmpegEncoderEncodesKeyFrame(t *testing.T) {
	if _, err := findFFmpegBinary(); err != nil {
		t.Skipf("ffmpeg not available: %v", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(x * 8),
				G: uint8(y * 8),
				B: uint8((x + y) * 4),
				A: 0xFF,
			})
		}
	}

	encoder := newFFmpegH264Encoder()
	if err := encoder.Initialize(32, 32, 15, 2000); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	defer func() {
		if err := encoder.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	}()

	data, isKeyFrame, sps, pps, err := encoder.Encode(img, true)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	if !isKeyFrame {
		t.Fatalf("expected keyframe output")
	}
	if len(data) == 0 {
		t.Fatalf("expected non-empty H.264 payload")
	}
	if len(sps) == 0 || len(pps) == 0 {
		t.Fatalf("expected SPS/PPS data, got SPS=%d PPS=%d", len(sps), len(pps))
	}
}
