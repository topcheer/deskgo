//go:build desktop && windows
// +build desktop,windows

package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	ole32DLL  = syscall.NewLazyDLL("ole32.dll")
	mfplatDLL = syscall.NewLazyDLL("mfplat.dll")

	procCoInitializeEx       = ole32DLL.NewProc("CoInitializeEx")
	procCoUninitialize       = ole32DLL.NewProc("CoUninitialize")
	procCoCreateInstance     = ole32DLL.NewProc("CoCreateInstance")
	procMFStartup            = mfplatDLL.NewProc("MFStartup")
	procMFShutdown           = mfplatDLL.NewProc("MFShutdown")
	procMFCreateMediaType    = mfplatDLL.NewProc("MFCreateMediaType")
	procMFCreateSample       = mfplatDLL.NewProc("MFCreateSample")
	procMFCreateMemoryBuffer = mfplatDLL.NewProc("MFCreateMemoryBuffer")
)

const (
	coinitMultithreaded = 0x0
	clsctxInprocServer  = 0x1
	mfVersion           = 0x00020070
	mfStartupFull       = 0x0

	mfVideoInterlaceProgressive = 2
	avEncH264ProfileBase        = 66

	mftMessageNotifyBeginStreaming = 0x10000000
	mftMessageNotifyStartOfStream  = 0x10000003

	mftOutputStreamProvidesSamples  = 0x100
	mftOutputStreamCanProvideSample = 0x200

	mftOutputDataBufferFormatChange = 0x100
	mftOutputDataBufferIncomplete   = 0x1000000

	hresultSFalse                = 0x00000001
	hresultENotImpl              = 0x80004001
	hresultRPCEChangedMode       = 0x80010106
	hresultMFEAttributeNotFound  = 0xc00d36e6
	hresultMFENotAccepting       = 0xc00d36b5
	hresultMFETransformTypeUnset = 0xc00d6d60
	hresultMFETransformChange    = 0xc00d6d61
	hresultMFETransformNeedInput = 0xc00d6d72
)

var (
	clsidCMSH264EncoderMFT = winGUID{0x6ca50344, 0x051a, 0x4ded, [8]byte{0x97, 0x79, 0xa4, 0x33, 0x05, 0x16, 0x5e, 0x35}}
	iidIMFTransform        = winGUID{0xbf94c121, 0x5b05, 0x4e6f, [8]byte{0x80, 0x00, 0xba, 0x59, 0x89, 0x61, 0x41, 0x4d}}

	mfMediaTypeVideo  = winGUID{0x73646976, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
	mfVideoFormatH264 = makeMediaSubtypeGUID("H264")
	mfVideoFormatNV12 = makeMediaSubtypeGUID("NV12")

	mfMTMajorType               = winGUID{0x48eba18e, 0xf8c9, 0x4687, [8]byte{0xbf, 0x11, 0x0a, 0x74, 0xc9, 0xf9, 0x6a, 0x8f}}
	mfMTSubtype                 = winGUID{0xf7e34c9a, 0x42e8, 0x4714, [8]byte{0xb7, 0x4b, 0xcb, 0x29, 0xd7, 0x2c, 0x35, 0xe5}}
	mfMTFrameSize               = winGUID{0x1652c33d, 0xd6b2, 0x4012, [8]byte{0xb8, 0x34, 0x72, 0x03, 0x08, 0x49, 0xa3, 0x7d}}
	mfMTFrameRate               = winGUID{0xc459a2e8, 0x3d2c, 0x4e44, [8]byte{0xb1, 0x32, 0xfe, 0xe5, 0x15, 0x6c, 0x7b, 0xb0}}
	mfMTPixelAspectRatio        = winGUID{0xc6376a1e, 0x8d0a, 0x4027, [8]byte{0xbe, 0x45, 0x6d, 0x9a, 0x0a, 0xd3, 0x9b, 0xb6}}
	mfMTInterlaceMode           = winGUID{0xe2724bb8, 0xe676, 0x4806, [8]byte{0xb4, 0xb2, 0xa8, 0xd6, 0xef, 0xb4, 0x4c, 0xcd}}
	mfMTAvgBitrate              = winGUID{0x20332624, 0xfb0d, 0x4d9e, [8]byte{0xbd, 0x0d, 0xcb, 0xf6, 0x78, 0x6c, 0x10, 0x2e}}
	mfMTMaxKeyframeSpacing      = winGUID{0xc16eb52b, 0x73a1, 0x476f, [8]byte{0x8d, 0x62, 0x83, 0x9d, 0x6a, 0x02, 0x06, 0x52}}
	mfMTMPEG2Profile            = winGUID{0xad76a80b, 0x2d5c, 0x4e0b, [8]byte{0xb3, 0x75, 0x64, 0xe5, 0x20, 0x13, 0x70, 0x36}}
	mfMTMPEGSequenceHeader      = winGUID{0x3c036de7, 0x3ad0, 0x4c9e, [8]byte{0x92, 0x16, 0xee, 0x6d, 0x6a, 0xc2, 0x1c, 0xb3}}
	mfMTH264MaxCodecConfigDelay = winGUID{0xf5929986, 0x4c45, 0x4fbb, [8]byte{0xbb, 0x49, 0x6c, 0xc5, 0x34, 0xd0, 0x5b, 0x9b}}
)

type H264EncoderWindowsMediaFoundation struct {
	mu sync.Mutex

	width        int
	height       int
	fps          int
	bitrateKbps  int
	keyInterval  int
	sampleTime   int64
	samplePeriod int64
	requestCh    chan windowsMFEncodeRequest
}

type windowsMFEncoderConfig struct {
	width       int
	height      int
	fps         int
	bitrateKbps int
	keyInterval int
}

type windowsMFEncodeRequest struct {
	frame          []byte
	sampleTime     int64
	sampleDuration int64
	forceKeyFrame  bool
	response       chan windowsMFEncodeResponse
	stop           chan error
}

type windowsMFEncodeResponse struct {
	data       []byte
	isKeyFrame bool
	sps        []byte
	pps        []byte
	err        error
}

type windowsMFSession struct {
	config windowsMFEncoderConfig

	transform        *imfTransform
	outputStreamInfo mftOutputStreamInfo
	cachedSPS        []byte
	cachedPPS        []byte
}

type hresultError struct {
	op string
	hr uint32
}

func (e *hresultError) Error() string {
	return fmt.Sprintf("%s 失败: %s", e.op, formatHRESULT(e.hr))
}

type winGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type comObject struct {
	lpVtbl *comVTable
}

type comVTable struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
}

type mftOutputStreamInfo struct {
	dwFlags     uint32
	cbSize      uint32
	cbAlignment uint32
}

type mftOutputDataBuffer struct {
	dwStreamID uint32
	pSample    *imfSample
	dwStatus   uint32
	pEvents    *comObject
}

type imfAttributes struct {
	lpVtbl *imfAttributesVtbl
}

type imfAttributesVtbl struct {
	QueryInterface     uintptr
	AddRef             uintptr
	Release            uintptr
	GetItem            uintptr
	GetItemType        uintptr
	CompareItem        uintptr
	Compare            uintptr
	GetUINT32          uintptr
	GetUINT64          uintptr
	GetDouble          uintptr
	GetGUID            uintptr
	GetStringLength    uintptr
	GetString          uintptr
	GetAllocatedString uintptr
	GetBlobSize        uintptr
	GetBlob            uintptr
	GetAllocatedBlob   uintptr
	GetUnknown         uintptr
	SetItem            uintptr
	DeleteItem         uintptr
	DeleteAllItems     uintptr
	SetUINT32          uintptr
	SetUINT64          uintptr
	SetDouble          uintptr
	SetGUID            uintptr
	SetString          uintptr
	SetBlob            uintptr
	SetUnknown         uintptr
	LockStore          uintptr
	UnlockStore        uintptr
	GetCount           uintptr
	GetItemByIndex     uintptr
	CopyAllItems       uintptr
}

type imfMediaType struct {
	lpVtbl *imfMediaTypeVtbl
}

type imfMediaTypeVtbl struct {
	imfAttributesVtbl
	GetMajorType       uintptr
	IsCompressedFormat uintptr
	IsEqual            uintptr
	GetRepresentation  uintptr
	FreeRepresentation uintptr
}

type imfSample struct {
	lpVtbl *imfSampleVtbl
}

type imfSampleVtbl struct {
	imfAttributesVtbl
	GetSampleFlags            uintptr
	SetSampleFlags            uintptr
	GetSampleTime             uintptr
	SetSampleTime             uintptr
	GetSampleDuration         uintptr
	SetSampleDuration         uintptr
	GetBufferCount            uintptr
	GetBufferByIndex          uintptr
	ConvertToContiguousBuffer uintptr
	AddBuffer                 uintptr
	RemoveBufferByIndex       uintptr
	RemoveAllBuffers          uintptr
	GetTotalLength            uintptr
	CopyToBuffer              uintptr
}

type imfMediaBuffer struct {
	lpVtbl *imfMediaBufferVtbl
}

type imfMediaBufferVtbl struct {
	QueryInterface   uintptr
	AddRef           uintptr
	Release          uintptr
	Lock             uintptr
	Unlock           uintptr
	GetCurrentLength uintptr
	SetCurrentLength uintptr
	GetMaxLength     uintptr
}

type imfTransform struct {
	lpVtbl *imfTransformVtbl
}

type imfTransformVtbl struct {
	QueryInterface            uintptr
	AddRef                    uintptr
	Release                   uintptr
	GetStreamLimits           uintptr
	GetStreamCount            uintptr
	GetStreamIDs              uintptr
	GetInputStreamInfo        uintptr
	GetOutputStreamInfo       uintptr
	GetAttributes             uintptr
	GetInputStreamAttributes  uintptr
	GetOutputStreamAttributes uintptr
	DeleteInputStream         uintptr
	AddInputStreams           uintptr
	GetInputAvailableType     uintptr
	GetOutputAvailableType    uintptr
	SetInputType              uintptr
	SetOutputType             uintptr
	GetInputCurrentType       uintptr
	GetOutputCurrentType      uintptr
	GetInputStatus            uintptr
	GetOutputStatus           uintptr
	SetOutputBounds           uintptr
	ProcessEvent              uintptr
	ProcessMessage            uintptr
	ProcessInput              uintptr
	ProcessOutput             uintptr
}

func newWindowsMediaFoundationH264Encoder() *H264EncoderWindowsMediaFoundation {
	cfg := DefaultH264Config()
	return &H264EncoderWindowsMediaFoundation{keyInterval: cfg.KeyInterval}
}

func (e *H264EncoderWindowsMediaFoundation) Initialize(width, height int, fps int, bitrateKbps int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("无效的编码尺寸: %dx%d", width, height)
	}
	if width%2 != 0 || height%2 != 0 {
		return fmt.Errorf("Windows Media Foundation H.264 目前要求偶数分辨率，当前为 %dx%d", width, height)
	}
	if fps <= 0 {
		fps = 15
	}
	if bitrateKbps <= 0 {
		bitrateKbps = DefaultH264Config().Bitrate
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.keyInterval <= 0 {
		e.keyInterval = DefaultH264Config().KeyInterval
	}
	if err := e.closeLocked(); err != nil {
		return err
	}

	cfg := windowsMFEncoderConfig{
		width:       width,
		height:      height,
		fps:         fps,
		bitrateKbps: bitrateKbps,
		keyInterval: e.keyInterval,
	}
	requestCh := make(chan windowsMFEncodeRequest, 1)
	initErrCh := make(chan error, 1)
	go runWindowsMediaFoundationWorker(requestCh, initErrCh, cfg)

	if err := <-initErrCh; err != nil {
		return err
	}

	e.width = width
	e.height = height
	e.fps = fps
	e.bitrateKbps = bitrateKbps
	e.sampleTime = 0
	e.samplePeriod = 10_000_000 / int64(fps)
	if e.samplePeriod <= 0 {
		e.samplePeriod = 10_000_000 / 15
	}
	e.requestCh = requestCh
	return nil
}

func (e *H264EncoderWindowsMediaFoundation) Encode(img image.Image, forceKeyFrame bool) ([]byte, bool, []byte, []byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.requestCh == nil {
		return nil, false, nil, nil, fmt.Errorf("Windows Media Foundation H.264 编码器尚未初始化")
	}

	bounds := img.Bounds()
	if bounds.Dx() != e.width || bounds.Dy() != e.height {
		return nil, false, nil, nil, fmt.Errorf("屏幕分辨率已变化: got %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), e.width, e.height)
	}

	frameData, err := imageToNV12Buffer(img)
	if err != nil {
		return nil, false, nil, nil, fmt.Errorf("转换 NV12 帧失败: %w", err)
	}

	responseCh := make(chan windowsMFEncodeResponse, 1)
	request := windowsMFEncodeRequest{
		frame:          frameData,
		sampleTime:     e.sampleTime,
		sampleDuration: e.samplePeriod,
		forceKeyFrame:  forceKeyFrame,
		response:       responseCh,
	}
	e.sampleTime += e.samplePeriod
	select {
	case e.requestCh <- request:
	case <-time.After(5 * time.Second):
		return nil, false, nil, nil, fmt.Errorf("提交 Windows Media Foundation 编码请求超时")
	}

	select {
	case response := <-responseCh:
		return response.data, response.isKeyFrame, response.sps, response.pps, response.err
	case <-time.After(5 * time.Second):
		return nil, false, nil, nil, fmt.Errorf("等待 Windows Media Foundation H.264 输出超时")
	}
}

func (e *H264EncoderWindowsMediaFoundation) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.closeLocked()
}

func (e *H264EncoderWindowsMediaFoundation) closeLocked() error {
	if e.requestCh == nil {
		return nil
	}

	stopCh := make(chan error, 1)
	select {
	case e.requestCh <- windowsMFEncodeRequest{stop: stopCh}:
	case <-time.After(5 * time.Second):
		return fmt.Errorf("提交 Windows Media Foundation 关闭请求超时")
	}
	e.requestCh = nil
	e.sampleTime = 0

	select {
	case err := <-stopCh:
		return err
	case <-time.After(5 * time.Second):
		return fmt.Errorf("关闭 Windows Media Foundation H.264 编码器超时")
	}
}

func (e *H264EncoderWindowsMediaFoundation) IsHardwareAccelerated() bool {
	return false
}

func runWindowsMediaFoundationWorker(requestCh <-chan windowsMFEncodeRequest, initErrCh chan<- error, cfg windowsMFEncoderConfig) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := coInitializeMultithreaded(); err != nil {
		initErrCh <- fmt.Errorf("初始化 Windows COM 失败: %w", err)
		return
	}
	defer coUninitialize()

	if err := mediaFoundationStartup(); err != nil {
		initErrCh <- fmt.Errorf("初始化 Windows Media Foundation 失败: %w", err)
		return
	}
	defer func() {
		_ = mediaFoundationShutdown()
	}()

	session, err := newWindowsMFSession(cfg)
	if err != nil {
		initErrCh <- err
		return
	}
	defer func() {
		_ = session.close()
	}()

	initErrCh <- nil

	for {
		request := <-requestCh
		if request.stop != nil {
			request.stop <- session.close()
			close(request.stop)
			return
		}

		if request.forceKeyFrame {
			restartedSession, restartErr := newWindowsMFSession(cfg)
			if restartErr != nil {
				request.response <- windowsMFEncodeResponse{err: restartErr}
				continue
			}
			_ = session.close()
			session = restartedSession
		}

		data, isKeyFrame, sps, pps, err := session.encodeFrame(request.frame, request.sampleTime, request.sampleDuration, request.forceKeyFrame)
		request.response <- windowsMFEncodeResponse{
			data:       data,
			isKeyFrame: isKeyFrame,
			sps:        sps,
			pps:        pps,
			err:        err,
		}
	}
}

func newWindowsMFSession(cfg windowsMFEncoderConfig) (*windowsMFSession, error) {
	transform, err := createWindowsH264Transform()
	if err != nil {
		return nil, err
	}

	session := &windowsMFSession{
		config:    cfg,
		transform: transform,
	}
	if err := session.configureMediaTypes(); err != nil {
		_ = session.close()
		return nil, err
	}
	if err := session.transform.ProcessMessage(mftMessageNotifyBeginStreaming, 0); err != nil {
		_ = session.close()
		return nil, fmt.Errorf("通知 H.264 编码器开始流式处理失败: %w", err)
	}
	if err := session.transform.ProcessMessage(mftMessageNotifyStartOfStream, 0); err != nil {
		_ = session.close()
		return nil, fmt.Errorf("通知 H.264 编码器开始输出失败: %w", err)
	}

	return session, nil
}

func (s *windowsMFSession) configureMediaTypes() error {
	outputType, err := mfCreateMediaType()
	if err != nil {
		return fmt.Errorf("创建 H.264 输出类型失败: %w", err)
	}
	defer releaseCOM(unsafe.Pointer(outputType))

	if err := outputType.SetGUID(&mfMTMajorType, &mfMediaTypeVideo); err != nil {
		return fmt.Errorf("设置 H.264 输出主类型失败: %w", err)
	}
	if err := outputType.SetGUID(&mfMTSubtype, &mfVideoFormatH264); err != nil {
		return fmt.Errorf("设置 H.264 输出子类型失败: %w", err)
	}
	if err := mfSetAttributeSize((*imfAttributes)(unsafe.Pointer(outputType)), &mfMTFrameSize, uint32(s.config.width), uint32(s.config.height)); err != nil {
		return fmt.Errorf("设置 H.264 输出分辨率失败: %w", err)
	}
	if err := mfSetAttributeRatio((*imfAttributes)(unsafe.Pointer(outputType)), &mfMTFrameRate, uint32(s.config.fps), 1); err != nil {
		return fmt.Errorf("设置 H.264 输出帧率失败: %w", err)
	}
	if err := mfSetAttributeRatio((*imfAttributes)(unsafe.Pointer(outputType)), &mfMTPixelAspectRatio, 1, 1); err != nil {
		return fmt.Errorf("设置 H.264 输出像素宽高比失败: %w", err)
	}
	if err := outputType.SetUINT32(&mfMTInterlaceMode, mfVideoInterlaceProgressive); err != nil {
		return fmt.Errorf("设置 H.264 输出扫描方式失败: %w", err)
	}
	if err := outputType.SetUINT32(&mfMTAvgBitrate, uint32(s.config.bitrateKbps*1000)); err != nil {
		return fmt.Errorf("设置 H.264 输出码率失败: %w", err)
	}
	if err := outputType.SetUINT32(&mfMTMaxKeyframeSpacing, uint32(s.config.keyInterval)); err != nil {
		return fmt.Errorf("设置 H.264 关键帧间隔失败: %w", err)
	}
	if err := outputType.SetUINT32(&mfMTMPEG2Profile, avEncH264ProfileBase); err != nil {
		return fmt.Errorf("设置 H.264 profile 失败: %w", err)
	}
	if err := outputType.SetUINT32(&mfMTH264MaxCodecConfigDelay, 0); err != nil {
		return fmt.Errorf("设置 H.264 配置延迟失败: %w", err)
	}
	if err := s.transform.SetOutputType(0, outputType, 0); err != nil {
		return fmt.Errorf("应用 H.264 输出类型失败: %w", err)
	}

	inputType, err := mfCreateMediaType()
	if err != nil {
		return fmt.Errorf("创建 NV12 输入类型失败: %w", err)
	}
	defer releaseCOM(unsafe.Pointer(inputType))

	if err := inputType.SetGUID(&mfMTMajorType, &mfMediaTypeVideo); err != nil {
		return fmt.Errorf("设置 NV12 输入主类型失败: %w", err)
	}
	if err := inputType.SetGUID(&mfMTSubtype, &mfVideoFormatNV12); err != nil {
		return fmt.Errorf("设置 NV12 输入子类型失败: %w", err)
	}
	if err := mfSetAttributeSize((*imfAttributes)(unsafe.Pointer(inputType)), &mfMTFrameSize, uint32(s.config.width), uint32(s.config.height)); err != nil {
		return fmt.Errorf("设置 NV12 输入分辨率失败: %w", err)
	}
	if err := mfSetAttributeRatio((*imfAttributes)(unsafe.Pointer(inputType)), &mfMTFrameRate, uint32(s.config.fps), 1); err != nil {
		return fmt.Errorf("设置 NV12 输入帧率失败: %w", err)
	}
	if err := mfSetAttributeRatio((*imfAttributes)(unsafe.Pointer(inputType)), &mfMTPixelAspectRatio, 1, 1); err != nil {
		return fmt.Errorf("设置 NV12 输入像素宽高比失败: %w", err)
	}
	if err := inputType.SetUINT32(&mfMTInterlaceMode, mfVideoInterlaceProgressive); err != nil {
		return fmt.Errorf("设置 NV12 输入扫描方式失败: %w", err)
	}
	if err := s.transform.SetInputType(0, inputType, 0); err != nil {
		return fmt.Errorf("应用 NV12 输入类型失败: %w", err)
	}

	if err := s.refreshOutputState(); err != nil && !hasHRESULT(err, hresultMFEAttributeNotFound) {
		return fmt.Errorf("读取 H.264 输出状态失败: %w", err)
	}

	return nil
}

func (s *windowsMFSession) encodeFrame(frame []byte, sampleTime int64, sampleDuration int64, hintedKeyFrame bool) ([]byte, bool, []byte, []byte, error) {
	inputSample, err := newInputSample(frame, sampleTime, sampleDuration)
	if err != nil {
		return nil, false, nil, nil, fmt.Errorf("创建输入样本失败: %w", err)
	}
	defer releaseCOM(unsafe.Pointer(inputSample))

	if err := s.transform.ProcessInput(0, inputSample, 0); err != nil {
		return nil, false, nil, nil, fmt.Errorf("提交 NV12 帧到 Windows H.264 编码器失败: %w", err)
	}

	rawOutput, err := s.collectOutput()
	if err != nil {
		return nil, false, nil, nil, err
	}

	payload, isKeyFrame, sps, pps, err := extractH264Packet(rawOutput, hintedKeyFrame)
	if err != nil {
		return nil, false, nil, nil, fmt.Errorf("解析 Windows H.264 输出失败: %w", err)
	}

	if len(sps) > 0 {
		s.cachedSPS = append([]byte(nil), sps...)
	}
	if len(pps) > 0 {
		s.cachedPPS = append([]byte(nil), pps...)
	}
	if len(s.cachedSPS) == 0 || len(s.cachedPPS) == 0 {
		if err := s.refreshOutputState(); err != nil && !hasHRESULT(err, hresultMFEAttributeNotFound) {
			return nil, false, nil, nil, fmt.Errorf("刷新 Windows H.264 序列头失败: %w", err)
		}
	}
	if len(sps) == 0 && len(s.cachedSPS) > 0 {
		sps = append([]byte(nil), s.cachedSPS...)
	}
	if len(pps) == 0 && len(s.cachedPPS) > 0 {
		pps = append([]byte(nil), s.cachedPPS...)
	}

	return payload, isKeyFrame, sps, pps, nil
}

func (s *windowsMFSession) collectOutput() ([]byte, error) {
	var combined bytes.Buffer

	for {
		providedSample, err := s.newOutputSample()
		if err != nil {
			return nil, fmt.Errorf("创建输出样本失败: %w", err)
		}

		outputData := mftOutputDataBuffer{
			dwStreamID: 0,
			pSample:    providedSample,
		}
		var status uint32
		err = s.transform.ProcessOutput(0, 1, &outputData, &status)
		if err != nil {
			if hasHRESULT(err, hresultMFETransformChange) {
				releaseOutputData(providedSample, &outputData)
				if refreshErr := s.refreshOutputState(); refreshErr != nil {
					return nil, fmt.Errorf("Windows H.264 输出格式发生变化，但刷新失败: %w", refreshErr)
				}
				continue
			}
			if hasHRESULT(err, hresultMFETransformNeedInput) {
				releaseOutputData(providedSample, &outputData)
				if combined.Len() > 0 {
					break
				}
				return nil, fmt.Errorf("%w: Windows H.264 编码器尚未返回完整帧 (%v)", errH264EncoderNeedsMoreInput, err)
			}
			releaseOutputData(providedSample, &outputData)
			return nil, fmt.Errorf("Windows H.264 ProcessOutput 失败: %w", err)
		}

		if outputData.dwStatus&mftOutputDataBufferFormatChange != 0 {
			if refreshErr := s.refreshOutputState(); refreshErr != nil {
				releaseOutputData(providedSample, &outputData)
				return nil, fmt.Errorf("Windows H.264 输出格式刷新失败: %w", refreshErr)
			}
		}

		sample := outputData.pSample
		if sample == nil {
			releaseOutputData(providedSample, &outputData)
			if combined.Len() > 0 {
				break
			}
			return nil, fmt.Errorf("Windows H.264 编码器返回了空输出样本")
		}

		chunk, readErr := readSampleBytes(sample)
		releaseOutputData(providedSample, &outputData)
		if readErr != nil {
			return nil, fmt.Errorf("读取 Windows H.264 输出样本失败: %w", readErr)
		}
		if len(chunk) > 0 {
			combined.Write(chunk)
		}

		if outputData.dwStatus&mftOutputDataBufferIncomplete == 0 {
			break
		}
	}

	if combined.Len() == 0 {
		return nil, fmt.Errorf("Windows H.264 编码器未产生任何输出")
	}
	return combined.Bytes(), nil
}

func (s *windowsMFSession) newOutputSample() (*imfSample, error) {
	if s.outputStreamInfo.dwFlags&(mftOutputStreamProvidesSamples|mftOutputStreamCanProvideSample) != 0 {
		return nil, nil
	}

	sample, err := mfCreateSample()
	if err != nil {
		return nil, err
	}

	bufferSize := s.outputStreamInfo.cbSize
	if bufferSize == 0 {
		bufferSize = uint32(s.config.width * s.config.height * 4)
	}
	if s.outputStreamInfo.cbAlignment > 0 {
		bufferSize = (bufferSize + s.outputStreamInfo.cbAlignment) &^ s.outputStreamInfo.cbAlignment
	}
	if bufferSize == 0 {
		bufferSize = 1
	}

	buffer, err := mfCreateMemoryBuffer(bufferSize)
	if err != nil {
		releaseCOM(unsafe.Pointer(sample))
		return nil, err
	}
	defer releaseCOM(unsafe.Pointer(buffer))

	if err := sample.AddBuffer(buffer); err != nil {
		releaseCOM(unsafe.Pointer(sample))
		return nil, err
	}

	return sample, nil
}

func (s *windowsMFSession) refreshOutputState() error {
	if err := s.transform.GetOutputStreamInfo(0, &s.outputStreamInfo); err != nil {
		return err
	}

	outputType, err := s.transform.GetOutputCurrentType(0)
	if err != nil {
		return err
	}
	defer releaseCOM(unsafe.Pointer(outputType))

	sequenceHeader, err := outputType.GetBlob(&mfMTMPEGSequenceHeader)
	if err != nil {
		return err
	}

	sps, pps := extractH264ParameterSets(sequenceHeader)
	if len(sps) > 0 {
		s.cachedSPS = append([]byte(nil), sps...)
	}
	if len(pps) > 0 {
		s.cachedPPS = append([]byte(nil), pps...)
	}
	return nil
}

func (s *windowsMFSession) close() error {
	if s.transform != nil {
		releaseCOM(unsafe.Pointer(s.transform))
		s.transform = nil
	}
	return nil
}

func newInputSample(frame []byte, sampleTime int64, sampleDuration int64) (*imfSample, error) {
	sample, err := mfCreateSample()
	if err != nil {
		return nil, err
	}

	buffer, err := mfCreateMemoryBuffer(uint32(len(frame)))
	if err != nil {
		releaseCOM(unsafe.Pointer(sample))
		return nil, err
	}
	defer releaseCOM(unsafe.Pointer(buffer))

	locked, unlock, err := lockMediaBufferForWrite(buffer)
	if err != nil {
		releaseCOM(unsafe.Pointer(sample))
		return nil, err
	}
	copy(locked, frame)
	unlock()

	if err := buffer.SetCurrentLength(uint32(len(frame))); err != nil {
		releaseCOM(unsafe.Pointer(sample))
		return nil, err
	}
	if err := sample.AddBuffer(buffer); err != nil {
		releaseCOM(unsafe.Pointer(sample))
		return nil, err
	}
	if err := sample.SetSampleTime(sampleTime); err != nil {
		releaseCOM(unsafe.Pointer(sample))
		return nil, err
	}
	if err := sample.SetSampleDuration(sampleDuration); err != nil {
		releaseCOM(unsafe.Pointer(sample))
		return nil, err
	}

	return sample, nil
}

func readSampleBytes(sample *imfSample) ([]byte, error) {
	buffer, err := sample.ConvertToContiguousBuffer()
	if err != nil {
		return nil, err
	}
	defer releaseCOM(unsafe.Pointer(buffer))

	locked, unlock, err := lockMediaBufferForRead(buffer)
	if err != nil {
		return nil, err
	}
	defer unlock()

	return append([]byte(nil), locked...), nil
}

func lockMediaBufferForWrite(buffer *imfMediaBuffer) ([]byte, func(), error) {
	var raw *byte
	var maxLength uint32
	var currentLength uint32
	if err := buffer.Lock(&raw, &maxLength, &currentLength); err != nil {
		return nil, nil, err
	}

	unlock := func() {
		_ = buffer.Unlock()
	}
	if maxLength == 0 {
		return nil, unlock, nil
	}

	return unsafe.Slice(raw, maxLength), unlock, nil
}

func lockMediaBufferForRead(buffer *imfMediaBuffer) ([]byte, func(), error) {
	var raw *byte
	var maxLength uint32
	var currentLength uint32
	if err := buffer.Lock(&raw, &maxLength, &currentLength); err != nil {
		return nil, nil, err
	}

	unlock := func() {
		_ = buffer.Unlock()
	}
	if currentLength == 0 {
		return nil, unlock, nil
	}

	return unsafe.Slice(raw, currentLength), unlock, nil
}

func createWindowsH264Transform() (*imfTransform, error) {
	var transform *imfTransform
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidCMSH264EncoderMFT)),
		0,
		uintptr(clsctxInprocServer),
		uintptr(unsafe.Pointer(&iidIMFTransform)),
		uintptr(unsafe.Pointer(&transform)),
	)
	if err := checkHRESULT("CoCreateInstance(CMSH264EncoderMFT)", hr); err != nil {
		return nil, err
	}
	return transform, nil
}

func coInitializeMultithreaded() error {
	hr, _, _ := procCoInitializeEx.Call(0, uintptr(coinitMultithreaded))
	return checkHRESULT("CoInitializeEx", hr)
}

func coUninitialize() {
	_, _, _ = procCoUninitialize.Call()
}

func mediaFoundationStartup() error {
	hr, _, _ := procMFStartup.Call(uintptr(mfVersion), uintptr(mfStartupFull))
	return checkHRESULT("MFStartup", hr)
}

func mediaFoundationShutdown() error {
	hr, _, _ := procMFShutdown.Call()
	return checkHRESULT("MFShutdown", hr)
}

func mfCreateMediaType() (*imfMediaType, error) {
	var mediaType *imfMediaType
	hr, _, _ := procMFCreateMediaType.Call(uintptr(unsafe.Pointer(&mediaType)))
	if err := checkHRESULT("MFCreateMediaType", hr); err != nil {
		return nil, err
	}
	return mediaType, nil
}

func mfCreateSample() (*imfSample, error) {
	var sample *imfSample
	hr, _, _ := procMFCreateSample.Call(uintptr(unsafe.Pointer(&sample)))
	if err := checkHRESULT("MFCreateSample", hr); err != nil {
		return nil, err
	}
	return sample, nil
}

func mfCreateMemoryBuffer(size uint32) (*imfMediaBuffer, error) {
	var buffer *imfMediaBuffer
	hr, _, _ := procMFCreateMemoryBuffer.Call(uintptr(size), uintptr(unsafe.Pointer(&buffer)))
	if err := checkHRESULT("MFCreateMemoryBuffer", hr); err != nil {
		return nil, err
	}
	return buffer, nil
}

func mfSetAttributeSize(attributes *imfAttributes, key *winGUID, width uint32, height uint32) error {
	return attributes.SetUINT64(key, packTwoUINT32(width, height))
}

func mfSetAttributeRatio(attributes *imfAttributes, key *winGUID, numerator uint32, denominator uint32) error {
	return attributes.SetUINT64(key, packTwoUINT32(numerator, denominator))
}

func packTwoUINT32(high uint32, low uint32) uint64 {
	return (uint64(high) << 32) | uint64(low)
}

func (a *imfAttributes) SetGUID(key *winGUID, value *winGUID) error {
	hr, _, _ := syscall.SyscallN(
		a.lpVtbl.SetGUID,
		uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(key)),
		uintptr(unsafe.Pointer(value)),
	)
	return checkHRESULT("IMFAttributes::SetGUID", hr)
}

func (a *imfAttributes) SetUINT32(key *winGUID, value uint32) error {
	hr, _, _ := syscall.SyscallN(
		a.lpVtbl.SetUINT32,
		uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(key)),
		uintptr(value),
	)
	return checkHRESULT("IMFAttributes::SetUINT32", hr)
}

func (a *imfAttributes) SetUINT64(key *winGUID, value uint64) error {
	hr, _, _ := syscall.SyscallN(
		a.lpVtbl.SetUINT64,
		uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(key)),
		uintptr(value),
	)
	return checkHRESULT("IMFAttributes::SetUINT64", hr)
}

func (a *imfAttributes) GetBlobSize(key *winGUID) (uint32, error) {
	var size uint32
	hr, _, _ := syscall.SyscallN(
		a.lpVtbl.GetBlobSize,
		uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(key)),
		uintptr(unsafe.Pointer(&size)),
	)
	if err := checkHRESULT("IMFAttributes::GetBlobSize", hr); err != nil {
		return 0, err
	}
	return size, nil
}

func (a *imfAttributes) GetBlob(key *winGUID, buffer []byte) error {
	var copied uint32
	var bufferPtr uintptr
	if len(buffer) > 0 {
		bufferPtr = uintptr(unsafe.Pointer(&buffer[0]))
	}
	hr, _, _ := syscall.SyscallN(
		a.lpVtbl.GetBlob,
		uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(key)),
		bufferPtr,
		uintptr(uint32(len(buffer))),
		uintptr(unsafe.Pointer(&copied)),
	)
	if err := checkHRESULT("IMFAttributes::GetBlob", hr); err != nil {
		return err
	}
	if copied != uint32(len(buffer)) {
		return fmt.Errorf("IMFAttributes::GetBlob 返回了 %d 字节，预期 %d 字节", copied, len(buffer))
	}
	return nil
}

func (m *imfMediaType) attrs() *imfAttributes {
	return (*imfAttributes)(unsafe.Pointer(m))
}

func (m *imfMediaType) SetGUID(key *winGUID, value *winGUID) error {
	return m.attrs().SetGUID(key, value)
}

func (m *imfMediaType) SetUINT32(key *winGUID, value uint32) error {
	return m.attrs().SetUINT32(key, value)
}

func (m *imfMediaType) GetBlob(key *winGUID) ([]byte, error) {
	size, err := m.attrs().GetBlobSize(key)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, nil
	}

	buffer := make([]byte, size)
	if err := m.attrs().GetBlob(key, buffer); err != nil {
		return nil, err
	}
	return buffer, nil
}

func (s *imfSample) SetSampleTime(value int64) error {
	hr, _, _ := syscall.SyscallN(
		s.lpVtbl.SetSampleTime,
		uintptr(unsafe.Pointer(s)),
		uintptr(value),
	)
	return checkHRESULT("IMFSample::SetSampleTime", hr)
}

func (s *imfSample) SetSampleDuration(value int64) error {
	hr, _, _ := syscall.SyscallN(
		s.lpVtbl.SetSampleDuration,
		uintptr(unsafe.Pointer(s)),
		uintptr(value),
	)
	return checkHRESULT("IMFSample::SetSampleDuration", hr)
}

func (s *imfSample) AddBuffer(buffer *imfMediaBuffer) error {
	hr, _, _ := syscall.SyscallN(
		s.lpVtbl.AddBuffer,
		uintptr(unsafe.Pointer(s)),
		uintptr(unsafe.Pointer(buffer)),
	)
	return checkHRESULT("IMFSample::AddBuffer", hr)
}

func (s *imfSample) ConvertToContiguousBuffer() (*imfMediaBuffer, error) {
	var buffer *imfMediaBuffer
	hr, _, _ := syscall.SyscallN(
		s.lpVtbl.ConvertToContiguousBuffer,
		uintptr(unsafe.Pointer(s)),
		uintptr(unsafe.Pointer(&buffer)),
	)
	if err := checkHRESULT("IMFSample::ConvertToContiguousBuffer", hr); err != nil {
		return nil, err
	}
	return buffer, nil
}

func (b *imfMediaBuffer) Lock(buffer **byte, maxLength *uint32, currentLength *uint32) error {
	hr, _, _ := syscall.SyscallN(
		b.lpVtbl.Lock,
		uintptr(unsafe.Pointer(b)),
		uintptr(unsafe.Pointer(buffer)),
		uintptr(unsafe.Pointer(maxLength)),
		uintptr(unsafe.Pointer(currentLength)),
	)
	return checkHRESULT("IMFMediaBuffer::Lock", hr)
}

func (b *imfMediaBuffer) Unlock() error {
	hr, _, _ := syscall.SyscallN(
		b.lpVtbl.Unlock,
		uintptr(unsafe.Pointer(b)),
	)
	return checkHRESULT("IMFMediaBuffer::Unlock", hr)
}

func (b *imfMediaBuffer) SetCurrentLength(value uint32) error {
	hr, _, _ := syscall.SyscallN(
		b.lpVtbl.SetCurrentLength,
		uintptr(unsafe.Pointer(b)),
		uintptr(value),
	)
	return checkHRESULT("IMFMediaBuffer::SetCurrentLength", hr)
}

func (t *imfTransform) SetInputType(streamID uint32, mediaType *imfMediaType, flags uint32) error {
	hr, _, _ := syscall.SyscallN(
		t.lpVtbl.SetInputType,
		uintptr(unsafe.Pointer(t)),
		uintptr(streamID),
		uintptr(unsafe.Pointer(mediaType)),
		uintptr(flags),
	)
	return checkHRESULT("IMFTransform::SetInputType", hr)
}

func (t *imfTransform) SetOutputType(streamID uint32, mediaType *imfMediaType, flags uint32) error {
	hr, _, _ := syscall.SyscallN(
		t.lpVtbl.SetOutputType,
		uintptr(unsafe.Pointer(t)),
		uintptr(streamID),
		uintptr(unsafe.Pointer(mediaType)),
		uintptr(flags),
	)
	return checkHRESULT("IMFTransform::SetOutputType", hr)
}

func (t *imfTransform) GetOutputCurrentType(streamID uint32) (*imfMediaType, error) {
	var mediaType *imfMediaType
	hr, _, _ := syscall.SyscallN(
		t.lpVtbl.GetOutputCurrentType,
		uintptr(unsafe.Pointer(t)),
		uintptr(streamID),
		uintptr(unsafe.Pointer(&mediaType)),
	)
	if err := checkHRESULT("IMFTransform::GetOutputCurrentType", hr); err != nil {
		return nil, err
	}
	return mediaType, nil
}

func (t *imfTransform) GetOutputStreamInfo(streamID uint32, info *mftOutputStreamInfo) error {
	hr, _, _ := syscall.SyscallN(
		t.lpVtbl.GetOutputStreamInfo,
		uintptr(unsafe.Pointer(t)),
		uintptr(streamID),
		uintptr(unsafe.Pointer(info)),
	)
	return checkHRESULT("IMFTransform::GetOutputStreamInfo", hr)
}

func (t *imfTransform) ProcessMessage(message uint32, param uintptr) error {
	hr, _, _ := syscall.SyscallN(
		t.lpVtbl.ProcessMessage,
		uintptr(unsafe.Pointer(t)),
		uintptr(message),
		param,
	)
	return checkHRESULT("IMFTransform::ProcessMessage", hr)
}

func (t *imfTransform) ProcessInput(streamID uint32, sample *imfSample, flags uint32) error {
	hr, _, _ := syscall.SyscallN(
		t.lpVtbl.ProcessInput,
		uintptr(unsafe.Pointer(t)),
		uintptr(streamID),
		uintptr(unsafe.Pointer(sample)),
		uintptr(flags),
	)
	return checkHRESULT("IMFTransform::ProcessInput", hr)
}

func (t *imfTransform) ProcessOutput(flags uint32, count uint32, outputData *mftOutputDataBuffer, status *uint32) error {
	hr, _, _ := syscall.SyscallN(
		t.lpVtbl.ProcessOutput,
		uintptr(unsafe.Pointer(t)),
		uintptr(flags),
		uintptr(count),
		uintptr(unsafe.Pointer(outputData)),
		uintptr(unsafe.Pointer(status)),
	)
	return checkHRESULT("IMFTransform::ProcessOutput", hr)
}

func releaseOutputData(providedSample *imfSample, outputData *mftOutputDataBuffer) {
	if outputData == nil {
		releaseCOM(unsafe.Pointer(providedSample))
		return
	}
	if outputData.pSample != nil && outputData.pSample != providedSample {
		releaseCOM(unsafe.Pointer(outputData.pSample))
	}
	if outputData.pEvents != nil {
		releaseCOM(unsafe.Pointer(outputData.pEvents))
	}
	releaseCOM(unsafe.Pointer(providedSample))
}

func releaseCOM(pointer unsafe.Pointer) {
	if pointer == nil {
		return
	}
	object := (*comObject)(pointer)
	if object.lpVtbl == nil || object.lpVtbl.Release == 0 {
		return
	}
	_, _, _ = syscall.SyscallN(object.lpVtbl.Release, uintptr(pointer))
}

func makeMediaSubtypeGUID(tag string) winGUID {
	if len(tag) != 4 {
		panic("media subtype must be exactly 4 ASCII bytes")
	}
	data1 := uint32(tag[0]) | uint32(tag[1])<<8 | uint32(tag[2])<<16 | uint32(tag[3])<<24
	return winGUID{Data1: data1, Data2: 0x0000, Data3: 0x0010, Data4: [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
}

func checkHRESULT(op string, hr uintptr) error {
	code := uint32(hr)
	if int32(code) >= 0 {
		return nil
	}
	return &hresultError{op: op, hr: code}
}

func hasHRESULT(err error, code uint32) bool {
	var hrErr *hresultError
	return errors.As(err, &hrErr) && hrErr.hr == code
}

func formatHRESULT(code uint32) string {
	switch code {
	case hresultSFalse:
		return "S_FALSE (0x00000001)"
	case hresultENotImpl:
		return "E_NOTIMPL (0x80004001)"
	case hresultRPCEChangedMode:
		return "RPC_E_CHANGED_MODE (0x80010106)"
	case hresultMFEAttributeNotFound:
		return "MF_E_ATTRIBUTENOTFOUND (0xc00d36e6)"
	case hresultMFENotAccepting:
		return "MF_E_NOTACCEPTING (0xc00d36b5)"
	case hresultMFETransformTypeUnset:
		return "MF_E_TRANSFORM_TYPE_NOT_SET (0xc00d6d60)"
	case hresultMFETransformChange:
		return "MF_E_TRANSFORM_STREAM_CHANGE (0xc00d6d61)"
	case hresultMFETransformNeedInput:
		return "MF_E_TRANSFORM_NEED_MORE_INPUT (0xc00d6d72)"
	default:
		return fmt.Sprintf("HRESULT=0x%08x", code)
	}
}
