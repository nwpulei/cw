package cw

import (
	"cw/Filters"
	"fmt"
	"math"
)

// SDRDemodulator implements Quadrature Down-Conversion (I/Q Demodulation)
type SDRDemodulator struct {
	sampleRate float64
	targetFreq float64 // [新增] 记录目标频率
	afcEnabled bool    // [新增] 记录 AFC 开关状态

	lpfI  *ButterworthFilter
	lpfQ  *ButterworthFilter
	afc   *Filters.AFC
	phase float64
}

func NewSDRDemodulator(sampleRate, targetFreq float64, cfg *Config) *SDRDemodulator {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	sdr := &SDRDemodulator{
		sampleRate: sampleRate,
		targetFreq: targetFreq,         // [记录]
		afcEnabled: cfg.SDR.AfcEnabled, // [记录] 听从 config 指挥

		lpfI: NewButterworthLowpass(4, sampleRate, cfg.SDR.FilterBW),
		lpfQ: NewButterworthLowpass(4, sampleRate, cfg.SDR.FilterBW),
		afc:  Filters.NewAFC(sampleRate, targetFreq),
	}
	return sdr
}

func (s *SDRDemodulator) SetTargetFreq(freq float64) {

	// 只有当新检测到的频率偏差超过 5Hz 时，才调整 SDR
	// 避免因为 1-2Hz 的检测误差导致 SDR 反复重置相位
	if math.Abs(freq-s.targetFreq) > 5.0 {
		fmt.Printf("[Auto-Tune] Following signal to %.1f Hz\n", freq)
		s.afc.UpdateTargetFreq(freq)
	}

	// 滤波器重置代码已被正确移除，保持现状
}

func (s *SDRDemodulator) Process(sample float64) float64 {
	// 1. LO generation
	loI := math.Cos(s.phase)
	loQ := math.Sin(s.phase)

	// 2. Mixing
	mixI := sample * loI
	mixQ := sample * loQ

	// 3. Low Pass Filtering
	filteredI := s.lpfI.Process(mixI)
	filteredQ := s.lpfQ.Process(mixQ)

	// 4. Envelope
	envelope := 2.0 * math.Sqrt(filteredI*filteredI+filteredQ*filteredQ)

	// 5. LO Phase Update (核心修复点)
	var phaseInc float64
	if s.afcEnabled {
		// 只有开启时才询问 AFC
		phaseInc = s.afc.Update(float64(filteredI), float64(filteredQ), envelope)
	} else {
		// 关闭时，直接计算固定的相位增量 (死锁频率)
		// Inc = 2 * PI * Freq / SampleRate
		phaseInc = 2.0 * math.Pi * s.targetFreq / s.sampleRate
	}

	s.updatePhase(phaseInc)

	return envelope
}

func (s *SDRDemodulator) updatePhase(phaseInc float64) {
	s.phase += phaseInc
	if s.phase > 2*math.Pi {
		s.phase -= 2 * math.Pi
	}
}
