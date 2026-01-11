package Filters

import "math"

type AFCEngine struct {
	TargetFreq    float64
	CurrentOffset float64 // 当前修正量

	lastPhase float64
	smoothing float64 // 平滑系数 (0.0 - 1.0)
}

func (a *AFCEngine) Update(I, Q, magnitude float64) {
	// 1. 门限控制：信号太弱时不瞎调，防止锁定噪音
	if magnitude < 0.05 {
		return
	}

	// 2. 计算当前相位
	currPhase := math.Atan2(Q, I)

	// 3. 计算相位差 (注意处理 -pi 到 +pi 的跳变)
	delta := currPhase - a.lastPhase
	if delta > math.Pi {
		delta -= 2 * math.Pi
	}
	if delta < -math.Pi {
		delta += 2 * math.Pi
	}

	a.lastPhase = currPhase

	// 4. 将相位差转换为频率误差 (Hz)
	// Formula: ErrorHz = (delta / (2*Pi)) * SampleRate
	rawErrorHz := (delta / (2 * math.Pi)) * 48000.0

	// 5. 死区控制 (Deadband) - 提升精度的关键！
	// 如果误差在 2Hz 以内，认为已经很准了，不动它，避免震荡。
	if math.Abs(rawErrorHz) < 2.0 {
		return
	}

	// 6. 缓慢修正 (Gain Control)
	// 不要一次修到位，每次只修 1% (Gain = 0.01)
	// 这样可以极大地平滑噪音带来的抖动
	gain := 0.01
	a.CurrentOffset += rawErrorHz * gain

	// 限制最大修正范围 (比如只允许追 ±50Hz，防止跑飞)
	if a.CurrentOffset > 50 {
		a.CurrentOffset = 50
	}
	if a.CurrentOffset < -50 {
		a.CurrentOffset = -50
	}
}
