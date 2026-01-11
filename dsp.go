package cw

import (
	"math"
	"math/cmplx"

	"github.com/mjibson/go-dsp/fft"
)

// SpectrumAnalyzer 用于频谱分析和峰值检测
type SpectrumAnalyzer struct {
	SampleRate float64
	FFTSize    int
	Window     []float64
}

// NewSpectrumAnalyzer 创建新的频谱分析器
func NewSpectrumAnalyzer(sampleRate float64, fftSize int) *SpectrumAnalyzer {
	// 创建汉宁窗 (Hanning Window)
	// 公式: 0.5 * (1 - cos(2*PI*n / (N-1)))
	window := make([]float64, fftSize)
	for i := 0; i < fftSize; i++ {
		window[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(fftSize-1)))
	}

	return &SpectrumAnalyzer{
		SampleRate: sampleRate,
		FFTSize:    fftSize,
		Window:     window,
	}
}

// FindDominantFrequency 计算当前音频块的主频
// 返回主频 (Hz) 和 对应的幅度
// minFreq, maxFreq: 限制搜索范围，避开低频噪声
func (sa *SpectrumAnalyzer) FindDominantFrequency(samples []float64, minFreq, maxFreq float64) (float64, float64) {
	if len(samples) < sa.FFTSize {
		return 0, 0
	}

	// 1. 应用窗函数
	input := make([]complex128, sa.FFTSize)
	for i := 0; i < sa.FFTSize; i++ {
		input[i] = complex(samples[i]*sa.Window[i], 0)
	}

	// 2. 执行 FFT
	spectrum := fft.FFT(input)

	// 3. 寻找幅度最大的频率分量
	maxMag := 0.0
	maxIndex := 0

	binWidth := sa.SampleRate / float64(sa.FFTSize)

	// 限制搜索范围
	startIndex := int(minFreq / binWidth)
	endIndex := int(maxFreq / binWidth)

	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex > len(spectrum)/2 {
		endIndex = len(spectrum) / 2
	}

	// 存储幅度谱以便插值
	mags := make([]float64, len(spectrum)/2+1)

	for i := startIndex; i < endIndex; i++ {
		mag := cmplx.Abs(spectrum[i])
		mags[i] = mag
		if mag > maxMag {
			maxMag = mag
			maxIndex = i
		}
	}

	// 4. 抛物线插值 (Parabolic Interpolation)
	// 利用峰值及其左右相邻点来估算真实的峰值位置
	// p = 0.5 * (alpha - gamma) / (alpha - 2*beta + gamma)
	// realPeak = bin + p

	var freq float64
	if maxIndex > 0 && maxIndex < len(mags)-1 {
		alpha := mags[maxIndex-1]
		beta := mags[maxIndex]
		gamma := mags[maxIndex+1]

		// 防止除零
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

	return freq, maxMag
}
