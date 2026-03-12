//go:build desktop
// +build desktop

package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ffmpegEncodedPacket struct {
	data       []byte
	isKeyFrame bool
}

// H264EncoderFFmpeg 使用 ffmpeg/libx264 提供跨平台的软件 H.264 编码能力。
type H264EncoderFFmpeg struct {
	mu sync.Mutex

	width       int
	height      int
	fps         int
	bitrateKbps int
	keyInterval int
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	packets     chan ffmpegEncodedPacket
	processErr  chan error
	processDone chan struct{}
	stopCh      chan struct{}
}

type safeStringBuffer struct {
	mu    sync.Mutex
	lines []string
}

func (b *safeStringBuffer) AppendLine(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}

	b.mu.Lock()
	b.lines = append(b.lines, trimmed)
	b.mu.Unlock()
}

func (b *safeStringBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Join(b.lines, "\n")
}

func newFFmpegH264Encoder() *H264EncoderFFmpeg {
	cfg := DefaultH264Config()
	return &H264EncoderFFmpeg{
		keyInterval: cfg.KeyInterval,
	}
}

func (e *H264EncoderFFmpeg) Initialize(width, height int, fps int, bitrateKbps int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("无效的编码尺寸: %dx%d", width, height)
	}
	if fps <= 0 {
		fps = 15
	}
	if bitrateKbps <= 0 {
		bitrateKbps = DefaultH264Config().Bitrate
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.width = width
	e.height = height
	e.fps = fps
	e.bitrateKbps = bitrateKbps
	if e.keyInterval <= 0 {
		e.keyInterval = DefaultH264Config().KeyInterval
	}

	return e.startProcessLocked()
}

func (e *H264EncoderFFmpeg) Encode(img image.Image, forceKeyFrame bool) ([]byte, bool, []byte, []byte, error) {
	e.mu.Lock()
	if forceKeyFrame && e.cmd != nil {
		if err := e.restartProcessLocked(); err != nil {
			e.mu.Unlock()
			return nil, false, nil, nil, err
		}
	}

	stdin := e.stdin
	packetCh := e.packets
	errCh := e.processErr
	e.mu.Unlock()

	if stdin == nil || packetCh == nil || errCh == nil {
		return nil, false, nil, nil, fmt.Errorf("ffmpeg H.264 编码器尚未初始化")
	}

	frameData, err := imageToBGRABuffer(img)
	if err != nil {
		return nil, false, nil, nil, fmt.Errorf("转换 BGRA 帧失败: %w", err)
	}

	if err := writeAll(stdin, frameData); err != nil {
		return nil, false, nil, nil, fmt.Errorf("写入 ffmpeg 原始帧失败: %w", err)
	}

	select {
	case packet, ok := <-packetCh:
		if !ok {
			select {
			case err := <-errCh:
				return nil, false, nil, nil, err
			default:
				return nil, false, nil, nil, fmt.Errorf("ffmpeg H.264 输出已结束")
			}
		}
		return extractAVCCPacket(packet)
	case err := <-errCh:
		return nil, false, nil, nil, err
	case <-time.After(5 * time.Second):
		return nil, false, nil, nil, fmt.Errorf("等待 ffmpeg H.264 输出超时")
	}
}

func (e *H264EncoderFFmpeg) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.stopProcessLocked()
	return nil
}

func (e *H264EncoderFFmpeg) IsHardwareAccelerated() bool {
	return false
}

func (e *H264EncoderFFmpeg) startProcessLocked() error {
	ffmpegPath, err := findFFmpegBinary()
	if err != nil {
		return err
	}
	stderrLog := new(safeStringBuffer)
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-f", "rawvideo",
		"-pix_fmt", "bgra",
		"-video_size", fmt.Sprintf("%dx%d", e.width, e.height),
		"-framerate", strconv.Itoa(e.fps),
		"-i", "pipe:0",
		"-an",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-pix_fmt", "yuv420p",
		"-profile:v", "baseline",
		"-b:v", fmt.Sprintf("%dk", e.bitrateKbps),
		"-maxrate", fmt.Sprintf("%dk", e.bitrateKbps),
		"-bufsize", fmt.Sprintf("%dk", e.bitrateKbps*2),
		"-g", strconv.Itoa(e.keyInterval),
		"-keyint_min", strconv.Itoa(e.keyInterval),
		"-sc_threshold", "0",
		"-bf", "0",
		"-refs", "1",
		"-flush_packets", "1",
		"-x264-params", "repeat-headers=1:aud=1:scenecut=0",
		"-stats_enc_post", "pipe:2",
		"-stats_enc_post_fmt", "{size} {key}",
		"-f", "h264",
		"pipe:1",
	}

	cmd := exec.Command(ffmpegPath, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("创建 ffmpeg stdin 管道失败: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("创建 ffmpeg stdout 管道失败: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("创建 ffmpeg stderr 管道失败: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 ffmpeg H.264 编码器失败: %w", err)
	}

	packetCh := make(chan ffmpegEncodedPacket, 8)
	errCh := make(chan error, 1)
	doneCh := make(chan struct{})
	stopCh := make(chan struct{})
	readDoneCh := make(chan struct{})

	e.cmd = cmd
	e.stdin = stdin
	e.packets = packetCh
	e.processErr = errCh
	e.processDone = doneCh
	e.stopCh = stopCh

	go func() {
		defer close(readDoneCh)
		readFFmpegPackets(stdout, stderr, packetCh, errCh, stopCh, stderrLog)
	}()
	go func() {
		defer close(doneCh)
		if waitErr := cmd.Wait(); waitErr != nil {
			<-readDoneCh
			sendFFmpegError(errCh, fmt.Errorf("ffmpeg H.264 进程退出: %w%s", waitErr, formatFFmpegStderr(stderrLog.String())))
		}
	}()

	return nil
}

func (e *H264EncoderFFmpeg) restartProcessLocked() error {
	e.stopProcessLocked()
	return e.startProcessLocked()
}

func (e *H264EncoderFFmpeg) stopProcessLocked() {
	cmd := e.cmd
	stdin := e.stdin
	doneCh := e.processDone
	stopCh := e.stopCh

	e.cmd = nil
	e.stdin = nil
	e.packets = nil
	e.processErr = nil
	e.processDone = nil
	e.stopCh = nil

	if stopCh != nil {
		close(stopCh)
	}
	if stdin != nil {
		_ = stdin.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if doneCh != nil {
		<-doneCh
	}
}

func readFFmpegPackets(stdout io.Reader, statsReader io.ReadCloser, packetCh chan<- ffmpegEncodedPacket, errCh chan<- error, stopCh <-chan struct{}, stderrLog *safeStringBuffer) {
	defer close(packetCh)
	defer statsReader.Close()

	scanner := bufio.NewScanner(statsReader)
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		size, isKeyFrame, ok := parseFFmpegPacketStatsLine(line)
		if !ok {
			if stderrLog != nil {
				stderrLog.AppendLine(line)
			}
			continue
		}
		if size <= 0 {
			continue
		}

		packet := make([]byte, size)
		if _, err := io.ReadFull(stdout, packet); err != nil {
			sendFFmpegError(errCh, fmt.Errorf("读取 ffmpeg H.264 数据失败: %w", err))
			return
		}

		select {
		case packetCh <- ffmpegEncodedPacket{
			data:       packet,
			isKeyFrame: isKeyFrame,
		}:
		case <-stopCh:
			return
		}
	}

	if err := scanner.Err(); err != nil {
		sendFFmpegError(errCh, fmt.Errorf("读取 ffmpeg 编码统计失败: %w%s", err, formatFFmpegStderr(stderrLog.String())))
	}
}

func parseFFmpegPacketStatsLine(line string) (int, bool, bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) != 2 {
		return 0, false, false
	}

	size, err := strconv.Atoi(fields[0])
	if err != nil || size < 0 {
		return 0, false, false
	}

	switch {
	case strings.EqualFold(fields[1], "K"):
		return size, true, true
	case strings.EqualFold(fields[1], "N"):
		return size, false, true
	default:
		return 0, false, false
	}
}

func extractAVCCPacket(packet ffmpegEncodedPacket) ([]byte, bool, []byte, []byte, error) {
	nalus := splitAnnexBNALUs(packet.data)
	if len(nalus) == 0 {
		return nil, false, nil, nil, fmt.Errorf("ffmpeg H.264 输出为空")
	}

	var videoNALUs [][]byte
	var sps []byte
	var pps []byte
	isKeyFrame := packet.isKeyFrame

	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}

		naluType := nalu[0] & 0x1F
		switch naluType {
		case 7:
			sps = append([]byte(nil), nalu...)
		case 8:
			pps = append([]byte(nil), nalu...)
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

	if len(videoNALUs) == 0 {
		return nil, false, sps, pps, fmt.Errorf("ffmpeg 输出中未找到视频 NALU")
	}

	return annexBNALUsToAVCC(videoNALUs), isKeyFrame, sps, pps, nil
}

func splitAnnexBNALUs(data []byte) [][]byte {
	var nalus [][]byte
	start := -1

	for i := 0; i < len(data); i++ {
		prefixLength := annexBStartCodeLength(data, i)
		if prefixLength == 0 {
			continue
		}

		if start != -1 && i > start {
			nalus = append(nalus, append([]byte(nil), data[start:i]...))
		}

		start = i + prefixLength
		i += prefixLength - 1
	}

	if start != -1 && start < len(data) {
		nalus = append(nalus, append([]byte(nil), data[start:]...))
	}

	return nalus
}

func annexBStartCodeLength(data []byte, index int) int {
	if index+3 > len(data) || data[index] != 0x00 || data[index+1] != 0x00 {
		return 0
	}

	if data[index+2] == 0x01 {
		return 3
	}

	if index+4 <= len(data) && data[index+2] == 0x00 && data[index+3] == 0x01 {
		return 4
	}

	return 0
}

func annexBNALUsToAVCC(nalus [][]byte) []byte {
	totalLength := 0
	for _, nalu := range nalus {
		totalLength += 4 + len(nalu)
	}

	result := make([]byte, totalLength)
	offset := 0
	for _, nalu := range nalus {
		binary.BigEndian.PutUint32(result[offset:offset+4], uint32(len(nalu)))
		offset += 4
		copy(result[offset:], nalu)
		offset += len(nalu)
	}

	return result
}

func imageToBGRABuffer(img image.Image) ([]byte, error) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("无效图像尺寸: %dx%d", width, height)
	}

	buffer := make([]byte, width*height*4)

	if rgba, ok := img.(*image.RGBA); ok {
		offsetX := bounds.Min.X - rgba.Rect.Min.X
		offsetY := bounds.Min.Y - rgba.Rect.Min.Y

		for y := 0; y < height; y++ {
			srcRowStart := (offsetY+y)*rgba.Stride + offsetX*4
			srcRow := rgba.Pix[srcRowStart : srcRowStart+width*4]
			dstRow := buffer[y*width*4 : (y+1)*width*4]

			for x := 0; x < width; x++ {
				src := x * 4
				dst := x * 4
				dstRow[dst] = srcRow[src+2]
				dstRow[dst+1] = srcRow[src+1]
				dstRow[dst+2] = srcRow[src]
				dstRow[dst+3] = srcRow[src+3]
			}
		}

		return buffer, nil
	}

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, a := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			offset := (y*width + x) * 4
			buffer[offset] = byte(b >> 8)
			buffer[offset+1] = byte(g >> 8)
			buffer[offset+2] = byte(r >> 8)
			buffer[offset+3] = byte(a >> 8)
		}
	}

	return buffer, nil
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		data = data[n:]
	}

	return nil
}

func sendFFmpegError(errCh chan<- error, err error) {
	select {
	case errCh <- err:
	default:
	}
}

func formatFFmpegStderr(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}

	return ": " + message
}

func findFFmpegBinary() (string, error) {
	candidates := []string{"ffmpeg"}
	if runtime.GOOS == "windows" {
		candidates = []string{"ffmpeg.exe", "ffmpeg"}
	}

	if executablePath, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executablePath)
		for _, candidateName := range candidates {
			candidatePath := filepath.Join(executableDir, candidateName)
			if info, statErr := os.Stat(candidatePath); statErr == nil && !info.IsDir() {
				return candidatePath, nil
			}
		}
	}

	for _, candidateName := range candidates {
		if candidatePath, err := exec.LookPath(candidateName); err == nil {
			return candidatePath, nil
		}
	}

	return "", fmt.Errorf("未找到 ffmpeg，请先安装 ffmpeg，或将 ffmpeg 可执行文件放到 DeskGo 可执行文件同目录后再启用 H.264")
}
