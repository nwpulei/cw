package BeamDecoder

import (
	"fmt"
	"math"
)

// 定义信号状态
type SignalState int

const (
	StateOn  SignalState = 1 // 信号 (Mark)
	StateOff SignalState = 0 // 空窗 (Space)
)

// DecoderConfig 配置参数
type DecoderConfig struct {
	InitialWPM        float64 // 初始猜测速度，推荐 20
	GlitchThresholdMs float64 // 缝合阈值：小于此值的空窗会被忽略并缝合信号 (推荐 15-30ms)
	UpdateAlpha       float64 // EMA 平滑因子 (推荐 0.25)
}

// CWDecoder 解码器核心结构
type CWDecoder struct {
	cfg      DecoderConfig
	unitTime float64 // 当前基准短点时长 (1t)

	// 状态缓冲，用于处理"缝合"逻辑
	pendingMarkDuration float64
	lastGapDuration     float64

	// 结果缓冲
	charBuffer string

	statsAnalyzer *StatisticalAnalyzer // 新增
	// --- 后端引擎 ---
	beamDecoder *BeamDecoder
	// --- 信号缓冲 (Staging Area) ---
	// 用来存当前正在接收的字符序列，例如 [点, 间隔, 划] 的时长
	pulseBuffer []float64
}

// NewCWDecoder 初始化
func NewCWDecoder(cfg DecoderConfig, lm *LanguageModel) *CWDecoder {
	// 标准莫尔斯电码计算：WPM = 1200 / unitTime(ms)
	// 所以 unitTime = 1200 / WPM
	initialUnit := 1200.0 / cfg.InitialWPM

	return &CWDecoder{
		cfg:           cfg,
		unitTime:      initialUnit,
		statsAnalyzer: NewAnalyzer(10),
		beamDecoder:   NewBeamDecoder(lm),
		pulseBuffer:   make([]float64, 0, 8), // 预分配，一般字符不超过8段
	}
}

// Feed 接收来自施密特触发器的序列
// durationMs: 持续时长
// state: StateOn 或 StateOff
// 返回: 解码出的字符 (如果没有则返回空字符串 "")
func (d *CWDecoder) FeedNew(durationMs float64, state SignalState) string {
	//fmt.Printf("[feedNew] %.1f  %d\n", durationMs, state)
	// 第一层：噪声缝合 (保留原有的抗噪逻辑)
	// --- 1. 噪声过滤与信号缝合 (Noise Stitching) ---
	if state == StateOff {
		// 收到空窗：先不处理，暂存起来，看看是不是只是个短毛刺
		d.lastGapDuration += durationMs
		return ""
	}

	// 现在的 state == StateOn
	// 检查上一个 Gap 是否非常短（毛刺/断裂）
	if d.lastGapDuration > 0 && d.lastGapDuration < d.cfg.GlitchThresholdMs {
		// on ->off-> on 合并为一个on
		// 【缝合核心】：上个空窗太短了，被视为噪声！
		// 操作：把“之前的Mark” + “短空窗” + “现在的Mark” 合并成一个大信号
		d.pendingMarkDuration += d.lastGapDuration + durationMs
		d.lastGapDuration = 0 // 消费掉了
		return ""             // 继续等待信号结束
	}

	// 当我们收到 StateOn 时，才去结算上一个 Gap 和之前的 Mark
	// 第二层：处理上一个完整的动作
	// 1. 如果有之前的 Mark 还没处理，先入库
	if d.pendingMarkDuration > 0 {
		// >>> [新增逻辑] 信号去抖 (Mark Filtering) <<<
		// 只有当信号时长超过阈值时，才被视为有效信号。
		// 否则它就是一个高电平毛刺 (Spike)，直接丢弃，不污染 Buffer。
		if d.pendingMarkDuration > d.cfg.GlitchThresholdMs {
			// 有效信号，更新 WPM 并入库
			d.updateWPM1(d.pendingMarkDuration)
			d.AddCode(d.pendingMarkDuration)
		} else {
			// [修改逻辑] 这是一个噪声! (e.g. 10ms)
			// 我们不仅要丢弃它，还要把"它前后的空窗"连起来。
			noiseDur := d.pendingMarkDuration
			// 1. 尝试从 Buffer 末尾回溯上一个 Gap
			// Buffer 结构预期是 [Mark, Gap, Mark, Gap...]
			// 所以如果 Buffer 非空，最后一个元素一定是 Gap
			if len(d.pulseBuffer) > 0 {
				prevGap := d.delCode()
				// 3. 合并时长：前Gap + 噪声 + 后Gap (即当前的 d.lastGapDuration)
				// 这样 d.lastGapDuration 就变成了真实的物理静音时长
				d.lastGapDuration += prevGap + noiseDur
				// fmt.Printf("Noise merged! New Gap: %.1f\n", d.lastGapDuration)
			} else {
				// 如果 Buffer 是空的，说明噪声出现在字符开头
				// 直接把噪声时长加到当前的 Gap 里即可
				d.lastGapDuration += noiseDur
			}
		}
	}

	// 2. 检查上一个 Gap 是什么性质？(字符内间隔 vs 字符间间隔)
	// 阈值通常设为 2.5 * unitTime
	if d.lastGapDuration > d.unitTime*2.5 {
		// >>> 触发 Beam Search !!! <<<
		// 发现了一个足够长的空窗，说明 pulseBuffer 里已经攒够了一个完整的字符

		if len(d.pulseBuffer) > 0 {
			d.beamDecoder.Step(d.pulseBuffer)
			d.pulseBuffer = d.pulseBuffer[:0] // reset
		}
		// D. 处理空格 (Word Space)
		// 如果空窗特别长 (比如 > 5.0t)，说明是单词间隔
		if d.lastGapDuration > d.unitTime*5.0 {
			// 可以在这里强制 BeamDecoder 提交单词，或者插入一个空格
			d.beamDecoder.InjectSpace()
		}
	} else if d.lastGapDuration > 0 {
		// 这是一个短 Gap (点划之间的间隔)，也要存进去！
		// 因为我们的 Pattern 模板是包含间隔的 [Mark, Gap, Mark...]
		if len(d.pulseBuffer) > 0 {
			d.AddCode(d.lastGapDuration)
		}
	}
	// ==========================================
	// 第三层：更新当前状态
	// ==========================================
	// 更新缓存，准备下一轮
	d.pendingMarkDuration = durationMs
	d.lastGapDuration = 0 // 重置

	return d.beamDecoder.GetResult()
}

func (d *CWDecoder) AddCode(dur float64) {
	//fmt.Printf("code %.1f\r\n", dur/d.unitTime)
	d.pulseBuffer = append(d.pulseBuffer, dur/d.unitTime)
}
func (d *CWDecoder) delCode() float64 {
	lastIndex := len(d.pulseBuffer) - 1
	prevGap := d.pulseBuffer[lastIndex]
	d.pulseBuffer = d.pulseBuffer[:lastIndex]
	fmt.Printf("code del\r\n")
	return prevGap * d.unitTime
}

// --- 2. 自适应分类与速度跟踪 (Adaptive Logic) ---

func (d *CWDecoder) getThreshold(dur float64) (float64, float64) {
	// 1. 喂数据
	d.statsAnalyzer.AddObservation(dur)

	// 2. 获取高阶统计结果
	stats := d.statsAnalyzer.Analyze()

	var threshold float64
	var currentAlpha float64

	// 默认参数
	defaultThreshold := d.unitTime * 2.2
	baseAlpha := d.cfg.UpdateAlpha // 比如 0.25

	if stats.Valid {
		// --- 策略 A: 动态阈值 ---
		threshold = stats.OptimalThreshold

		// --- 策略 B: 动态学习率 (Dynamic Alpha) ---
		// 如果置信度高（比如 0.9），说明信号极其稳定，Alpha 可以大一点（比如 1.2倍），跟得紧一点
		// 如果置信度低（比如 0.4），说明乱得很，Alpha 就要降下来，靠历史惯性滑行
		currentAlpha = baseAlpha * stats.Confidence

		//fmt.Printf("threshold %.1f %.1f %.1f %.1f \r\n", threshold, baseAlpha, stats.Confidence, currentAlpha)
		// 钳位保护
		if currentAlpha < 0.05 {
			currentAlpha = 0.05
		}
		if currentAlpha > 0.5 {
			currentAlpha = 0.5
		}

		// --- 策略 C: 模糊区检测 (Confusion Zone) ---
		// 阈值附近的 "无人区"
		distToThreshold := math.Abs(dur - threshold)
		// 如果距离阈值太近（小于 1个标准差），说明这个信号很模糊
		// 这里我们可以做一个标记，虽然目前还是得强行判一个，但你可以打印警告
		uncertaintyLimit := (stats.DitStats.StdDev + stats.DahStats.StdDev) / 2.0
		if distToThreshold < uncertaintyLimit {
			// fmt.Printf("⚠️ 模糊信号: %.1f ms (阈值 %.1f)\n", dur, threshold)
			// 高级玩法：这里可以返回 "?" 让上层逻辑去猜
		}

	} else {
		// 统计失效，退回原始算法
		threshold = defaultThreshold
		currentAlpha = baseAlpha
	}
	return threshold, currentAlpha
}

// 简单的 WPM 更新逻辑 (EMA)
func (d *CWDecoder) updateWPM1(dur float64) {
	threshold, currentAlpha := d.getThreshold(dur)

	var sampleUnit float64
	if dur > threshold {
		sampleUnit = dur / 3.0 // 划是 3t，还原回 1t
	} else {
		sampleUnit = dur // 点是 1t
	}
	// 简单的估算：如果是点(1t附近)或划(3t附近)，就更新 unitTime
	// 这里可以使用之前 Level 1 写过的 updateUnitTime 逻辑
	// ...

	// --- 3. 安全更新基准速度 (Outlier Rejection) ---
	// 只有当样本在合理范围内（0.5倍到1.5倍当前速度）才更新
	// 防止极长或极短的噪声带偏解码器
	// 4. 更新 unitTime (使用动态 Alpha)
	// 依然保留异常值剔除保护
	if sampleUnit > d.unitTime*0.5 && sampleUnit < d.unitTime*1.5 {
		d.unitTime = currentAlpha*sampleUnit + (1.0-currentAlpha)*d.unitTime
		//fmt.Printf("\033[s\033[H\033[11B DEBUG: Sample=%.1f ms, New UnitTime=%.1f ms (%.1f WPM)\033[u \n", sampleUnit, d.unitTime, 1200.0/d.unitTime)
	} else {
		// 记录日志：发现异常样本，虽然用于了解码，但不用于更新速度
		//fmt.Printf("\u001B[s\u001B[H\u001B[11B DEBUG: Sample=%.1f ms, New UnitTime=%.1f ms (%.1f WPM)\u001B[u \n", sampleUnit, d.unitTime, 1200.0/d.unitTime)
	}
	if d.unitTime < 10.0 {
		fmt.Printf("ERROR: 时间间隔小于10，有问题。先强制到60。Sample=%.1f ms, New UnitTime=%.1f ms (%.1f WPM)\n", sampleUnit, d.unitTime, 1200.0/d.unitTime)
		d.unitTime = 60.0 // 默认 20 WPM
	}
	// 调试日志：你可以打开这个看它如何自适应速度
	//fmt.Printf("DEBUG: Sample=%.1f ms, New UnitTime=%.1f ms (%.1f WPM)\n", sampleUnit, d.unitTime, 1200.0/d.unitTime)
}

// --- 辅助逻辑 ---

func (d *CWDecoder) addToBuffer(s string) {
	d.charBuffer += s
}

// 在 beam_decoder.go 中添加

// AddSpace 强制在所有路径末尾添加空格 (当 CWDecoder 检测到 >5.0t 的停顿时间时调用)
func (bd *BeamDecoder) AddSpace() {
	for i := range bd.paths {
		// 只有当最后一个字符不是空格时才加，避免重复空格
		if len(bd.paths[i].Sentence) > 0 && bd.paths[i].Sentence[len(bd.paths[i].Sentence)-1] != ' ' {
			bd.paths[i].Sentence += " "
			// 空格通常不影响分数，或者给予微小的奖励/惩罚，这里保持分数不变
			bd.paths[i].LastChar = " " // 标记一下，虽然 LanguageModel 可能没有 " " 的键
		}
	}
}

// GetBestPath 返回当前分数最高的路径字符串
func (bd *BeamDecoder) GetBestPath() string {
	if len(bd.paths) == 0 {
		return ""
	}
	return bd.paths[0].Sentence
}

// GetBestPath 返回当前分数最高的路径字符串
func (d *CWDecoder) GetBestPath() string {
	return d.beamDecoder.GetBestPath()
}

// 在 CWDecoder 中增加这个方法
func (d *CWDecoder) CheckTimeout() string {
	// 假设我们定义超时时间为 5倍单位时长 (即单词间隔)
	//timeout := d.unitTime * 5.0

	// 如果手里有存货，且上次 Gap 已经持续很久了
	// 注意：这需要 Feed 函数配合，记录最后一次 Feed 的时间戳
	// 这里为了简单，我们假设外部调用这个函数意味着"已经静默很久了"
	if d.pendingMarkDuration > 0 {
		// 1. 把扣押的 Mark 放入 Buffer
		d.AddCode(d.pendingMarkDuration)
		d.pendingMarkDuration = 0

		// 2. 强行触发解码
		if len(d.pulseBuffer) > 0 {
			d.beamDecoder.Step(d.pulseBuffer) // 喂给 Beam
			d.pulseBuffer = d.pulseBuffer[:0] // 清空
			return d.beamDecoder.GetResult()
		}
	}
	return ""
}
