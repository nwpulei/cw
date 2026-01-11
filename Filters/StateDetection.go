package Filters

import "fmt"

/*
施密特触发器
判断当前的信号是mark 还是space

同时这里会把一些信号合并，祛除抖动

这里有个问题，需要比较好的阈值
*/
// StateTransition 代表一次完整的状态结束事件
// 例如：一个 MARK 刚刚结束，持续了 100ms
type StateTransition struct {
	FinishedState bool    // 刚刚结束的状态 (true=Mark/Signal, false=Space/Silence)
	DurationMs    float64 // 该状态持续了多少毫秒
}

// SchmittTrigger 结合了滞回比较器（施密特）和去抖动逻辑
type SchmittTrigger struct {
	// 配置参数
	thresholdHigh float64
	thresholdLow  float64
	sampleRate    float64
	debounceCount int64 // 需要多少个采样点确认去抖

	// 内部状态
	currentState     bool  // 当前稳定的状态
	totalSamples     int64 // 总采样计数
	stateStartSample int64 // 当前稳定状态的开始时间点

	// 去抖动临时状态
	pendingChange     bool
	changeStartSample int64
	thresholder       *AdaptiveThresholder
}

// NewSchmittTrigger 创建触发器
func NewSchmittTrigger(sampleRate float64, high, low, debounceMs float64) *SchmittTrigger {
	thresholder := NewAdaptiveThresholder(0.9995, 0.005)
	return &SchmittTrigger{
		sampleRate:    sampleRate,
		thresholdHigh: high,
		thresholdLow:  low,
		debounceCount: int64(debounceMs * sampleRate),
		currentState:  false, // 默认为静音
		thresholder:   thresholder,
	}
}

// Feed 输入一个包络样本，返回状态变化事件
// 如果没有发生状态切换（或者切换被去抖过滤了），返回 nil
func (st *SchmittTrigger) Feed(envelope float64) *StateTransition {
	st.totalSamples++

	h, l := st.thresholder.Update(envelope)
	//st.SetThresholds(h, l)
	if st.totalSamples%200000 == 0 {
		fmt.Printf("[DEBUG] hight%.1f,low,%.1f value%.2f\n", h, l, envelope)
	}

	// 1. 原始施密特逻辑 (Raw Schmitt Logic)
	rawSignal := st.currentState
	if st.currentState {
		// 如果当前是 High，只有低于 Low 才变 Low
		if envelope < st.thresholdLow {
			rawSignal = false
		}
	} else {
		// 如果当前是 Low，只有高于 High 才变 High
		if envelope > st.thresholdHigh {
			rawSignal = true
		}
	}

	// 2. 去抖动逻辑 (Debounce Logic)
	if rawSignal == st.currentState {
		// 信号稳定（或者回到了原状态），取消任何待定的变更
		st.pendingChange = false
		return nil
	}

	// 信号与当前状态不一致，且没有正在进行的变更检查 -> 开始计时
	if !st.pendingChange {
		st.pendingChange = true
		st.changeStartSample = st.totalSamples
		return nil
	}

	// 检查是否满足去抖时间
	pendingDuration := st.totalSamples - st.changeStartSample
	if pendingDuration > st.debounceCount {
		// --- 确认状态变更 ---

		// 计算上一个状态持续了多久 (从 stateStart 到 changeStart)
		// 注意：持续时间截止到 pending 开始的那一刻，而不是现在
		prevDurationSamples := st.changeStartSample - st.stateStartSample
		durationMs := (float64(prevDurationSamples) / st.sampleRate) * 1000.0

		// 记录刚刚结束的状态
		finishedState := st.currentState

		// 更新状态
		st.currentState = rawSignal
		st.stateStartSample = st.changeStartSample
		st.pendingChange = false

		return &StateTransition{
			FinishedState: finishedState,
			DurationMs:    durationMs,
		}
	}

	return nil
}

// GetCurrentState 获取当前稳定状态
func (st *SchmittTrigger) GetCurrentState() bool {
	return st.currentState
}

// SetThresholds 动态调整阈值
func (st *SchmittTrigger) SetThresholds(high, low float64) {
	st.thresholdHigh = high
	st.thresholdLow = low
}
