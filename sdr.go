package cw

import (
	"cw/Filters"
	"math"
)

// SDRDemodulator implements Quadrature Down-Conversion (I/Q Demodulation)
// It mixes the input signal with a local oscillator (LO) to extract the envelope
// and performs Automatic Frequency Control (AFC) to track the signal.
type SDRDemodulator struct {
	// 配置
	sampleRate float64

	// Low Pass Filter state
	lpfI *ButterworthFilter
	lpfQ *ButterworthFilter
	afc  *Filters.AFC
	// Local Oscillator state
	phase float64
}

// NewSDRDemodulator creates a new SDR demodulator
func NewSDRDemodulator(sampleRate, targetFreq float64, cfg *Config) *SDRDemodulator {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	// 创建 4 阶巴特沃斯低通滤波器
	sdr := &SDRDemodulator{
		sampleRate: sampleRate,
		lpfI:       NewButterworthLowpass(4, sampleRate, cfg.SDR.FilterBW),
		lpfQ:       NewButterworthLowpass(4, sampleRate, cfg.SDR.FilterBW),
		afc:        Filters.NewAFC(sampleRate, targetFreq),
	}

	return sdr
}

// SetTargetFreq updates the base target frequency
func (s *SDRDemodulator) SetTargetFreq(freq float64) {
	s.afc.UpdateTargetFreq(freq)
	// 重置滤波器状态
	//cfg := DefaultConfig() // Get default config to access filter params
	//s.lpfI = NewButterworthLowpass(4, s.sampleRate, cfg.SDR.FilterBW)
	//s.lpfQ = NewButterworthLowpass(4, s.sampleRate, cfg.SDR.FilterBW)
}

// Process processes a single audio sample and returns the signal envelope
func (s *SDRDemodulator) Process(sample float64) float64 {
	// 1. Local Oscillator (LO) generation
	loI := math.Cos(s.phase)
	loQ := math.Sin(s.phase)

	// 2. Mixing (Down-conversion)
	mixI := sample * loI
	mixQ := sample * loQ

	// 3. Low Pass Filtering (LPF)
	// 使用新的巴特沃斯滤波器
	filteredI := s.lpfI.Process(mixI)
	filteredQ := s.lpfQ.Process(mixQ)

	//if n < 20 {
	//	fmt.Printf("en %e,%e,%e,%e,%e,%e,%e,%e\n", s.phase, sample, loI, loQ, mixI, mixQ, filteredI, filteredQ)
	//	n++
	//}
	// 4. Envelope Calculation
	envelope := 2.0 * math.Sqrt(filteredI*filteredI+filteredQ*filteredQ)

	// 5. Automatic Frequency Control (AFC)
	phaseInc := s.afc.Update(float64(filteredI), float64(filteredQ), envelope)
	// Update LO phase for next sample
	s.updatePhase(phaseInc)

	return envelope
}

func (s *SDRDemodulator) updatePhase(phaseInc float64) {
	s.phase += phaseInc
	if s.phase > 2*math.Pi {
		s.phase -= 2 * math.Pi
	}
}
