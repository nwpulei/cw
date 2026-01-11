package cw

import (
	"math"
)

// Goertzel 用于检测特定频率的能量
type Goertzel struct {
	sampleRate float64
	targetFreq float64
	coeff      float64
	q1         float64
	q2         float64
}

// NewGoertzel 初始化算法
func NewGoertzel(sampleRate, targetFreq float64) *Goertzel {
	// 计算系数
	// k = (N * targetFreq) / sampleRate
	// coeff = 2 * cos(2 * PI * k / N)
	// 简化后 coeff = 2 * cos(2 * PI * targetFreq / sampleRate)

	normalizedFreq := targetFreq / sampleRate
	coeff := 2.0 * math.Cos(2.0*math.Pi*normalizedFreq)

	return &Goertzel{
		sampleRate: sampleRate,
		targetFreq: targetFreq,
		coeff:      coeff,
		q1:         0,
		q2:         0,
	}
}

// Reset 重置状态，通常在处理完一个块（Block）后调用
func (g *Goertzel) Reset() {
	g.q1 = 0
	g.q2 = 0
}

// ProcessSample 处理单个采样点
func (g *Goertzel) ProcessSample(sample float64) {
	q0 := g.coeff*g.q1 - g.q2 + sample
	g.q2 = g.q1
	g.q1 = q0
}

// ProcessBlock 处理一整块音频数据
func (g *Goertzel) ProcessBlock(samples []float64) {
	for _, s := range samples {
		g.ProcessSample(s)
	}
}

// Detect 计算当前块的能量幅度
// 返回值越大，表示该频率成分越强
func (g *Goertzel) Detect() float64 {
	// magnitude^2 = q1^2 + q2^2 - q1*q2*coeff
	magnitudeSquared := g.q1*g.q1 + g.q2*g.q2 - g.q1*g.q2*g.coeff
	if magnitudeSquared < 0 {
		return 0
	}
	return math.Sqrt(magnitudeSquared)
}
