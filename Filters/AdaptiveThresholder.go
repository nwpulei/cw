package Filters

// AdaptiveThresholder 实现双路包络追踪，用于生成动态的 Schmidt 触发阈值。
// 它可以抵抗 QSB (信号衰落) 并具备自动静噪功能。
type AdaptiveThresholder struct {
	// 状态变量
	maxLevel float64 // 追踪信号顶部的包络 (Signal Peak)
	minLevel float64 // 追踪底噪的基准 (Noise Floor)

	// 配置参数
	decayRate float64 // 衰减系数 (0.0 ~ 1.0)，控制 max 下降和 min 上升的速度
	minRange  float64 // 最小动态范围，小于此值视为静噪开启
}

// NewAdaptiveThresholder 初始化追踪器
// decayRate: 推荐 0.9995 (48kHz)
// minRange: 推荐 0.2 (视 AGC 增益策略而定)
func NewAdaptiveThresholder(decayRate, minRange float64) *AdaptiveThresholder {
	return &AdaptiveThresholder{
		maxLevel:  0.0,
		minLevel:  0.0,
		decayRate: decayRate,
		minRange:  minRange,
	}
}

// Update 更新追踪器状态并计算当前的迟滞阈值。
// 输入 sample: 经过 AGC 归一化的信号包络 (0.0 ~ 1.0)
// 输出 high, low: 用于施密特触发器的动态阈值
func (at *AdaptiveThresholder) Update(sample float64) (high, low float64) {
	// 1. Max Level 追踪 (Fast Attack, Slow Decay)
	// 如果当前样本大于记录的峰值，立即更新（捕捉信号上升沿）
	// 否则，按系数衰减（在信号间隙缓慢下降，适应 fading）
	if sample > at.maxLevel {
		at.maxLevel = sample
	} else {
		at.maxLevel *= at.decayRate
	}

	// 2. Min Level 追踪 (Fast Attack Down, Slow Recovery Up)
	// 如果当前样本低于底噪基准，立即更新（捕捉更深的底噪）
	// 否则，缓慢向 maxLevel 靠拢（适应底噪抬升或信号消失的情况）
	if sample < at.minLevel {
		at.minLevel = sample
	} else {
		// 使用互补系数缓慢抬升底噪基准
		// 逻辑：min 总是试图向上“漂浮”，直到碰到真实的底噪样本被压下去
		at.minLevel += (at.maxLevel - at.minLevel) * (1.0 - at.decayRate)
	}

	// 防止浮点漂移导致的异常交叉 (Safety Check)
	if at.minLevel > at.maxLevel {
		at.minLevel = at.maxLevel
	}

	// 3. 计算动态范围
	dynRange := at.maxLevel - at.minLevel

	// 4. 静噪逻辑 (Squelch)
	// 如果动态范围太小，说明没有有效信号（全是底噪），
	// 返回一组输入信号永远无法达到的阈值，强制输出 Space (Low)。
	if dynRange < at.minRange {
		// 返回 > 1.0 的阈值，确保 sample > high 永远为 false
		return 10.0, 9.0
	}

	// 5. 计算迟滞阈值 (Hysteresis)
	// 中点根据 min 和 range 浮动
	center := at.minLevel + (dynRange * 0.5)

	// 迟滞宽度设为动态范围的 5% (上下各 5%，总共 10% 的缓冲区)
	hysteresis := dynRange * 0.05

	high = center + hysteresis
	low = center - hysteresis

	return high, low
}
