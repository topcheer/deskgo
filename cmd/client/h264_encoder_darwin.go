//go:build darwin
// +build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework VideoToolbox -framework CoreMedia -framework CoreFoundation -framework CoreVideo

#include <VideoToolbox/VideoToolbox.h>
#include <CoreMedia/CoreMedia.h>
#include <CoreFoundation/CoreFoundation.h>
#include <CoreVideo/CoreVideo.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>
#include <stdio.h>
#include <pthread.h>

// 全局 CFRunLoop
static CFRunLoopRef gRunLoop = NULL;

// 编码结果
typedef struct {
    int status;
    uint8_t *data;
    size_t dataLength;
    bool isKeyFrame;
    uint8_t *sps;
    size_t spsLength;
    uint8_t *pps;
    size_t ppsLength;
    volatile bool completed;
} EncodeResult;

// 释放内存
void cleanupResult(EncodeResult *r) {
    if (!r) return;
    if (r->data) { free(r->data); r->data = NULL; }
    if (r->sps) { free(r->sps); r->sps = NULL; }
    if (r->pps) { free(r->pps); r->pps = NULL; }
    r->completed = false;
}

// 释放会话
void releaseSession(VTCompressionSessionRef s) {
    if (s) {
        VTCompressionSessionInvalidate(s);
        CFRelease(s);
    }
}

// 输出回调
void outputCallback(void *refcon, void *srcRefCon, int status, unsigned int flags, CMSampleBufferRef buf) {
    EncodeResult *r = (EncodeResult *)srcRefCon;  // 使用 srcRefCon（每帧的 result）
    if (!r) return;

    r->status = status;
    if (status != 0) { r->completed = true; return; }

    CMBlockBufferRef dbuf = CMSampleBufferGetDataBuffer(buf);
    if (!dbuf) { r->status = -1; r->completed = true; return; }

    size_t len = CMBlockBufferGetDataLength(dbuf);
    if (len == 0) { r->status = -2; r->completed = true; return; }

    r->data = malloc(len);
    if (!r->data) { r->status = -3; r->completed = true; return; }

    OSStatus copyStatus = CMBlockBufferCopyDataBytes(dbuf, 0, len, r->data);
    if (copyStatus != kCMBlockBufferNoErr) {
        free(r->data);
        r->data = NULL;
        r->status = copyStatus;
        r->completed = true;
        return;
    }
    r->dataLength = len;

    // 关键帧检测
    r->isKeyFrame = false;
    CFArrayRef att = CMSampleBufferGetSampleAttachmentsArray(buf, 0);
    if (att && CFArrayGetCount(att) > 0) {
        CFDictionaryRef d = CFArrayGetValueAtIndex(att, 0);
        if (d && !CFDictionaryContainsKey(d, kCMSampleAttachmentKey_NotSync)) {
            r->isKeyFrame = true;
        }
    }

    // SPS/PPS
    if (r->isKeyFrame) {
        CMFormatDescriptionRef fmt = CMSampleBufferGetFormatDescription(buf);
        if (fmt) {
            const uint8_t *spsPtr, *ppsPtr;
            size_t spsLen, ppsLen;
            if (CMVideoFormatDescriptionGetH264ParameterSetAtIndex(fmt, 0, &spsPtr, &spsLen, NULL, NULL) == 0) {
                r->sps = malloc(spsLen);
                memcpy(r->sps, spsPtr, spsLen);
                r->spsLength = spsLen;
            }
            if (CMVideoFormatDescriptionGetH264ParameterSetAtIndex(fmt, 1, &ppsPtr, &ppsLen, NULL, NULL) == 0) {
                r->pps = malloc(ppsLen);
                memcpy(r->pps, ppsPtr, ppsLen);
                r->ppsLength = ppsLen;
            }
        }
    }

    r->completed = true;
}

// 创建会话
VTCompressionSessionRef createSession(int w, int h, int fps, int bitrate, EncodeResult *r, int *err) {
    VTCompressionSessionRef s = NULL;
    CFBooleanRef hw = kCFBooleanTrue;
    CFStringRef hwKey = CFSTR("VTVideoEncoderSpecification_EnableHardwareAcceleratedVideoEncoder");
    CFTypeRef keys[1] = { (CFTypeRef)hwKey };
    CFTypeRef vals[1] = { (CFTypeRef)hw };
    CFDictionaryRef spec = CFDictionaryCreate(NULL, (const void**)keys, (const void**)vals, 1,
                                              &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);

    // 创建会话（先不传 runloop）
    *err = VTCompressionSessionCreate(NULL, w, h, kCMVideoCodecType_H264, spec, NULL, NULL, outputCallback, NULL, &s);
    CFRelease(spec);

    if (*err != 0) return NULL;

    // 设置回调运行在 CFRunLoop 上
    if (gRunLoop != NULL) {
        VTSessionSetProperty(s, CFSTR("VTCompressionPropertyKey_OutputCallbackRunLoop"), gRunLoop);
    }

    if (*err != 0) return NULL;

    // 设置参数
    int br = bitrate * 1000;
    CFNumberRef n = CFNumberCreate(NULL, kCFNumberIntType, &br);
    VTSessionSetProperty(s, kVTCompressionPropertyKey_AverageBitRate, n);
    CFRelease(n);

    // 关键帧间隔：设置为 1，强制每帧都是关键帧
    // 这样可以确保每帧都是 IDR，解决浏览器无法配置解码器的问题
    int ki = 1;
    n = CFNumberCreate(NULL, kCFNumberIntType, &ki);
    VTSessionSetProperty(s, kVTCompressionPropertyKey_MaxKeyFrameInterval, n);
    CFRelease(n);

    n = CFNumberCreate(NULL, kCFNumberIntType, &fps);
    VTSessionSetProperty(s, kVTCompressionPropertyKey_ExpectedFrameRate, n);
    CFRelease(n);

    VTSessionSetProperty(s, kVTCompressionPropertyKey_ProfileLevel, CFSTR("H264_Baseline_Profile_Level_3.1"));
    VTSessionSetProperty(s, kVTCompressionPropertyKey_RealTime, kCFBooleanTrue);

    *err = VTCompressionSessionPrepareToEncodeFrames(s);
    if (*err != 0) {
        releaseSession(s);
        return NULL;
    }

    return s;
}

// 编码帧
int encodeFrame(VTCompressionSessionRef s, void *pb, int fps, int forceKeyFrame, EncodeResult *r) {
    CMTime t;
    t.value = 0;
    t.timescale = fps;
    t.flags = 1;
    t.epoch = 0;

    OSStatus status;

    CFMutableDictionaryRef frameInfo = NULL;

    // 强制关键帧的策略（简化版，避免卡死）
    if (forceKeyFrame) {
        printf("🔍 [强制关键帧] 尝试强制下一帧成为关键帧\n");

        // 只设置 MaxKeyFrameInterval = 1（最简单的方法）
        int one = 1;
        CFNumberRef oneRef = CFNumberCreate(NULL, kCFNumberIntType, &one);
        status = VTSessionSetProperty(s, kVTCompressionPropertyKey_MaxKeyFrameInterval, oneRef);
        CFRelease(oneRef);

        if (status != 0) {
            printf("⚠️  [强制关键帧] 设置 MaxKeyFrameInterval 失败: %d，继续编码\n", (int)status);
        } else {
            printf("✅ [强制关键帧] 已设置 MaxKeyFrameInterval = 1\n");
        }

        // 注意：不调用 CompleteFrames，不修改 AllowFrameReordering
        // 这些可能导致编码器卡死
    }

    status = VTCompressionSessionEncodeFrame(s, (CVPixelBufferRef)pb, t, t, frameInfo, r, NULL);

    if (frameInfo) {
        CFRelease(frameInfo);
    }

    // 注意：不再恢复 MaxKeyFrameInterval
    // 因为初始化时已经设置为 2，不需要频繁修改
    if (status == 0) {
        VTCompressionSessionCompleteFrames(s, kCMTimeInvalid);
    }

    return status;
}

// 创建 CVPixelBuffer
CVPixelBufferRef createPixelBuffer(int w, int h, void *pixels, size_t srcBytesPerRow, int *err) {
    CVPixelBufferRef pb = NULL;
    *err = CVPixelBufferCreate(NULL, w, h, kCVPixelFormatType_32BGRA, NULL, &pb);
    if (*err != 0) return NULL;

    CVPixelBufferLockBaseAddress(pb, 0);
    uint8_t *base = (uint8_t *)CVPixelBufferGetBaseAddress(pb);
    size_t dstBytesPerRow = CVPixelBufferGetBytesPerRow(pb);
    size_t rowBytes = (size_t)w * 4;

    memset(base, 0, (size_t)h * dstBytesPerRow);
    for (int y = 0; y < h; y++) {
        memcpy(base + ((size_t)y * dstBytesPerRow),
               ((uint8_t *)pixels) + ((size_t)y * srcBytesPerRow),
               rowBytes);
    }
    CVPixelBufferUnlockBaseAddress(pb, 0);

    return pb;
}

// 释放 CVPixelBuffer
void releasePixelBuffer(CVPixelBufferRef pb) {
    if (pb) CFRelease(pb);
}

// 检查会话是否有效
bool isSessionValid(VTCompressionSessionRef s) {
    return s != NULL;
}

// 检查像素缓冲是否有效
bool isPixelBufferValid(CVPixelBufferRef pb) {
    return pb != NULL;
}

// 检查 CFRunLoop 是否已启动
bool isRunLoopReady() {
    return gRunLoop != NULL;
}

// Dummy timer callback to keep runloop alive
void dummyTimerCallback(CFRunLoopTimerRef timer, void *info) {
    // Do nothing, just keep the runloop alive
}

// 启动 CFRunLoop 线程
void* runLoopThread(void *arg) {
    gRunLoop = CFRunLoopGetCurrent();
    CFRetain(gRunLoop);

    // 添加一个重复的 timer 来保持 runloop 运行
    CFRunLoopTimerContext context = {0};
    context.info = NULL;

    CFRunLoopTimerRef timer = CFRunLoopTimerCreate(
        NULL,
        CFAbsoluteTimeGetCurrent() + 0.1,  // 0.1秒后首次触发
        0.1,                                // 每0.1秒触发一次
        0,                                  // 无 flags
        0,                                  // 优先级
        dummyTimerCallback,
        &context
    );

    CFRunLoopAddTimer(gRunLoop, timer, kCFRunLoopCommonModes);
    CFRunLoopRun();

    CFRunLoopRemoveTimer(gRunLoop, timer, kCFRunLoopCommonModes);
    CFRelease(timer);
    CFRelease(gRunLoop);
    gRunLoop = NULL;
    return NULL;
}

// 启动 CFRunLoop 线程
void startRunLoopThread() {
    if (gRunLoop != NULL) return;

    pthread_t thread;
    pthread_create(&thread, NULL, runLoopThread, NULL);
    pthread_detach(thread);

    // 等待 runloop 初始化
    int count = 0;
    while (gRunLoop == NULL && count < 100) {
        usleep(10000); // 10ms
        count++;
    }
}
*/
import "C"
import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"runtime"
	"sync"
	"time"
	"unsafe"
)

type H264EncoderDarwin struct {
	session     C.VTCompressionSessionRef
	width       int
	height      int
	fps         int
	bitrate     int
	initialized bool
	mu          sync.Mutex

	sps, pps []byte
}

func (e *H264EncoderDarwin) Initialize(width, height int, fps, bitrateKbps int) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.initialized {
		return nil
	}

	// 启动 CFRunLoop 线程（全局只需启动一次）
	C.startRunLoopThread()

	// 等待 CFRunLoop 线程启动完成（最多等待1秒）
	for i := 0; i < 100; i++ {
		if C.isRunLoopReady() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !C.isRunLoopReady() {
		return fmt.Errorf("CFRunLoop 线程启动超时")
	}

	e.width = width
	e.height = height
	e.fps = fps
	e.bitrate = bitrateKbps

	var err C.int
	e.session = C.createSession(C.int(width), C.int(height), C.int(fps), C.int(bitrateKbps), nil, &err)
	if !C.isSessionValid(e.session) || err != 0 {
		return fmt.Errorf("创建 VideoToolbox 会话失败: %d", err)
	}

	e.initialized = true
	return nil
}

func (e *H264EncoderDarwin) Encode(img image.Image, forceKeyFrame bool) ([]byte, bool, []byte, []byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.initialized {
		return nil, false, nil, nil, errors.New("编码器未初始化")
	}

	// 为每一帧创建新的 result（不要重用，避免并发问题）
	result := (*C.EncodeResult)(C.malloc(C.size_t(unsafe.Sizeof(C.EncodeResult{}))))
	if result == nil {
		return nil, false, nil, nil, errors.New("分配编码结果失败")
	}
	*result = C.EncodeResult{}
	defer C.free(unsafe.Pointer(result))
	defer C.cleanupResult(result)

	// 转换图像格式
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// 创建 BGRA 像素数据（与 macOS/VideoToolbox 常用像素格式一致）
	pixels := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y))
			r, g, b, a := c.RGBA()
			offset := (y*w + x) * 4
			pixels[offset+0] = byte(b >> 8)
			pixels[offset+1] = byte(g >> 8)
			pixels[offset+2] = byte(r >> 8)
			pixels[offset+3] = byte(a >> 8)
		}
	}

	// 创建 CVPixelBuffer
	var err C.int
	pb := C.createPixelBuffer(C.int(w), C.int(h), unsafe.Pointer(&pixels[0]), C.size_t(w*4), &err)
	if pb == 0 || err != 0 {
		return nil, false, nil, nil, fmt.Errorf("创建 CVPixelBuffer 失败: %d", err)
	}
	defer C.CFRelease(C.CFTypeRef(pb))

	// 编码
	var forceKeyFrameC C.int
	if forceKeyFrame {
		forceKeyFrameC = 1
		fmt.Printf("🔍 [C] 强制关键帧模式已启用 (forceKeyFrameC=1)\n")
	}
	err = C.encodeFrame(e.session, unsafe.Pointer(pb), C.int(e.fps), forceKeyFrameC, result)
	if err != 0 {
		return nil, false, nil, nil, fmt.Errorf("编码失败: %d", err)
	}
	if forceKeyFrame {
		fmt.Printf("🔍 [C] encodeFrame 返回，等待编码完成...\n")
	}

	// 等待完成
	deadline := time.Now().Add(5 * time.Second)
	for !result.completed {
		if time.Now().After(deadline) {
			return nil, false, nil, nil, errors.New("编码超时")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if result.status != 0 {
		return nil, false, nil, nil, fmt.Errorf("编码失败: %d", result.status)
	}

	var data []byte
	if result.data != nil {
		data = C.GoBytes(unsafe.Pointer(result.data), C.int(result.dataLength))
	}

	// 检查 NALU 类型，确认是否是真正的 IDR 帧
	isKeyFrame := bool(result.isKeyFrame)
	hasSPSPPS := (result.sps != nil && result.pps != nil)

	if isKeyFrame && len(data) > 4 {
		// 遍历所有 NALU，跳过 SEI（类型 6），找到第一个视频 NALU
		offset := 0
		foundIDR := false
		firstNonSEINALU := -1

		for offset < len(data)-4 {
			naluLength := (uint32(data[offset]) << 24) | (uint32(data[offset+1]) << 16) | (uint32(data[offset+2]) << 8) | uint32(data[offset+3])
			if naluLength == 0 || int(naluLength) > len(data)-offset-4 {
				break
			}

			naluType := data[offset+4] & 0x1F

			// 跳过非视频 NALU（SEI=6, SPS=7, PPS=8, AUD=9）
			if naluType >= 1 && naluType <= 5 {
				// 这是视频 NALU（1-5）
				if firstNonSEINALU == -1 {
					firstNonSEINALU = int(naluType)
				}

				if naluType == 5 {
					foundIDR = true
					fmt.Printf("✅ [C] 找到 IDR 帧 (NALU 类型=5)\n")
					break
				}
			}

			offset += 4 + int(naluLength)
		}

		if !foundIDR {
			if firstNonSEINALU != -1 {
				fmt.Printf("⚠️  [C] VideoToolbox 标记为关键帧，但第一个视频 NALU 类型=%d (不是 IDR=5)\n", firstNonSEINALU)
			} else {
				fmt.Printf("⚠️  [C] VideoToolbox 标记为关键帧，但没有找到视频 NALU\n")
			}
			isKeyFrame = false
		}
	}

	if forceKeyFrame {
		fmt.Printf("🔍 [C] 编码完成，isKeyFrame=%v (期望=true)\n", isKeyFrame)
		if !isKeyFrame {
			fmt.Printf("⚠️  [C] 警告：请求了强制关键帧，但编码器返回了普通帧！\n")
		}
	}

	var sps, pps []byte

	// 处理 SPS/PPS
	if hasSPSPPS {
		if result.sps != nil {
			sps = C.GoBytes(unsafe.Pointer(result.sps), C.int(result.spsLength))
			e.sps = sps
			fmt.Printf("🔍 SPS 已提取 (%d字节)\n", len(sps))
		}
		if result.pps != nil {
			pps = C.GoBytes(unsafe.Pointer(result.pps), C.int(result.ppsLength))
			e.pps = pps
			fmt.Printf("🔍 PPS 已提取 (%d字节)\n", len(pps))
		}

		// ⚠️ 重要：不要强制标记为 key！浏览器会检测实际的 NALU 类型
		// 如果不是 IDR 帧，标记为 key 会导致解码错误
		if !isKeyFrame {
			fmt.Printf("⚠️  [C] 帧包含 SPS/PPS，但不是 IDR 帧，保持为普通帧\n")
		}
	} else {
		// 使用缓存的 SPS/PPS
		sps = e.sps
		pps = e.pps
		if len(sps) > 0 {
			fmt.Printf("🔍 使用缓存的 SPS (%d字节)\n", len(sps))
		}
		if len(pps) > 0 {
			fmt.Printf("🔍 使用缓存的 PPS (%d字节)\n", len(pps))
		}
	}

	// 调试：打印帧数据前32字节
	if len(data) > 0 {
		fmt.Printf("🔍 帧 %s 数据 (%d字节)，前32字节:\n", map[bool]string{true: "关键", false: "普通"}[isKeyFrame], len(data))
		for i := 0; i < min(32, len(data)); i++ {
			if i%16 == 0 {
				fmt.Printf("   %04X: ", i)
			}
			fmt.Printf("%02X ", data[i])
			if i%16 == 15 || i == len(data)-1 {
				fmt.Println()
			}
		}

		// 检测数据格式
		if len(data) >= 4 {
			first4 := (uint32(data[0]) << 24) | (uint32(data[1]) << 16) | (uint32(data[2]) << 8) | uint32(data[3])
			if data[0] == 0x00 && data[1] == 0x00 && (data[2] == 0x01 || (data[2] == 0x00 && data[3] == 0x01)) {
				fmt.Printf("   ✅ 检测到 Annex-B 格式（起始码）\n")
			} else if first4 > 0 && first4 <= uint32(len(data)-4) {
				fmt.Printf("   ✅ 检测到 AVCC 格式（首个NALU长度=%d）\n", first4)
			} else {
				fmt.Printf("   ⚠️  未知格式，前4字节=%08X\n", first4)
			}
		}
	}

	return data, isKeyFrame, sps, pps, nil
}

func (e *H264EncoderDarwin) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.initialized {
		return nil
	}

	if C.isSessionValid(e.session) {
		C.releaseSession(e.session)
		e.session = 0
	}

	e.initialized = false
	return nil
}

func (e *H264EncoderDarwin) IsHardwareAccelerated() bool {
	return runtime.GOOS == "darwin"
}

// ConvertAnnexBToAVCC 转换 Annex B 到 AVCC 格式
func ConvertAnnexBToAVCC(data []byte) []byte {
	// 简单实现：查找起始码并添加长度前缀
	var nalus [][]byte
	start := 0

	for i := 0; i < len(data)-3; i++ {
		if data[i] == 0x00 && data[i+1] == 0x00 && data[i+2] == 0x00 && data[i+3] == 0x01 {
			if start > 0 || i > 0 {
				if start < i {
					nalus = append(nalus, data[start:i])
				}
			}
			start = i + 4
		}
	}

	if start < len(data) {
		nalus = append(nalus, data[start:])
	}

	// 构建 AVCC 格式
	result := make([]byte, 0)
	for _, nalu := range nalus {
		// 添加长度前缀（大端序）
		length := make([]byte, 4)
		binary.BigEndian.PutUint32(length, uint32(len(nalu)))
		result = append(result, length...)
		result = append(result, nalu...)
	}

	return result
}
