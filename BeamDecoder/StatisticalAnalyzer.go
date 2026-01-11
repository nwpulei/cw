package BeamDecoder

import (
	"math"
	"sort"
)

/*
StatisticalAnalyzer 主要用于 自适应地分析摩尔斯电码信号的时长统计特征，以动态计算区分“点”（Dit）和“划”（Dah）的最佳阈值。
在摩尔斯电码解码中，发送速度可能会变化（例如手动发报时快时慢），因此不能使用固定的时间长度来判断一个信号是点还是划。
这个类通过统计最近一段时间内的信号时长，利用聚类思想找到点和划的自然分界线。以下是该类的详细功能拆解：
1. 核心目标
它的核心目标是解决 “多长的信号算点，多长的信号算划？” 这个问题。它不依赖预设的固定速度（WPM），而是根据实际接收到的数据分布来判断。
2. 主要工作流程
	数据收集 (Sliding Window):通过 AddObservation(duration float64) 方法，它维护一个固定大小（windowSize）的循环缓冲区 (history)。
这意味它只分析最近接收到的 N 个信号，从而能够适应发报速度的变化。
	寻找最佳切分点 (Gap Analysis):在 GetOptimalThreshold 和 Analyze 方法中，它将缓冲区内的时长数据复制并 排序。它假设数据会自然聚类成两堆：一堆较短的（点）和一堆较长的（划）。
它遍历排序后的数据（避开首尾极端值），寻找相邻数据间 最大的跳变（Max Gap）。这个跳变的位置就是点和划的天然分界线。
统计与置信度计算:一旦找到分界线，它将数据切分为 dits（点集）和 dahs（划集）。
使用 calculateStats 计算这两组数据的 均值 (Mean) 和 标准差 (StdDev)。
置信度 (Confidence)：通过计算变异系数（CV），判断信号的稳定性。如果点和划的时长非常稳定（标准差小），置信度就高；如果时长忽长忽短，置信度就低。
3. 关键结构体与方法•StatisticalAnalyzer:
主类，维护历史数据窗口。
SignalStats: 存储统计结果（均值、标准差、样本数）。
StatsResult: Analyze 方法的返回结果，包含：
OptimalThreshold: 区分点划的最佳时长阈值（通常是点集最大值和划集最小值的中间值）。•DitStats / DahStats: 点和划的具体统计信息。•Confidence: 0.0 到 1.0 的数值，表示分析结果的可信程度。
4. 代码逻辑亮点
抗干扰: 在寻找最大断层时，代码特意跳过了前 20%-25% 和后 20%-25% 的数据 (startIndex, endIndex)，这是为了防止极端的噪声数据（例如极短的毛刺或极长的干扰）影响分界线的判断。
最小断层保护: 代码中有 if maxGap < 30.0 或 20.0 的检查。如果最大断层太小，说明所有信号时长都差不多（可能全是点，或全是划，或者信号质量极差），此时它会返回无效结果，避免强行切分。
总结这个类是一个 基于统计学的自适应信号分类器。它将一维的时长数据进行二分类聚类（类似于一维的 K-Means 或 Jenks Natural Breaks），为解码器提供动态的、鲁棒的判定标准。
*/
// StatisticalAnalyzer (保持之前的结构，增加方法)
type StatisticalAnalyzer struct {
	windowSize int
	history    []float64
	cursor     int
	full       bool
}

// SignalStats 存储点或划的统计特征
type SignalStats struct {
	Mean   float64 // 平均值 (期望)
	StdDev float64 // 标准差 (离散度)
	Count  int     // 样本数量
}

// StatsResult 完整的分析结果
type StatsResult struct {
	OptimalThreshold float64     // 最佳切分阈值
	DitStats         SignalStats // 点的统计
	DahStats         SignalStats // 划的统计
	Confidence       float64     // 整体置信度 (0.0 - 1.0)
	Valid            bool        // 是否有效
}

func NewAnalyzer(size int) *StatisticalAnalyzer {
	return &StatisticalAnalyzer{
		windowSize: size,
		history:    make([]float64, size),
	}
}

// AddObservation 添加一个新的信号时长样本
func (s *StatisticalAnalyzer) AddObservation(duration float64) {
	s.history[s.cursor] = duration
	s.cursor = (s.cursor + 1) % s.windowSize
	if s.cursor == 0 {
		s.full = true
	}
}

// GetOptimalThreshold 计算最佳分割阈值
// 如果数据不足或分布极差，返回 -1 (表示建议回退到默认算法)
func (s *StatisticalAnalyzer) GetOptimalThreshold() float64 {
	if !s.full {
		return -1 // 数据太少，没法统计
	}

	// 1. 复制并排序，方便找分布
	data := make([]float64, s.windowSize)
	copy(data, s.history)
	sort.Float64s(data)

	// 2. 寻找最大断层 (Jenks Natural Breaks 的简化版)
	// 我们假设数据必然分为两堆（点和划）。
	// 那么，这两堆之间最大的那个跳变（Gap），就是最佳分割点。

	maxGap := 0.0
	splitIndex := -1

	// 优化：不要在首尾搜索，因为我们假设点划都有。
	// 从 20% 到 80% 的区域搜索，防止极值干扰。
	startIndex := int(float64(s.windowSize) * 0.2)
	endIndex := int(float64(s.windowSize) * 0.8)

	for i := startIndex; i < endIndex; i++ {
		gap := data[i+1] - data[i]
		if gap > maxGap {
			maxGap = gap
			splitIndex = i
		}
	}

	// 3. 验证统计显著性
	// 如果最大 Gap 很小（比如小于 20ms），说明只有一种信号（全也是点，或者全是划）
	// 这时候统计学失效，不能强行切分。
	if maxGap < 30.0 { // 30ms 是个经验值
		return -1
	}

	// 4. 返回断层中间的值作为阈值
	return (data[splitIndex] + data[splitIndex+1]) / 2.0
}

// Analyze 执行完整的统计分析
func (s *StatisticalAnalyzer) Analyze() StatsResult {
	if !s.full {
		return StatsResult{Valid: false}
	}

	// 1. 准备数据
	data := make([]float64, s.windowSize)
	copy(data, s.history)
	sort.Float64s(data)

	// 2. 寻找最佳切分点 (Max Gap)
	maxGap := 0.0
	splitIndex := -1
	startIndex := int(float64(s.windowSize) * 0.25) // 稍微收缩搜索范围，避开极端值
	endIndex := int(float64(s.windowSize) * 0.75)

	for i := startIndex; i < endIndex; i++ {
		gap := data[i+1] - data[i]
		if gap > maxGap {
			maxGap = gap
			splitIndex = i
		}
	}

	// 如果断层太小，说明混在一起了，统计失效
	if maxGap < 20.0 {
		return StatsResult{Valid: false}
	}

	// 3. 计算两堆数据的统计特征
	dits := data[:splitIndex+1]
	dahs := data[splitIndex+1:]

	ditStats := calculateStats(dits)
	dahStats := calculateStats(dahs)

	// 4. 计算置信度
	// 简单算法：Gap 越大，StandardDeviation 越小，置信度越高
	// 变异系数 CV = Sigma / Mean
	avgCV := (ditStats.StdDev/ditStats.Mean + dahStats.StdDev/dahStats.Mean) / 2.0
	confidence := 1.0 - avgCV // CV 越小，置信度越高
	if confidence < 0 {
		confidence = 0.1
	}

	return StatsResult{
		OptimalThreshold: (dits[len(dits)-1] + dahs[0]) / 2.0,
		DitStats:         ditStats,
		DahStats:         dahStats,
		Confidence:       confidence,
		Valid:            true,
	}
}

// 辅助函数：计算均值和标准差
func calculateStats(data []float64) SignalStats {
	if len(data) == 0 {
		return SignalStats{}
	}
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	mean := sum / float64(len(data))

	varianceSum := 0.0
	for _, v := range data {
		varianceSum += math.Pow(v-mean, 2)
	}
	stdDev := math.Sqrt(varianceSum / float64(len(data)))

	return SignalStats{Mean: mean, StdDev: stdDev, Count: len(data)}
}
