package cw

import "math"

// BiquadFilter 表示一个二阶 IIR 滤波器节
// 用于级联实现高阶滤波器
type BiquadFilter struct {
	// 系数
	a0, a1, a2, b1, b2 float64
	// 状态 (延迟线)
	z1, z2 float64
}

// Process 处理单个采样点
func (f *BiquadFilter) Process(in float64) float64 {
	out := in*f.a0 + f.z1
	f.z1 = in*f.a1 - out*f.b1 + f.z2
	f.z2 = in*f.a2 - out*f.b2
	return out
}

// ButterworthFilter 表示一个由多个 Biquad 节级联组成的巴特沃斯滤波器
type ButterworthFilter struct {
	sections []*BiquadFilter
}

// NewButterworthLowpass 创建一个新的 N 阶巴特沃斯低通滤波器
// order: 滤波器阶数 (必须是偶数)
// sampleRate: 采样率 (Hz)
// cutoffFreq: 截止频率 (Hz)
func NewButterworthLowpass(order int, sampleRate, cutoffFreq float64) *ButterworthFilter {
	if order%2 != 0 {
		panic("Butterworth filter order must be even")
	}

	// 限制截止频率以防止 Nyquist 频率附近的数值不稳定
	// 如果 cutoffFreq 接近 sampleRate/2，math.Tan 会趋向无穷大
	if cutoffFreq >= sampleRate*0.499 {
		cutoffFreq = sampleRate * 0.499
	}

	sections := make([]*BiquadFilter, order/2)

	// 使用双线性变换从模拟原型计算数字滤波器系数
	// 1. 预畸变截止频率
	w := 2.0 * sampleRate * math.Tan(math.Pi*cutoffFreq/sampleRate)

	// 2. 计算每个二阶节的系数
	for i := 0; i < order/2; i++ {
		// 级联顺序优化：将 Q 值较低的节放在前面 (Low Q -> High Q)
		// 原来的顺序是 i=0 (High Q) -> i=order/2-1 (Low Q)
		// 我们使用倒序索引计算极点
		poleIdx := (order/2 - 1) - i

		// 极点角度
		theta := math.Pi * (2.0*float64(poleIdx) + 1.0) / (2.0 * float64(order))

		// 模拟原型极点
		p_re := -w * math.Sin(theta)
		p_im := w * math.Cos(theta)

		// 双线性变换
		// alpha corresponds to the z^0 coefficient of the denominator: K^2 - 2*K*p_re + |p|^2
		// Since p_re is negative, -2*K*p_re is positive.
		alpha := 4.0*sampleRate*sampleRate - 4.0*sampleRate*p_re + p_re*p_re + p_im*p_im

		b1 := (-8.0*sampleRate*sampleRate + 2.0*(p_re*p_re+p_im*p_im)) / alpha
		// b2 corresponds to the z^-2 coefficient of the denominator: K^2 + 2*K*p_re + |p|^2
		b2 := (4.0*sampleRate*sampleRate + 4.0*sampleRate*p_re + p_re*p_re + p_im*p_im) / alpha

		a0 := (w * w) / alpha
		a1 := (2.0 * w * w) / alpha
		a2 := (w * w) / alpha

		sections[i] = &BiquadFilter{
			a0: a0, a1: a1, a2: a2,
			b1: b1, b2: b2,
		}
	}

	return &ButterworthFilter{sections: sections}
}

// Process 处理单个采样点，通过所有级联节
func (f *ButterworthFilter) Process(in float64) float64 {
	out := in
	for _, s := range f.sections {
		out = s.Process(out)
	}
	return out
}
