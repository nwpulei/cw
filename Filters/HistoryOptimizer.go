package Filters

import (
	"sort"
)

// HistoryOptimizer 维护一段历史时长的信号包络，用于计算最佳的静态阈值
type HistoryOptimizer struct {
	buffer     []float64 // 环形缓冲区
	head       int       // 写入位置
	isFull     bool      // 缓冲区是否已满
	sampleRate float64
	downSample int // 降采样倍率 (例如每 480 个点存 1 个)
	counter    int // 降采样计数器
}

// NewHistoryOptimizer 创建实例
// historyDuration: 历史时长 (秒)，建议 30.0
// sampleRate: 音频采样率，如 48000
func NewHistoryOptimizer(historyDuration float64, sampleRate float64) *HistoryOptimizer {
	// 目标：每秒存储 100 个点 (10ms 分辨率)，足够分析包络了
	targetRate := 100.0
	downSample := int(sampleRate / targetRate)
	if downSample < 1 {
		downSample = 1
	}

	bufferSize := int(historyDuration * targetRate)

	return &HistoryOptimizer{
		buffer:     make([]float64, bufferSize),
		sampleRate: sampleRate,
		downSample: downSample,
	}
}

// Push 输入当前的包络值 (envelope)
func (h *HistoryOptimizer) Push(value float64) {
	h.counter++
	if h.counter < h.downSample {
		return
	}
	h.counter = 0

	// 写入环形缓冲区
	h.buffer[h.head] = value
	h.head = (h.head + 1) % len(h.buffer)
	if h.head == 0 {
		h.isFull = true
	}
}

// SuggestThreshold 根据历史数据计算最佳阈值
// 返回值: (最佳阈值, 信号峰值, 噪声底值)
func (h *HistoryOptimizer) SuggestThreshold() (float64, float64, float64) {
	// 1. 提取有效数据
	var data []float64
	if h.isFull {
		// 必须复制一份数据进行排序，不能打乱原 buffer
		data = make([]float64, len(h.buffer))
		copy(data, h.buffer)
	} else {
		if h.head == 0 {
			return 0.05, 0.1, 0.0 // 还没数据
		}
		data = make([]float64, h.head)
		copy(data, h.buffer[:h.head])
	}

	// 2. 排序，寻找分位点
	sort.Float64s(data)
	count := len(data)

	// A. 估算底噪 (Noise Floor)
	// 取低位 10% 处的值，通常稳健地代表底噪水平
	noiseIndex := int(float64(count) * 0.10)
	noiseFloor := data[noiseIndex]

	// B. 估算信号峰值 (Signal Peak)
	// 取高位 95% 处的值 (排除极端的干扰脉冲)
	signalIndex := int(float64(count) * 0.95)
	signalPeak := data[signalIndex]

	// C. 安全检查
	// 如果信号太弱 (峰值和底噪几乎一样)，说明没信号
	if signalPeak < noiseFloor*1.5 {
		// 默认返回一个稍高于底噪的值
		return noiseFloor * 3.0, signalPeak, noiseFloor
	}

	// D. 计算最佳阈值 (Threshold)
	// 经典算法：在对数域或线性域取中间值。
	// 这里使用线性域的加权平均：底噪 + (动态范围 * 30%)
	// 30% 是一个经验值，既能避开底噪毛刺，又能在信号衰落时保持锁定
	threshold := noiseFloor + (signalPeak-noiseFloor)*0.2

	return threshold, signalPeak, noiseFloor
}
