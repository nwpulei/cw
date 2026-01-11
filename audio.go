package cw

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/gen2brain/malgo"
)

// AudioCallback 定义音频数据回调函数类型
type AudioCallback func(samples []float32)

// AudioCapture 管理音频捕获
type AudioCapture struct {
	ctx        *malgo.AllocatedContext
	device     *malgo.Device
	SampleRate int
	Callback   AudioCallback
}

// NewAudioCapture 创建新的音频捕获实例
func NewAudioCapture(sampleRate int, targetDeviceName string, callback AudioCallback) (*AudioCapture, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to init malgo context: %v", err)
	}

	ac := &AudioCapture{
		ctx:        ctx,
		SampleRate: sampleRate,
		Callback:   callback,
	}

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatF32
	deviceConfig.Capture.Channels = 1
	deviceConfig.SampleRate = uint32(sampleRate)
	deviceConfig.Alsa.NoMMap = 1

	if targetDeviceName != "" {
		infos, err := ctx.Devices(malgo.Capture)
		if err == nil {
			for _, info := range infos {
				if strings.Contains(strings.ToLower(info.Name()), strings.ToLower(targetDeviceName)) {
					deviceConfig.Capture.DeviceID = info.ID.Pointer()
					fmt.Printf("Selected Audio Device: %s\n", info.Name())
					break
				}
			}
		}
	}

	onRecvFrames := func(pOutputSample, pInputSamples []byte, framecount uint32) {
		if ac.Callback == nil {
			return
		}
		if len(pInputSamples) == 0 {
			return
		}
		samples := unsafe.Slice((*float32)(unsafe.Pointer(&pInputSamples[0])), int(framecount))
		ac.Callback(samples)
	}

	deviceCallbacks := malgo.DeviceCallbacks{
		Data: onRecvFrames,
	}

	device, err := malgo.InitDevice(ctx.Context, deviceConfig, deviceCallbacks)
	if err != nil {
		_ = ctx.Uninit()
		ctx.Free()
		return nil, fmt.Errorf("failed to init device: %v", err)
	}
	ac.device = device
	
	// 打印实际采样率
	// 注意：malgo.Device 的方法可能因版本而异，这里只打印 SampleRate()
	fmt.Printf("Audio Device Initialized. Rate: %d Hz\n", device.SampleRate())

	return ac, nil
}

// Start 启动音频捕获
func (ac *AudioCapture) Start() error {
	if ac.device == nil {
		return fmt.Errorf("device not initialized")
	}
	return ac.device.Start()
}

// Stop 停止音频捕获并释放资源
func (ac *AudioCapture) Stop() {
	if ac.device != nil {
		ac.device.Uninit()
		ac.device = nil
	}
	if ac.ctx != nil {
		_ = ac.ctx.Uninit()
		ac.ctx.Free()
		ac.ctx = nil
	}
}
