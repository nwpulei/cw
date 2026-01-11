package Filters

import (
	"math"
)

/*
*
频率锁定，通过检测相位，计算之前检测的频率是否准确。
并对相位进行矫正
*/
type AFC struct {
	sampleRate        float64 // 采样率 48000
	signalConsecutive int     // 连续信号计数
	CurrentFreq       float64 // 当前频率
	targetFreq        float64 // 目标频率
	prevPhase         float64 // 上一次的相位
	phaseInc          float64 // 频率增量
	gain              float64 // 增益
}

func NewAFC(sampleRate, targetFreq float64) *AFC {
	afc := AFC{
		sampleRate:        sampleRate,
		targetFreq:        targetFreq,
		CurrentFreq:       targetFreq,
		signalConsecutive: 0,
		prevPhase:         0,
		phaseInc:          0,
		gain:              0.0002, // 不要一次修到位，每次只修 0.01% (Gain = 0.0002)  这样可以极大地平滑噪音带来的抖动
	}
	afc.updatePhaseInc()
	return &afc
}

func (s *AFC) Update(filteredI, filteredQ, envelope float64) float64 {
	// 1. 门限控制：信号太弱时不瞎调，防止锁定噪音
	if envelope > 0.005 {
		currentPhase := math.Atan2(filteredQ, filteredI)

		if s.signalConsecutive > 5 {
			phaseDelta := currentPhase - s.prevPhase

			if phaseDelta > math.Pi {
				phaseDelta -= 2 * math.Pi
			} else if phaseDelta < -math.Pi {
				phaseDelta += 2 * math.Pi
			}

			// 4. 将相位差转换为频率误差 (Hz)
			// Formula: ErrorHz = (delta / (2*Pi)) * SampleRate
			freqError := phaseDelta * s.sampleRate / (2 * math.Pi)

			// 5. 死区控制 (Deadband) - 提升精度的关键！
			// 如果误差在 2Hz 以内，认为已经很准了，不动它，避免震荡。
			if math.Abs(freqError) > 2.0 {

				// 6. 缓慢修正 (Gain Control)
				// 不要一次修到位，每次只修 1% (Gain = 0.01)
				// 这样可以极大地平滑噪音带来的抖动
				correction := freqError * s.gain
				//maxStep := 0.5
				//if correction > maxStep {
				//	correction = maxStep
				//} else if correction < -maxStep {
				//	correction = -maxStep
				//}

				s.CurrentFreq += correction

				if s.CurrentFreq > s.targetFreq+100 {
					s.CurrentFreq = s.targetFreq + 100
				} else if s.CurrentFreq < s.targetFreq-100 {
					s.CurrentFreq = s.targetFreq - 100
				}
				s.updatePhaseInc()
			}
		}

		s.prevPhase = currentPhase
		s.signalConsecutive++
	} else {
		s.signalConsecutive = 0
	}
	return s.phaseInc
}

func (s *AFC) updatePhaseInc() {
	s.phaseInc = 2 * math.Pi * s.CurrentFreq / s.sampleRate
}

func (s *AFC) UpdateTargetFreq(freq float64) {
	if math.Abs(freq-s.targetFreq) < 5 {
		return
	}
	//fmt.Printf("[DEBUG] update freq %f -> %f\n", s.targetFreq, freq)
	s.targetFreq = freq
	s.CurrentFreq = freq
	s.updatePhaseInc()
	s.signalConsecutive = 0
}
