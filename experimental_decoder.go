package cw

import (
	"cw/BeamDecoder"
	"cw/Filters"
	"fmt"
)

// ExperimentalDecoder implements the new decoding logic:
// 1. SDR-based Demodulation (I/Q)
// 2. Beam Search Decoder (Logic & WPM Tracking)
type ExperimentalDecoder struct {
	sdr              *SDRDemodulator
	beam             *BeamDecoder.CWDecoder
	agc              *Filters.MedianAGC
	samplesProcessed int64
	// Callback
	OnDecoded func(string)

	debugger      SignalDebugger
	trigger       *Filters.SchmittTrigger
	pitchDetector *PitchDetector

	historyOpt   *Filters.HistoryOptimizer // 历史分析器
	processedCnt int                       // 用于定期触发计算的计数器
}

// NewExperimentalDecoder creates the new decoder instance
func NewExperimentalDecoder(sampleRate, targetFreq float64) *ExperimentalDecoder {
	// Use DefaultConfig for the experimental decoder
	cfg := DefaultConfig()
	dbg, _ := NewCsvFileDebugger("debug_session_01.csv")

	// Debounce window: 5ms
	debounceMs := 0.012
	// 【解耦点】初始化施密特触发器
	// 阈值 0.5/0.4, 去抖 12ms
	trigger := Filters.NewSchmittTrigger(sampleRate, 0.5, 0.4, debounceMs)
	lmodel := BeamDecoder.NewLanguageModel()
	// 衰减系数 0.99995 (假设48kHz采样) 意味着峰值大约在 1-2秒内衰减一半
	// 适合 CW 这种时断时续的信号
	agc := Filters.NewMedianAGC()
	sdr := NewSDRDemodulator(sampleRate, targetFreq, cfg)
	// 初始化历史优化器，记录最近 30 秒
	historyOpt := Filters.NewHistoryOptimizer(30.0, sampleRate)

	pitch := NewPitchDetector(PitchDetectorConfig{
		SampleRate:     sampleRate,
		FFTSize:        1024,
		MinFreq:        300,
		MaxFreq:        1200,
		SmoothingAlpha: 0.1,
		MaxJumpHz:      50,
		NoiseThreshold: 8,
	})
	cwDecoder := BeamDecoder.NewCWDecoder(BeamDecoder.DecoderConfig{
		InitialWPM:        30,   // 初始假设
		GlitchThresholdMs: 20.0, // 过滤极短噪声
		UpdateAlpha:       0.25,
	},
		lmodel,
	)
	return &ExperimentalDecoder{
		sdr:  sdr,
		beam: cwDecoder,

		agc:     agc,
		trigger: trigger,

		debugger:      dbg,
		pitchDetector: pitch,
		historyOpt:    historyOpt,
	}
}

// ProcessAudioChunk processes a block of audio samples
func (d *ExperimentalDecoder) ProcessAudioChunk(samples []float32) {
	sampe64 := make([]float64, len(samples))
	for i, s := range samples {
		// 转一次 float64 即可，避免重复转换
		val := float64(s)

		// 任务 A: 逐个样本处理 (SDR/解码)
		d.processSample(val)

		// 任务 B: 填充切片 (用于下面的 Pitch Detect)
		// 因为长度已经初始化为 len(samples)，所以这里使用下标 [i] 是安全的
		sampe64[i] = val
	}

	//a, b := d.pitchDetector.Detect(sampe64)
	//if b == true {
	//	d.UpdateTargetFreq(a)
	//	//fmt.Printf("[debug] find %f\n", a)
	//
	//}
}

func (d *ExperimentalDecoder) processSample(sample float64) {
	d.samplesProcessed++
	d.processedCnt++

	// 1.Orthogonal Down-Conversion + Butterworth Filter
	rawEnvelope := d.sdr.Process(sample)
	// 注意：这里不需要再过 d.agc.Update 了，
	// 因为我们要用历史统计来做更有智慧的 AGC。
	// 直接把 rawEnvelope 喂给历史分析器即可。
	d.historyOpt.Push(rawEnvelope)

	// 2. 定期更新阈值 (例如每 2 秒更新一次)
	// 48000 * 2 = 96000
	if d.processedCnt > 96000 {
		d.processedCnt = 0

		// ★ 核心魔法：从历史中获取智慧
		bestThresh, peak, noise := d.historyOpt.SuggestThreshold()

		// 更新施密特触发器的阈值
		// High = 最佳阈值
		// Low  = 最佳阈值 * 0.8 (防止抖动)
		d.trigger.SetThresholds(bestThresh, bestThresh*0.8)

		// 可选：打印调试信息，看看现在的决策是基于什么数据
		fmt.Printf("[AUTO-TUNE] Noise: %.4f | Peak: %.4f | Set Thresh: %.4f\n", noise, peak, bestThresh)
	}

	// 2. AGC Normalization (关键步骤)
	//envelope := d.agc.Update(rawEnvelope)

	// 【调试插桩】如果还不工作，取消下面这行的注释，看看 envelope 到底是多少
	//if d.samplesProcessed%1000 == 0 {
	// fmt.Printf("Env: %.4f | Thr: %.4f\n", envelope, d.ThresholdHigh)
	//}

	// 3. 状态检测 (委托给 SchmittTrigger)
	transition := d.trigger.Feed(rawEnvelope)

	if transition != nil {
		// 映射 bool -> BeamDecoder 枚举
		// finishedState true = 刚刚结束的是 Signal (Mark)
		// finishedState false = 刚刚结束的是 Silence (Space)
		finishedState := BeamDecoder.StateOff
		//stateStr := "SPACE"

		if transition.FinishedState {
			finishedState = BeamDecoder.StateOn
			///	stateStr = "MARK "
		}

		// 打印调试信息
		//fmt.Printf("\033[s\033[H\033[10B [DEBUG]-> Feed: %s | %.2f ms\033[u\n", stateStr, transition.DurationMs)

		// 输入到 Beam Decoder
		decodedText := d.beam.FeedNew(transition.DurationMs, finishedState)

		if decodedText != "" {
			d.emit(decodedText)
		}
	}
}

func (d *ExperimentalDecoder) emit(text string) {
	if d.OnDecoded != nil {
		d.OnDecoded(text)
	} else {
		fmt.Print("\033[s\033[H\033[8B " + text + "\r\n\033[u")
		//fmt.Print(text)
	}
}

// Interface compliance methods
func (d *ExperimentalDecoder) UpdateTargetFreq(freq float64) {
	d.sdr.SetTargetFreq(freq)
}

func (d *ExperimentalDecoder) SetThreshold(threshold float64) {
	//d.ThresholdHigh = threshold
	//d.ThresholdLow = threshold * 0.85
}

func (d *ExperimentalDecoder) SetOnDecoded(callback func(string)) {
	d.OnDecoded = callback
}

func (d *ExperimentalDecoder) Stop() {
	d.emit(d.beam.CheckTimeout())
	d.debugger.Close()
}
