package cw

import (
	"context"
	"fmt"
	"math"
	"math/cmplx"
	"sort"
	"time"

	"github.com/mjibson/go-dsp/fft"
)

// SpectrumMonitor 在后台异步运行，使用 Welch 法计算平均功率谱，
// 以高精度、抗噪声的方式提取主频，并持续校正解码器。
type SpectrumMonitor struct {
	// 配置
	cfg *Config // 引用全局配置

	// 配置 (从 cfg 中读取)
	sampleRate     float64
	fftSize        int
	overlap        int
	updateInterval time.Duration

	// 通信
	audioInChan       chan []float32     // 从主线程接收音频数据
	OnFrequencyUpdate func(freq float64) // 回调函数，通知系统更新频率

	// 内部状态
	analyzer   *SpectrumAnalyzer // 复用现有的频谱分析器
	ringBuffer []float64         // 环形缓冲区，存储足够进行 Welch 计算的数据
	ringPos    int               // 当前写入位置
	ctx        context.Context
	cancel     context.CancelFunc

	// 频率平滑状态
	smoothedFreq float64 // 当前平滑后的频率
	hasLock      bool    // 是否已经锁定过一次频率
}

// NewSpectrumMonitor 创建实例
func NewSpectrumMonitor(sampleRate float64, cfg *Config, onUpdate func(float64)) *SpectrumMonitor {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	fftSize := cfg.Monitor.FFTSize
	overlap := fftSize / 2
	numSegments := 4
	bufferSize := fftSize + (numSegments-1)*(fftSize-overlap)

	ctx, cancel := context.WithCancel(context.Background())

	return &SpectrumMonitor{
		cfg:               cfg,
		sampleRate:        sampleRate,
		fftSize:           fftSize,
		overlap:           overlap,
		updateInterval:    cfg.Monitor.UpdateInterval,
		audioInChan:       make(chan []float32, 100),
		OnFrequencyUpdate: onUpdate,
		analyzer:          NewSpectrumAnalyzer(sampleRate, fftSize),
		ringBuffer:        make([]float64, bufferSize),
		ctx:               ctx,
		cancel:            cancel,
		smoothedFreq:      700.0,
	}
}

// Start 启动后台监控 goroutine
func (sm *SpectrumMonitor) Start() {
	if sm.cfg.Monitor.Enabled {
		go sm.run()
	}
}

// Stop 停止监控
func (sm *SpectrumMonitor) Stop() {
	sm.cancel()
}

// PushAudioData 主音频线程调用此方法，将数据推送到监控器
func (sm *SpectrumMonitor) PushAudioData(samples []float32) {
	if !sm.cfg.Monitor.Enabled {
		return
	}
	select {
	case sm.audioInChan <- samples:
		// 数据成功发送
	default:
		// 缓冲区已满，丢弃数据以避免阻塞主线程
	}
}

// run 是后台运行的主循环
func (sm *SpectrumMonitor) run() {
	ticker := time.NewTicker(sm.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sm.ctx.Done():
			return // 收到停止信号
		case samples := <-sm.audioInChan:
			// 将新数据写入环形缓冲区
			for _, s := range samples {
				sm.ringBuffer[sm.ringPos] = float64(s)
				sm.ringPos = (sm.ringPos + 1) % len(sm.ringBuffer)
			}
		case <-ticker.C:
			// 时间到了，执行 Welch 分析
			freq, mag, noiseFloor := sm.calculateWelch()

			// --- 自适应静噪 (Adaptive Squelch) ---
			requiredSNR := sm.cfg.Monitor.RequiredSNR

			if mag > noiseFloor*requiredSNR && mag > 0.001 {
				// --- 频率平滑更新 (Weighted Smoothing) ---
				snr := mag / noiseFloor
				// 计算学习率 alpha
				alpha := sm.cfg.Monitor.AlphaBase + db(snr)/requiredSNR*sm.cfg.Monitor.AlphaGain
				if alpha > sm.cfg.Monitor.AlphaMax {
					alpha = sm.cfg.Monitor.AlphaMax
				}

				// 如果是第一次锁定，直接跳转，不平滑
				if !sm.hasLock {
					sm.smoothedFreq = freq
					sm.hasLock = true
					fmt.Printf("[MONITOR] Initial Lock: %.1f Hz (SNR: %.1f)\n", freq, db(snr))
				} else {
					oldFreq := sm.smoothedFreq
					sm.smoothedFreq = sm.smoothedFreq*(1-alpha) + freq*alpha
					// 只有当频率变化超过一定阈值时才打印，避免刷屏
					if abs(sm.smoothedFreq-oldFreq) > 2.0 {
						fmt.Printf("[MONITOR] Frequency Update: %.1f Hz %.1f Hz -> %.1f Hz (SNR: %.1f)\n", oldFreq, freq, sm.smoothedFreq, db(snr))
					}
				}

				if sm.OnFrequencyUpdate != nil {
					sm.OnFrequencyUpdate(sm.smoothedFreq)
				}
			}
		}
	}
}
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
func db(x float64) float64 {
	return 10 * math.Log10(x)
}

// calculateWelch 执行 Welch 平均周期图法
// 返回: 峰值频率, 峰值功率, 噪声基底功率
func (sm *SpectrumMonitor) calculateWelch() (float64, float64, float64) {
	numSegments := 0
	avgSpectrum := make([]float64, sm.fftSize/2+1)
	step := sm.fftSize - sm.overlap

	// 遍历环形缓冲区，分段计算
	for i := 0; (i + sm.fftSize) <= len(sm.ringBuffer); i += step {
		segment := sm.ringBuffer[i : i+sm.fftSize]

		// 1. 加窗
		windowedSegment := make([]complex128, sm.fftSize)
		for j, v := range segment {
			windowedSegment[j] = complex(v*sm.analyzer.Window[j], 0)
		}

		// 2. FFT
		spectrum := fft.FFT(windowedSegment)

		// 3. 计算功率谱并累加
		for j := 0; j < len(avgSpectrum); j++ {
			power := cmplx.Abs(spectrum[j])
			avgSpectrum[j] += power * power // 使用功率 (幅度的平方)
		}
		numSegments++
	}

	if numSegments == 0 {
		return 0, 0, 0
	}

	// 4. 计算平均功率谱
	for i := range avgSpectrum {
		avgSpectrum[i] /= float64(numSegments)
	}

	// 5. 估算噪声基底 (Noise Floor)
	// 使用中位数 (Median) 来抵抗信号峰值的干扰
	sortedSpectrum := make([]float64, len(avgSpectrum))
	copy(sortedSpectrum, avgSpectrum)
	sort.Float64s(sortedSpectrum)
	noiseFloor := sortedSpectrum[len(sortedSpectrum)/2]

	// --- 保护措施 ---
	// 防止在完美静音时 noiseFloor 为 0，导致除零错误
	if noiseFloor < 1e-9 {
		noiseFloor = 1e-9
	}

	// 6. 在平均谱中寻找峰值
	maxMag := 0.0
	maxIndex := 0
	binWidth := sm.sampleRate / float64(sm.fftSize)

	// 限制搜索范围
	minFreq := sm.cfg.Monitor.MinFrequency
	maxFreq := sm.cfg.Monitor.MaxFrequency

	startIndex := int(minFreq / binWidth)
	endIndex := int(maxFreq / binWidth)

	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex > len(avgSpectrum) {
		endIndex = len(avgSpectrum)
	}

	for i := startIndex; i < endIndex; i++ {
		if avgSpectrum[i] > maxMag {
			maxMag = avgSpectrum[i]
			maxIndex = i
		}
	}

	// 简单的抛物线插值，提高频率精度
	var freq float64
	if maxIndex > 0 && maxIndex < len(avgSpectrum)-1 {
		alpha := avgSpectrum[maxIndex-1]
		beta := avgSpectrum[maxIndex]
		gamma := avgSpectrum[maxIndex+1]
		denom := alpha - 2*beta + gamma
		if denom != 0 {
			p := 0.5 * (alpha - gamma) / denom
			freq = (float64(maxIndex) + p) * binWidth
		} else {
			freq = float64(maxIndex) * binWidth
		}
	} else {
		freq = float64(maxIndex) * binWidth
	}

	return freq, maxMag, noiseFloor
}
