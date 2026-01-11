package cw

import (
	"math"
	"testing"
)

const (
	testSampleRate = 48000.0
	testFFTSize    = 2048
)

// 生成正弦波辅助函数
func generateSineWave(freq float64, durationSec float64, sampleRate float64) []float64 {
	samples := int(durationSec * sampleRate)
	data := make([]float64, samples)
	for i := 0; i < samples; i++ {
		t := float64(i) / sampleRate
		data[i] = math.Sin(2 * math.Pi * freq * t)
	}
	return data
}

func TestPitchDetector_Accuracy(t *testing.T) {
	cfg := PitchDetectorConfig{
		SampleRate:     testSampleRate,
		FFTSize:        testFFTSize,
		MinFreq:        400,
		MaxFreq:        1000,
		SmoothingAlpha: 1.0, // 测试时设为 1.0 以禁用平滑，直接看单帧结果
		MaxJumpHz:      1000,
		NoiseThreshold: 0.1,
	}
	pd := NewPitchDetector(cfg)

	// 测试场景 1: 精准落在 Bin 上的频率
	// Resolution = 48000 / 2048 = 23.4375 Hz
	// 23.4375 * 25 = 585.9375 Hz
	targetFreq1 := 585.9375
	input1 := generateSineWave(targetFreq1, 0.1, testSampleRate) // 0.1s足够覆盖2048点
	detected1, found1 := pd.Detect(input1)

	if !found1 {
		t.Fatal("Should verify signal")
	}
	if math.Abs(detected1-targetFreq1) > 0.1 {
		t.Errorf("Exact Bin Test Failed: Target %v, Got %v", targetFreq1, detected1)
	}

	// 测试场景 2: 落在两个 Bin 中间的频率 (测试插值能力)
	// 600 Hz 并不在 Bin 整数倍上
	targetFreq2 := 600.0
	pd.Reset()
	input2 := generateSineWave(targetFreq2, 0.1, testSampleRate)
	detected2, _ := pd.Detect(input2)

	// 允许 1Hz 的误差（通常插值后误差在 0.1Hz 以内，但考虑到加窗影响放宽一点）
	if math.Abs(detected2-targetFreq2) > 1.0 {
		t.Errorf("Interpolation Test Failed: Target %v, Got %v", targetFreq2, detected2)
	} else {
		t.Logf("Interpolation Success: Target %v, Got %v, Error %v", targetFreq2, detected2, math.Abs(detected2-targetFreq2))
	}
}

func TestPitchDetector_InterferenceProtection(t *testing.T) {
	cfg := PitchDetectorConfig{
		SampleRate:     testSampleRate,
		FFTSize:        testFFTSize,
		MinFreq:        300,
		MaxFreq:        1200,
		SmoothingAlpha: 0.1,  // 开启平滑
		MaxJumpHz:      50.0, // 设定保护门限 50Hz
		NoiseThreshold: 0.1,
	}
	pd := NewPitchDetector(cfg)

	// 1. 先锁定 600Hz 信号
	input600 := generateSineWave(600.0, 0.1, testSampleRate)
	// 预热几次让它稳定锁定
	for i := 0; i < 5; i++ {
		pd.Detect(input600)
	}
	stableFreq, _ := pd.Detect(input600)
	t.Logf("Locked at: %v", stableFreq)

	// 2. 突然输入 800Hz 的强干扰信号
	// 由于 MaxJumpHz = 50，这个 800Hz 应该被忽略，频率应该维持在 600Hz 附近
	input800 := generateSineWave(800.0, 0.1, testSampleRate)
	detectedInterference, _ := pd.Detect(input800)

	if math.Abs(detectedInterference-600.0) > 10.0 {
		t.Errorf("Protection Failed: Jumped to %v, expected to stay near 600", detectedInterference)
	} else {
		t.Logf("Protection Success: Input 800Hz, Output stayed at %v", detectedInterference)
	}
}

func TestPitchDetector_Silence(t *testing.T) {
	cfg := PitchDetectorConfig{
		SampleRate:     testSampleRate,
		FFTSize:        testFFTSize,
		MinFreq:        300,
		MaxFreq:        1200,
		NoiseThreshold: 1.0, // 调高门限
	}
	pd := NewPitchDetector(cfg)

	// 输入极小幅度的噪音
	inputSilence := generateSineWave(600.0, 0.1, testSampleRate)
	for i := range inputSilence {
		inputSilence[i] *= 0.001 // 衰减
	}

	_, found := pd.Detect(inputSilence)
	if found {
		t.Error("Should not detect signal in silence/noise below threshold")
	}
}
