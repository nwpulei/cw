package cw

import (
	"bufio"
	"fmt"
	"math"
	"os"
)

// ClusterDecoder 使用 K-Means 聚类算法进行高精度 CW 解码
type ClusterDecoder struct {
	cfg *Config // 引用全局配置
	sdr *SDRDemodulator

	// 配置 (动态)
	ThresholdHigh float64
	ThresholdLow  float64

	// 状态
	signalState      bool
	samplesProcessed int64
	stateStartSample int64

	// AGC (自动增益/阈值控制)
	signalPeak float64 // 当前跟踪到的信号峰值

	// 统计缓冲区
	markBuffer  *WindowBuffer
	spaceBuffer *WindowBuffer

	// 聚类结果 (缓存)
	dotLen     float64
	dashLen    float64
	elemGapLen float64
	charGapLen float64

	// 输出
	symbolBuffer string
	OnDecoded    func(string)

	// Debug
	debugFile   *os.File
	debugWriter *bufio.Writer
}

// NewClusterDecoder 创建实例
func NewClusterDecoder(sampleRate, targetFreq float64, cfg *Config) *ClusterDecoder {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// 创建调试文件
	f, err := os.Create("debug_signal.txt")
	var bw *bufio.Writer
	if err != nil {
		fmt.Printf("Error creating debug_signal.txt: %v\n", err)
	} else {
		bw = bufio.NewWriter(f)
	}

	return &ClusterDecoder{
		cfg:           cfg,
		sdr:           NewSDRDemodulator(sampleRate, targetFreq, cfg),
		ThresholdHigh: 0.05,
		ThresholdLow:  0.03,
		signalPeak:    0.05,
		markBuffer:    NewWindowBuffer(cfg.Decoder.MarkWindowSize),
		spaceBuffer:   NewWindowBuffer(cfg.Decoder.SpaceWindowSize),
		// 初始默认值 (20 WPM)
		dotLen:      0.06,
		dashLen:     0.18,
		elemGapLen:  0.06,
		charGapLen:  0.18,
		debugFile:   f,
		debugWriter: bw,
	}
}

// ProcessAudioChunk 处理音频块
func (d *ClusterDecoder) ProcessAudioChunk(samples []float32) {
	for _, s := range samples {
		d.processSample(float64(s))
	}
}

func (d *ClusterDecoder) processSample(sample float64) {
	d.samplesProcessed++

	// 1. SDR 解调
	envelope := d.sdr.Process(sample)

	// DEBUG: 打印包络和阈值信息 (每 2000 个采样点打印一次，避免刷屏)
	if d.samplesProcessed%2000 == 0 {
		//fmt.Printf("[DEBUG] Env: %.4f, Peak: %.4f, ThHigh: %.4f, ThLow: %.4f, State: %v\n", envelope, d.signalPeak, d.ThresholdHigh, d.ThresholdLow, d.signalState)
	}

	// --- AGC (动态阈值跟踪) ---
	if d.cfg.Decoder.AgcEnabled {
		d.signalPeak *= d.cfg.Decoder.AgcPeakDecay
		if d.signalPeak < d.cfg.Decoder.AgcPeakFloor {
			d.signalPeak = d.cfg.Decoder.AgcPeakFloor
		}

		if envelope > d.signalPeak {
			d.signalPeak = envelope
		}

		newHigh := d.signalPeak * d.cfg.Decoder.AgcHighRatio
		if newHigh < d.cfg.Decoder.AgcMinHigh {
			newHigh = d.cfg.Decoder.AgcMinHigh
		}

		d.ThresholdHigh = newHigh
		d.ThresholdLow = newHigh * d.cfg.Decoder.AgcLowRatio
	}
	// ---------------------------

	// 2. 施密特触发器
	isSignal := d.signalState
	if d.signalState {
		if envelope < d.ThresholdLow {
			isSignal = false
		}
	} else {
		if envelope > d.ThresholdHigh {
			isSignal = true
		}
	}

	// 3. 状态转换
	if isSignal != d.signalState {
		now := d.samplesProcessed
		durationSamples := now - d.stateStartSample
		durationSec := float64(durationSamples) / d.sdr.sampleRate

		// DEBUG: 打印状态转换
		//fmt.Printf("[DEBUG] State Change: %v -> %v (Duration: %.4fs)\n", d.signalState, isSignal, durationSec)

		if d.signalState {
			// Mark 结束 -> 记录 Mark 时长，开始 Space
			d.handleMarkEnd(durationSec)
		} else {
			// Space 结束 -> 记录 Space 时长，开始 Mark
			d.handleSpaceEnd(durationSec)
		}

		d.signalState = isSignal
		d.stateStartSample = now
	}

	// 输出到调试文件
	if d.debugWriter != nil {
		if d.signalState {
			d.debugWriter.WriteString("1\n")
		} else {
			d.debugWriter.WriteString("0\n")
		}
		// 定期刷新，防止程序异常退出导致数据丢失
		if d.samplesProcessed%4096 == 0 {
			d.debugWriter.Flush()
		}
	}

	// 4. 处理超长静音 (实时输出空格)
	if !d.signalState {
		durationSamples := d.samplesProcessed - d.stateStartSample
		durationSec := float64(durationSamples) / d.sdr.sampleRate

		// 单词间隔阈值
		wordGapThreshold := d.charGapLen * d.cfg.Decoder.WordGapRatio
		if wordGapThreshold < 0.2 {
			wordGapThreshold = 0.2
		}

		if durationSec > wordGapThreshold && d.symbolBuffer == "" {
			// 已经处理过或 buffer 为空，不做操作
		} else if durationSec > wordGapThreshold && d.symbolBuffer != "" {
			// 强制输出单词间隔
			fmt.Printf("[DEBUG] Word Gap Detected (%.4fs > %.4fs)\n", durationSec, wordGapThreshold)
			d.decodeBuffer()
			d.emit(" ")
		}
	}
}

func (d *ClusterDecoder) handleMarkEnd(duration float64) {
	// 过滤极短脉冲 (Glitch)
	if duration < float64(d.cfg.Decoder.MarkGlitchMs)/1000.0 {
		//fmt.Printf("[DEBUG] Ignored Mark Glitch: %.4fs\n", duration)
		return
	}

	// 1. 加入统计缓冲区
	d.markBuffer.Add(duration)

	// 2. 重新聚类计算点划长度
	d.updateMarkClusters()

	// 3. 判定当前符号
	// 阈值 = (点 + 划) / 2
	threshold := (d.dotLen + d.dashLen) / 2.0

	var symbol string
	if duration < threshold {
		symbol = "."
	} else {
		symbol = "-"
	}

	//fmt.Printf("[DEBUG] Mark: %.4fs -> %s (Th: %.4f, Dot: %.4f, Dash: %.4f)\n", duration, symbol, threshold, d.dotLen, d.dashLen)

	d.symbolBuffer += symbol

	if len(d.symbolBuffer) > 7 {
		d.decodeBuffer()
	}
}

func (d *ClusterDecoder) handleSpaceEnd(duration float64) {
	// 过滤极短静音 (Glitch)
	if duration < float64(d.cfg.Decoder.SpaceGlitchMs)/1000.0 {
		//fmt.Printf("[DEBUG] Ignored Space Glitch: %.4fs\n", duration)
		return
	}

	// 1. 加入统计缓冲区
	d.spaceBuffer.Add(duration)

	// 2. 重新聚类计算间隔长度
	d.updateSpaceClusters()

	// 3. 判定间隔类型
	// 字符分割阈值 = (元素间隔 + 字符间隔) / 2
	charThreshold := (d.elemGapLen + d.charGapLen) / 2.0

	// 硬性兜底
	minCharGap := float64(d.cfg.Decoder.CharGapMinMs) / 1000.0
	if charThreshold < minCharGap {
		charThreshold = minCharGap
	}

	//fmt.Printf("[DEBUG] Space: %.4fs (Th: %.4f, Elem: %.4f, Char: %.4f)\n", duration, charThreshold, d.elemGapLen, d.charGapLen)

	if duration > charThreshold {
		// 字符间隔 -> 解码当前 buffer
		d.decodeBuffer()
	} else {
		// 元素间隔 -> 不做操作，等待下一个点划
	}
}

// updateMarkClusters 使用 K-Means (K=2) 更新点划长度估计
func (d *ClusterDecoder) updateMarkClusters() {
	data := d.markBuffer.GetData()
	if len(data) < 2 {
		return
	}

	// 简单的 K-Means
	c1 := data[0] // Dot
	c2 := data[0] // Dash
	for _, v := range data {
		if v < c1 {
			c1 = v
		}
		if v > c2 {
			c2 = v
		}
	}

	if c2 < c1*1.5 {
		c2 = c1 * 3.0
	}

	for i := 0; i < 5; i++ {
		sum1, count1 := 0.0, 0.0
		sum2, count2 := 0.0, 0.0

		for _, v := range data {
			dist1 := math.Abs(v - c1)
			dist2 := math.Abs(v - c2)
			if dist1 < dist2 {
				sum1 += v
				count1++
			} else {
				sum2 += v
				count2++
			}
		}

		if count1 > 0 {
			c1 = sum1 / count1
		}
		if count2 > 0 {
			c2 = sum2 / count2
		}
	}

	if c1 > c2 {
		c1, c2 = c2, c1
	}

	d.dotLen = c1
	d.dashLen = c2

	// 限制范围
	if d.dotLen < d.cfg.Decoder.MinDotLen {
		d.dotLen = d.cfg.Decoder.MinDotLen
	}
	if d.dotLen > d.cfg.Decoder.MaxDotLen {
		d.dotLen = d.cfg.Decoder.MaxDotLen
	}
	if d.dashLen < d.dotLen*2.0 {
		d.dashLen = d.dotLen * 3.0
	}
}

// updateSpaceClusters 使用 K-Means (K=2) 更新间隔长度估计
func (d *ClusterDecoder) updateSpaceClusters() {
	data := d.spaceBuffer.GetData()
	if len(data) < 2 {
		return
	}

	c1 := 0.06 // ElemGap
	c2 := 0.18 // CharGap

	if d.dotLen > 0 {
		c1 = d.dotLen
		c2 = d.dotLen * 3.0
	}

	for i := 0; i < 5; i++ {
		sum1, count1 := 0.0, 0.0
		sum2, count2 := 0.0, 0.0

		for _, v := range data {
			dist1 := math.Abs(v - c1)
			dist2 := math.Abs(v - c2)
			if dist1 < dist2 {
				sum1 += v
				count1++
			} else {
				sum2 += v
				count2++
			}
		}

		if count1 > 0 {
			c1 = sum1 / count1
		}
		if count2 > 0 {
			c2 = sum2 / count2
		}
	}

	if c1 > c2 {
		c1, c2 = c2, c1
	}

	d.elemGapLen = c1
	d.charGapLen = c2
}

func (d *ClusterDecoder) decodeBuffer() {
	if d.symbolBuffer == "" {
		return
	}
	fmt.Printf("[DEBUG] Decoding Buffer: [%s]\n", d.symbolBuffer)
	if char, ok := MorseCodeMap[d.symbolBuffer]; ok {
		d.emit(char)
	}
	d.symbolBuffer = ""
}

func (d *ClusterDecoder) emit(text string) {
	if d.OnDecoded != nil {
		d.OnDecoded(text)
	} else {
		fmt.Print(text)
	}
}

// 接口实现
func (d *ClusterDecoder) UpdateTargetFreq(freq float64) { d.sdr.SetTargetFreq(freq) }
func (d *ClusterDecoder) SetThreshold(t float64) {
	// 初始设置，后续会被 AGC 覆盖
	d.ThresholdHigh = t
	d.ThresholdLow = t * d.cfg.Decoder.AgcLowRatio
	d.signalPeak = t * 2.0
}
func (d *ClusterDecoder) SetOnDecoded(cb func(string)) { d.OnDecoded = cb }

// --- 辅助类: 滑动窗口缓冲区 ---

type WindowBuffer struct {
	buffer []float64
	index  int
	full   bool
}

func NewWindowBuffer(size int) *WindowBuffer {
	return &WindowBuffer{
		buffer: make([]float64, size),
	}
}

func (wb *WindowBuffer) Add(val float64) {
	wb.buffer[wb.index] = val
	wb.index = (wb.index + 1) % len(wb.buffer)
	if wb.index == 0 {
		wb.full = true
	}
}

func (wb *WindowBuffer) GetData() []float64 {
	if !wb.full {
		return wb.buffer[:wb.index]
	}
	return wb.buffer
}
