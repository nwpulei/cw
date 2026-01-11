package cw

import (
	"math"
	"math/cmplx"

	"github.com/mjibson/go-dsp/fft"
	"github.com/mjibson/go-dsp/window"
)

// PitchDetectorConfig 配置参数
type PitchDetectorConfig struct {
	SampleRate     float64
	FFTSize        int     // 建议 1024 或 2048
	MinFreq        float64 // 搜索下限，如 300Hz
	MaxFreq        float64 // 搜索上限，如 1200Hz
	SmoothingAlpha float64 // 平滑系数 (0.0-1.0)，越小越平滑，建议 0.1
	MaxJumpHz      float64 // 允许的最大突变频率，超过此值视为干扰，建议 50Hz
	NoiseThreshold float64 // 绝对能量门限，低于此值视为噪音
}

// PitchDetector 频率检测器
type PitchDetector struct {
	config      PitchDetectorConfig
	lastFreq    float64 // 上一次锁定的频率
	hasLock     bool    // 是否已经锁定信号
	windowCache []float64
}

// NewPitchDetector 创建新实例
func NewPitchDetector(cfg PitchDetectorConfig) *PitchDetector {
	return &PitchDetector{
		config:      cfg,
		windowCache: window.Blackman(cfg.FFTSize),
		hasLock:     false,
	}
}

// Reset 重置状态（换台时调用）
func (pd *PitchDetector) Reset() {
	pd.lastFreq = 0
	pd.hasLock = false
}

// Detect 输入音频切片，返回探测到的频率。
// found: 是否找到有效信号
func (pd *PitchDetector) Detect(samples []float64) (freq float64, found bool) {
	if len(samples) < pd.config.FFTSize {
		// 数据不足，无法计算
		return pd.lastFreq, pd.hasLock
	}

	// 1. 预处理与FFT
	fftResult := pd.computeFFT(samples)

	// 2. 寻找峰值
	peakFreq, peakMag := pd.findPeak(fftResult)

	// 3. 更新状态并返回结果
	return pd.updateFrequencyState(peakFreq, peakMag)
}

// computeFFT 截取数据、加窗并执行FFT
func (pd *PitchDetector) computeFFT(samples []float64) []complex128 {
	// 只取最新的 FFTSize 个点
	input := samples[len(samples)-pd.config.FFTSize:]
	windowed := make([]float64, len(input))
	for i, v := range input {
		windowed[i] = v * pd.windowCache[i]
	}
	return fft.FFTReal(windowed)
}

// findPeak 在频谱中寻找最大能量点，并使用抛物线插值细化频率
func (pd *PitchDetector) findPeak(fftResult []complex128) (freq float64, mag float64) {
	binRes := pd.config.SampleRate / float64(pd.config.FFTSize)
	minBin := int(pd.config.MinFreq / binRes)
	maxBin := int(pd.config.MaxFreq / binRes)

	var maxMag float64 = -1.0
	var maxIndex int = -1

	// 粗略寻峰 (Coarse Search)
	// 只在感兴趣的频段内搜索
	for i := minBin; i < maxBin && i < len(fftResult)/2; i++ {
		m := cmplx.Abs(fftResult[i])
		if m > maxMag {
			maxMag = m
			maxIndex = i
		}
	}

	if maxIndex == -1 {
		return 0, 0
	}

	// 抛物线插值 (Parabolic Interpolation)
	// 防止数组越界
	if maxIndex <= 0 || maxIndex >= len(fftResult)-1 {
		return float64(maxIndex) * binRes, maxMag
	}

	y1 := cmplx.Abs(fftResult[maxIndex-1])
	y2 := maxMag
	y3 := cmplx.Abs(fftResult[maxIndex+1])

	delta := 0.0
	denominator := 2 * (2*y2 - y1 - y3)
	if denominator != 0 {
		delta = (y3 - y1) / denominator
	}

	detectedFreq := (float64(maxIndex) + delta) * binRes
	return detectedFreq, maxMag
}

// updateFrequencyState 根据当前探测结果更新内部状态（平滑、防跳变）
func (pd *PitchDetector) updateFrequencyState(detectedFreq, magnitude float64) (freq float64, found bool) {
	// 噪声门限判断
	if magnitude < pd.config.NoiseThreshold {
		// 信号太弱，保持上一次的状态，但标记 found=false
		return pd.lastFreq, false
	} else {
		//fmt.Printf("magnitude %f\n", magnitude)
	}

	// 惯性追踪与防跳变 (Inertial Tracking)
	if pd.hasLock {
		diff := math.Abs(detectedFreq - pd.lastFreq)
		if diff > pd.config.MaxJumpHz {
			// 频率突变过大！
			// 简单起见，这里直接丢弃突变，保持惯性
			return pd.lastFreq, true
		}
		// 平滑更新
		pd.lastFreq = (1.0-pd.config.SmoothingAlpha)*pd.lastFreq + pd.config.SmoothingAlpha*detectedFreq
	} else {
		// 第一次锁定，直接赋值
		pd.lastFreq = detectedFreq
		pd.hasLock = true
	}

	return pd.lastFreq, true
}
