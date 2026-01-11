package Filters

import (
	"math"
	"sort"
)

// SimpleAGC 实现“快充慢放”的自动增益控制
// AGC指自动增益控制(Automatic Gain Control)，
// 是一种自动调整信号强度（音量、亮度等）的功能，
// 使输出保持稳定，确保信号在强弱变化时音量适中不失真
type SimpleAGC struct {
	peak      float64
	decayRate float64 // 衰减系数，控制适应速度
}

func NewSimpleAGC(decayFactor float64) *SimpleAGC {
	return &SimpleAGC{
		peak:      0.001, // 避免除以0
		decayRate: decayFactor,
	}
}

// Update 处理样本并返回归一化后的值 (0.0 - 1.0)
func (agc *SimpleAGC) Update(sample float64) float64 {
	val := sample
	if val < 0 {
		val = -val
	}

	// Fast Attack: 如果当前值超过峰值，立即推高峰值
	if val > agc.peak {
		agc.peak = val
	} else {
		// Slow Decay: 否则让峰值缓慢衰减
		agc.peak *= agc.decayRate
		// 设定一个底限，防止在纯静音时放大底噪
		if agc.peak < 0.005 {
			agc.peak = 0.005
		}
	}

	// 返回归一化幅度
	return val / agc.peak
}

type RobustAGC struct {
	peak      float64
	decayRate float64
}

func (agc *RobustAGC) Update(sample float64) float64 {
	val := math.Abs(sample)

	// 核心逻辑：非对称的增量更新
	// 我们想锁定 95% 分位点，意味着：
	// 我们希望只有 5% 的时间 sample > peak
	// 所以 sample > peak 时的“惩罚”应该是 sample < peak 时的 19 倍 (95/5)

	if val > agc.peak {
		// 只有 5% 的机会进这里，所以步长要小，或者按比例控制
		// 这种累加方式模拟了分位点追踪
		agc.peak += 0.001 * val
	} else {
		// 有 95% 的机会进这里
		agc.peak -= 0.00005 * val // 这里的减量是增量的 1/20 左右
	}

	// 安全底限
	if agc.peak < 0.001 {
		agc.peak = 0.001
	}

	// 归一化
	normalized := val / agc.peak

	// 软限幅 (Soft Clipping)：防止因为偶尔的估算滞后导致输出 > 1.0
	if normalized > 1.0 {
		normalized = 1.0
	}

	return normalized
}

type MedianAGC struct {
	buffer    []float64
	cursor    int
	size      int
	simpleAGC *SimpleAGC
}

func NewMedianAGC() *MedianAGC {
	return &MedianAGC{
		size:      5,
		buffer:    make([]float64, 5), // 5阶中值滤波
		simpleAGC: NewSimpleAGC(0.99995),
	}
}

func (m *MedianAGC) Update(sample float64) float64 {
	// 1. 存入环形缓冲区
	m.buffer[m.cursor] = sample
	m.cursor = (m.cursor + 1) % m.size

	// 2. 复制并排序以找到中值
	// 注意：为了性能，对于小数组(3或5)，可以用硬编码比较代替 sort
	tmp := make([]float64, len(m.buffer))
	copy(tmp, m.buffer)

	// 简单的冒泡排序或插入排序 (对于5个元素，这比 sort.Float64s 快得多)
	// 这里为了演示用 sort，生产环境建议手写比较网络
	sort.Float64s(tmp)

	median := tmp[m.size/2] // 取第3个，即中位数

	// 3. 将清洗后的数据喂给 AGC
	return m.simpleAGC.Update(median)
}
