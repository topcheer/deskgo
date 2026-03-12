//go:build desktop
// +build desktop

package main

import (
	"bytes"
	"testing"
)

func TestBGRAToNV12Buffer(t *testing.T) {
	bgra := []byte{
		0x00, 0x00, 0x00, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0x00, 0x00, 0xFF, 0xFF,
		0x00, 0xFF, 0x00, 0xFF,
	}

	nv12, err := bgraToNV12Buffer(bgra, 2, 2)
	if err != nil {
		t.Fatalf("bgraToNV12Buffer returned error: %v", err)
	}

	want := []byte{16, 235, 82, 144, 100, 132}
	if !bytes.Equal(nv12, want) {
		t.Fatalf("bgraToNV12Buffer = %v, want %v", nv12, want)
	}
}

func TestBGRAToNV12BufferRejectsOddSize(t *testing.T) {
	if _, err := bgraToNV12Buffer(make([]byte, 12), 3, 1); err == nil {
		t.Fatalf("expected odd-size conversion to fail")
	}
}

func TestExtractH264PacketFromAnnexB(t *testing.T) {
	annexB := []byte{
		0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x1F,
		0x00, 0x00, 0x00, 0x01, 0x68, 0xCE, 0x3C, 0x80,
		0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84,
	}

	payload, isKeyFrame, sps, pps, err := extractH264Packet(annexB, false)
	if err != nil {
		t.Fatalf("extractH264Packet returned error: %v", err)
	}
	if !isKeyFrame {
		t.Fatalf("expected keyframe")
	}
	if !bytes.Equal(sps, []byte{0x67, 0x42, 0x00, 0x1F}) {
		t.Fatalf("unexpected SPS: %v", sps)
	}
	if !bytes.Equal(pps, []byte{0x68, 0xCE, 0x3C, 0x80}) {
		t.Fatalf("unexpected PPS: %v", pps)
	}

	wantPayload := annexBNALUsToAVCC([][]byte{{0x65, 0x88, 0x84}})
	if !bytes.Equal(payload, wantPayload) {
		t.Fatalf("unexpected payload: %v", payload)
	}
}

func TestExtractH264PacketFromAVCC(t *testing.T) {
	avcc := annexBNALUsToAVCC([][]byte{
		{0x67, 0x42, 0x00, 0x1F},
		{0x68, 0xCE, 0x3C, 0x80},
		{0x65, 0x88, 0x84},
	})

	payload, isKeyFrame, sps, pps, err := extractH264Packet(avcc, false)
	if err != nil {
		t.Fatalf("extractH264Packet returned error: %v", err)
	}
	if !isKeyFrame {
		t.Fatalf("expected keyframe")
	}
	if !bytes.Equal(sps, []byte{0x67, 0x42, 0x00, 0x1F}) {
		t.Fatalf("unexpected SPS: %v", sps)
	}
	if !bytes.Equal(pps, []byte{0x68, 0xCE, 0x3C, 0x80}) {
		t.Fatalf("unexpected PPS: %v", pps)
	}

	wantPayload := annexBNALUsToAVCC([][]byte{{0x65, 0x88, 0x84}})
	if !bytes.Equal(payload, wantPayload) {
		t.Fatalf("unexpected payload: %v", payload)
	}
}

func TestExtractH264ParameterSetsFromAVCC(t *testing.T) {
	avcc := annexBNALUsToAVCC([][]byte{
		{0x67, 0x42, 0x00, 0x1F},
		{0x68, 0xCE, 0x3C, 0x80},
	})

	sps, pps := extractH264ParameterSets(avcc)
	if !bytes.Equal(sps, []byte{0x67, 0x42, 0x00, 0x1F}) {
		t.Fatalf("unexpected SPS: %v", sps)
	}
	if !bytes.Equal(pps, []byte{0x68, 0xCE, 0x3C, 0x80}) {
		t.Fatalf("unexpected PPS: %v", pps)
	}
}
