//go:build desktop
// +build desktop

package main

import (
	"encoding/binary"
	"fmt"
	"image"
)

func imageToNV12Buffer(img image.Image) ([]byte, error) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	bgra, err := imageToBGRABuffer(img)
	if err != nil {
		return nil, err
	}

	return bgraToNV12Buffer(bgra, width, height)
}

func bgraToNV12Buffer(bgra []byte, width, height int) ([]byte, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("无效图像尺寸: %dx%d", width, height)
	}
	if width%2 != 0 || height%2 != 0 {
		return nil, fmt.Errorf("NV12 仅支持偶数尺寸，当前为 %dx%d", width, height)
	}

	expectedSize := width * height * 4
	if len(bgra) != expectedSize {
		return nil, fmt.Errorf("BGRA 数据长度不匹配: got %d, want %d", len(bgra), expectedSize)
	}

	output := make([]byte, width*height+(width*height)/2)
	yPlane := output[:width*height]
	uvPlane := output[width*height:]

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			offset := (y*width + x) * 4
			b := int(bgra[offset])
			g := int(bgra[offset+1])
			r := int(bgra[offset+2])

			yPlane[y*width+x] = clampToByte(((66*r + 129*g + 25*b + 128) >> 8) + 16)
		}
	}

	for y := 0; y < height; y += 2 {
		for x := 0; x < width; x += 2 {
			uSum := 0
			vSum := 0

			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					offset := ((y+dy)*width + (x + dx)) * 4
					b := int(bgra[offset])
					g := int(bgra[offset+1])
					r := int(bgra[offset+2])

					uSum += ((-38*r - 74*g + 112*b + 128) >> 8) + 128
					vSum += ((112*r - 94*g - 18*b + 128) >> 8) + 128
				}
			}

			uvOffset := (y/2)*width + x
			uvPlane[uvOffset] = clampToByte(uSum / 4)
			uvPlane[uvOffset+1] = clampToByte(vSum / 4)
		}
	}

	return output, nil
}

func clampToByte(value int) byte {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return byte(value)
}

func extractH264Packet(data []byte, hintedKeyFrame bool) ([]byte, bool, []byte, []byte, error) {
	if len(data) == 0 {
		return nil, false, nil, nil, fmt.Errorf("H.264 输出为空")
	}

	if annexBStartCodeLength(data, 0) != 0 {
		return extractAVCCPacket(ffmpegEncodedPacket{data: data, isKeyFrame: hintedKeyFrame})
	}

	videoNALUs, isKeyFrame, sps, pps, ok := splitAVCCNALUs(data)
	if !ok {
		return nil, false, nil, nil, fmt.Errorf("H.264 输出既不是 Annex-B 也不是 AVCC")
	}
	if len(videoNALUs) == 0 {
		return nil, false, sps, pps, fmt.Errorf("H.264 输出中未找到视频 NALU")
	}

	return annexBNALUsToAVCC(videoNALUs), hintedKeyFrame || isKeyFrame, sps, pps, nil
}

func extractH264ParameterSets(data []byte) ([]byte, []byte) {
	if len(data) == 0 {
		return nil, nil
	}

	if annexBStartCodeLength(data, 0) != 0 {
		return extractH264ParameterSetsFromNALUs(splitAnnexBNALUs(data))
	}

	_, _, sps, pps, ok := splitAVCCNALUs(data)
	if !ok {
		return nil, nil
	}
	return sps, pps
}

func extractH264ParameterSetsFromNALUs(nalus [][]byte) ([]byte, []byte) {
	var sps []byte
	var pps []byte

	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}

		switch nalu[0] & 0x1F {
		case 7:
			sps = append([]byte(nil), nalu...)
		case 8:
			pps = append([]byte(nil), nalu...)
		}
	}

	return sps, pps
}

func splitAVCCNALUs(data []byte) ([][]byte, bool, []byte, []byte, bool) {
	if len(data) < 4 {
		return nil, false, nil, nil, false
	}

	pos := 0
	var videoNALUs [][]byte
	var sps []byte
	var pps []byte
	isKeyFrame := false

	for pos < len(data) {
		if pos+4 > len(data) {
			return nil, false, nil, nil, false
		}

		naluLength := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		pos += 4
		if naluLength <= 0 || pos+naluLength > len(data) {
			return nil, false, nil, nil, false
		}

		nalu := append([]byte(nil), data[pos:pos+naluLength]...)
		pos += naluLength
		if len(nalu) == 0 {
			continue
		}

		naluType := nalu[0] & 0x1F
		switch naluType {
		case 7:
			sps = nalu
		case 8:
			pps = nalu
		case 6, 9:
			continue
		default:
			if naluType >= 1 && naluType <= 5 {
				videoNALUs = append(videoNALUs, nalu)
				if naluType == 5 {
					isKeyFrame = true
				}
			}
		}
	}

	return videoNALUs, isKeyFrame, sps, pps, true
}
