package cw

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// CWSystem 管理整个 CW 解码与控制系统的生命周期
type CWSystem struct {
	// 配置
	cfg             *Config
	SampleRate      int
	AudioDeviceName string
	SerialPort      string
	BaudRate        int

	// 组件
	civClient    *CIVClient
	decoder      CWDecoder // 使用接口
	analyzer     *SpectrumAnalyzer
	audioCapture *AudioCapture
	wavReader    *WavReader
	wavWriter    *WavWriter

	// 状态
	isCalibrated      bool
	calibrationBuffer []float64
	replayFile        string
	recordFile        string

	// 回调
	OnTextDecoded   func(text string) // 当解码出文本时回调
	spectrumMonitor *SpectrumMonitor
}

// NewCWSystem 创建系统实例
func NewCWSystem() *CWSystem {
	return &CWSystem{
		cfg:             DefaultConfig(),
		SampleRate:      48000,
		AudioDeviceName: "USB Audio CODEC",
		SerialPort:      "/dev/tty.SLAB_USBtoUART",
		BaudRate:        115200,
	}
}

// EnableRecording 开启录音
func (s *CWSystem) EnableRecording(filename string) {
	s.recordFile = filename
}

// SetReplayFile 设置回放文件 (设置后将进入回放模式)
func (s *CWSystem) SetReplayFile(filename string) {
	s.replayFile = filename
}

// Start 启动系统
func (s *CWSystem) Start() error {
	fmt.Print("\033[2J\033[H")
	// 1. 初始化组件
	if s.replayFile != "" {
		// 回放模式：从文件读取采样率
		var err error
		s.wavReader, err = NewWavReader(s.replayFile)
		if err != nil {
			return fmt.Errorf("failed to open replay file: %v", err)
		}
		s.SampleRate = s.wavReader.SampleRate
		fmt.Printf("Mode: REPLAY (%s, %dHz)\n", s.replayFile, s.SampleRate)
	} else {
		// 实时模式：尝试连接电台
		s.civClient = NewCIVClient(s.SerialPort, s.BaudRate)
		fmt.Printf("Connecting to radio on %s...\n", s.SerialPort)
		if err := s.civClient.Open(); err != nil {
			log.Printf("Warning: Could not open serial port: %v\n", err)
			s.civClient = nil
		} else {
			fmt.Println("Serial port opened.")
		}
	}

	// 初始化 DSP 组件
	// 使用 ExperimentalDecoder (硬编码阈值版本)
	s.decoder = NewExperimentalDecoder(float64(s.SampleRate), 703)
	s.analyzer = NewSpectrumAnalyzer(float64(s.SampleRate), 4096)

	s.spectrumMonitor = NewSpectrumMonitor(float64(s.SampleRate), s.cfg, s.handleFrequencyUpdate)
	//s.spectrumMonitor.Start()
	// 初始化录音 (仅在实时模式或显式要求时)
	if s.recordFile != "" && s.replayFile == "" {
		var err error
		s.wavWriter, err = NewWavWriter(s.recordFile, s.SampleRate)
		if err != nil {
			return fmt.Errorf("failed to create wav file: %v", err)
		}
		fmt.Printf("Recording audio to %s\n", s.recordFile)
	}

	// 2. 启动音频流
	if s.replayFile != "" {
		go s.runReplayLoop()
	} else {
		if err := s.startAudioCapture(); err != nil {
			return err
		}
	}

	return nil
}

// Stop 停止系统并释放资源
func (s *CWSystem) Stop() {
	if s.audioCapture != nil {
		s.audioCapture.Stop()
	}
	if s.wavWriter != nil {
		fmt.Println("\nSaving recording...")
		s.wavWriter.Close()
		fmt.Println("Recording saved.")
	}
	if s.wavReader != nil {
		s.wavReader.Close()
	}
	if s.civClient != nil {
		s.civClient.Close()
	}
	s.decoder.Stop()
}

// HandleInput 处理用户输入的文本 (发送 CW)
func (s *CWSystem) HandleInput(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	if s.civClient != nil {
		fmt.Printf("\n[TX]: %s\n", strings.ToUpper(text))
		if err := s.civClient.SendText(strings.ToUpper(text)); err != nil {
			log.Printf("Error sending text: %v", err)
		}
	} else {
		fmt.Println("Error: Radio not connected (cannot transmit).")
	}
}

// 内部：处理音频块
func (s *CWSystem) processAudioChunk(samples []float32) {
	// 录音
	if s.wavWriter != nil {
		_ = s.wavWriter.WriteSamples(samples)
	}
	//s.spectrumMonitor.PushAudioData(samples)
	//s.isCalibrated = true
	// 校准或解码
	if !s.isCalibrated {
		s.runCalibration(samples)
	} else {
		s.decoder.ProcessAudioChunk(samples)
	}
}

// 内部：执行校准逻辑
func (s *CWSystem) runCalibration(samples []float32) {
	for _, v := range samples {
		s.calibrationBuffer = append(s.calibrationBuffer, float64(v))
	}

	fftSize := s.analyzer.FFTSize
	// 只有当 buffer 填满时才进行一次分析
	if len(s.calibrationBuffer) >= fftSize {
		// 1. 限制搜索频率范围 (Bandwidth Limiting)
		minFreq := 500.0
		maxFreq := 900.0

		freq, rawMag := s.analyzer.FindDominantFrequency(s.calibrationBuffer, minFreq, maxFreq)

		// 归一化 FFT 幅度
		normalizedMag := rawMag * 2.0 / float64(fftSize)

		// 2. 能量绝对阈值 (Magnitude Threshold)
		const MinSignalStrength = 0.01

		if normalizedMag > MinSignalStrength {
			s.decoder.UpdateTargetFreq(freq)

			// 动态设置阈值：取信号幅度的 30%
			newThreshold := normalizedMag * 0.3

			// 增加一个最小阈值保护
			if newThreshold < 0.01 {
				newThreshold = 0.01
			}

			s.decoder.SetThreshold(newThreshold)

			fmt.Printf("\n[CALIB] LOCKED! Freq: %.1f Hz, Mag: %.4f, Thresh: %.4f\n", freq, normalizedMag, newThreshold)

			s.isCalibrated = true
			s.calibrationBuffer = nil
			fmt.Println("Decoding started. Type text to send.")
			fmt.Print("> ")
		} else {
			// 信号太弱，认为是噪声，继续等待
			fmt.Print(".")
			s.calibrationBuffer = s.calibrationBuffer[:0]
		}
	}
}

// 内部：启动实时音频捕获
func (s *CWSystem) startAudioCapture() error {
	var err error
	s.audioCapture, err = NewAudioCapture(s.SampleRate, s.AudioDeviceName, s.processAudioChunk)
	if err != nil {
		return fmt.Errorf("failed to init audio capture: %v", err)
	}
	return s.audioCapture.Start()
}

// 内部：运行回放循环
func (s *CWSystem) runReplayLoop() {
	chunkSize := 1024
	// 计算 ticker 间隔以模拟实时速度
	interval := time.Second * time.Duration(chunkSize) / time.Duration(s.SampleRate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	fmt.Println("Replay started...")
	for range ticker.C {
		samples, err := s.wavReader.ReadSamples(chunkSize)
		if err != nil {
			fmt.Println("\nEnd of file.")
			os.Exit(0) // 回放结束直接退出程序
		}
		s.processAudioChunk(samples)
	}
}

// 内部：处理频率更新回调
func (s *CWSystem) handleFrequencyUpdate(freq float64) {
	// 这里可以添加平滑逻辑，但为了快速验证，我们先直接更新
	//log.Printf("[MONITOR] Detected dominant frequency: %.1f Hz\n", freq)
	s.decoder.UpdateTargetFreq(freq)
}
