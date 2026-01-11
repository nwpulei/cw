package BeamDecoder

import (
	"sort"
)

// StandardPattern 定义标准字符的时长比例序列
// 例如 "A" (.-) => Signal(1), Gap(1), Signal(3)
// 注意：为了简化计算，我们只存 Signal 和 内部Gap 的比例
type StandardPattern struct {
	Char     string
	Sequence []float64 // 标准化的时长序列
}

var Patterns = []StandardPattern{
	// 字母（A-Z）
	{"A", []float64{1.0, 1.0, 3.0}},                     // . -
	{"B", []float64{3.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0}}, // - . . .
	{"C", []float64{3.0, 1.0, 1.0, 1.0, 3.0, 1.0, 1.0}}, // - . - .
	{"D", []float64{3.0, 1.0, 1.0, 1.0, 1.0}},           // - . .
	{"E", []float64{1.0}},                               // .
	{"F", []float64{1.0, 1.0, 1.0, 1.0, 3.0, 1.0, 1.0}}, // . . - .
	{"G", []float64{3.0, 1.0, 3.0, 1.0, 1.0}},           // - - .
	{"H", []float64{1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0}}, // . . . .
	{"I", []float64{1.0, 1.0, 1.0}},                     // . .
	{"J", []float64{1.0, 1.0, 3.0, 1.0, 3.0, 1.0, 3.0}}, // . - - -
	{"K", []float64{3.0, 1.0, 1.0, 1.0, 3.0}},           // - . -
	{"L", []float64{1.0, 1.0, 3.0, 1.0, 1.0, 1.0, 1.0}}, // . - . .
	{"M", []float64{3.0, 1.0, 3.0}},                     // - -
	{"N", []float64{3.0, 1.0, 1.0}},                     // - .
	{"O", []float64{3.0, 1.0, 3.0, 1.0, 3.0}},           // - - -
	{"P", []float64{1.0, 1.0, 3.0, 1.0, 3.0, 1.0, 1.0}}, // . - - .
	{"Q", []float64{3.0, 1.0, 3.0, 1.0, 1.0, 1.0, 3.0}}, // - - . -
	{"R", []float64{1.0, 1.0, 3.0, 1.0, 1.0}},           // . - .
	{"S", []float64{1.0, 1.0, 1.0, 1.0, 1.0}},           // . . .
	{"T", []float64{3.0}},                               // -
	{"U", []float64{1.0, 1.0, 1.0, 1.0, 3.0}},           // . . -
	{"V", []float64{1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 3.0}}, // . . . -
	{"W", []float64{1.0, 1.0, 3.0, 1.0, 3.0}},           // . - -
	{"X", []float64{3.0, 1.0, 1.0, 1.0, 1.0, 1.0, 3.0}}, // - . . -
	{"Y", []float64{3.0, 1.0, 1.0, 1.0, 3.0, 1.0, 3.0}}, // - . - -
	{"Z", []float64{3.0, 1.0, 3.0, 1.0, 1.0, 1.0, 1.0}}, // - - . .

	// 数字（0-9）
	{"0", []float64{3.0, 1.0, 3.0, 1.0, 3.0, 1.0, 3.0, 1.0, 3.0}}, // - - - - -
	{"1", []float64{1.0, 1.0, 3.0, 1.0, 3.0, 1.0, 3.0, 1.0, 3.0}}, // . - - - -
	{"2", []float64{1.0, 1.0, 1.0, 1.0, 3.0, 1.0, 3.0, 1.0, 3.0}}, // . . - - -
	{"3", []float64{1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 3.0, 1.0, 3.0}}, // . . . - -
	{"4", []float64{1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 3.0}}, // . . . . -
	{"5", []float64{1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0}}, // . . . . .
	{"6", []float64{3.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0}}, // - . . . .
	{"7", []float64{3.0, 1.0, 3.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0}}, // - - . . .
	{"8", []float64{3.0, 1.0, 3.0, 1.0, 3.0, 1.0, 1.0, 1.0, 1.0}}, // - - - . .
	{"9", []float64{3.0, 1.0, 3.0, 1.0, 3.0, 1.0, 3.0, 1.0, 1.0}}, // - - - - .

	// 常用标点及特殊符号
	{".", []float64{1.0, 1.0, 3.0, 1.0, 1.0, 1.0, 3.0, 1.0, 1.0, 1.0, 3.0}},           // . - . - . - 句号
	{",", []float64{3.0, 1.0, 3.0, 1.0, 1.0, 1.0, 1.0, 1.0, 3.0, 1.0, 3.0}},           // - - . . - - 逗号
	{"?", []float64{1.0, 1.0, 1.0, 1.0, 3.0, 1.0, 3.0, 1.0, 1.0, 1.0, 1.0}},           // . . - - . . 问号
	{"'", []float64{1.0, 1.0, 3.0, 1.0, 3.0, 1.0, 3.0, 1.0, 3.0, 1.0, 1.0}},           // . - - - - . 单引号
	{"!", []float64{3.0, 1.0, 1.0, 1.0, 3.0, 1.0, 1.0, 1.0, 3.0, 1.0, 3.0}},           // - . - . - - 感叹号
	{"/", []float64{3.0, 1.0, 1.0, 1.0, 1.0, 1.0, 3.0, 1.0, 1.0}},                     // - . . - .   斜杠
	{"(", []float64{3.0, 1.0, 1.0, 1.0, 3.0, 1.0, 3.0, 1.0, 1.0}},                     // - . - - .   左括号
	{")", []float64{3.0, 1.0, 1.0, 1.0, 3.0, 1.0, 3.0, 1.0, 1.0, 1.0, 3.0}},           // - . - - . - 右括号
	{"&", []float64{1.0, 1.0, 3.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0}},                     // . - . . .   与符号
	{":", []float64{3.0, 1.0, 3.0, 1.0, 3.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0}},           // - - - . . . 冒号
	{";", []float64{3.0, 1.0, 1.0, 1.0, 3.0, 1.0, 1.0, 1.0, 3.0, 1.0, 1.0}},           // - . - . - . 分号
	{"=", []float64{3.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 3.0}},                     // - . . . -   等号
	{"+", []float64{1.0, 1.0, 3.0, 1.0, 1.0, 1.0, 3.0, 1.0, 1.0}},                     // . - . - .   加号
	{"-", []float64{3.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 3.0}},           // - . . . . - 减号
	{"_", []float64{1.0, 1.0, 1.0, 1.0, 3.0, 1.0, 3.0, 1.0, 1.0, 1.0, 3.0}},           // . . - - . - 下划线
	{"\"", []float64{1.0, 1.0, 3.0, 1.0, 1.0, 1.0, 1.0, 1.0, 3.0, 1.0, 1.0}},          // . - . . - . 引号
	{"$", []float64{1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 3.0, 1.0, 1.0, 1.0, 1.0, 1.0, 3.0}}, // . . . - . . - 美元
	{"@", []float64{1.0, 1.0, 3.0, 1.0, 3.0, 1.0, 1.0, 1.0, 3.0, 1.0, 1.0}},           // . - - . - . AT符号
}

// Path 代表一条解码路径（一条时间线）
type Path struct {
	Sentence   string  // 这条路径解码出的完整句子
	LastChar   string  // 最后一个字符（用于查找转移概率）
	TotalScore float64 // 总得分 (Log Probability)
}

// BeamDecoder 维特比束搜索解码器
type BeamDecoder struct {
	lm        *LanguageModel
	beamWidth int    // 束宽 (K)，每一轮只保留前 K 个最优解
	paths     []Path // 当前活着的所有路径

	//  字符模板库 (你需要填充之前定义的 Patterns)
	patterns []StandardPattern

	statsAnalyzer *StatisticalAnalyzer // 新增
}

func NewBeamDecoder(lm *LanguageModel) *BeamDecoder {
	return &BeamDecoder{
		lm:            lm,
		beamWidth:     10,                                                    // 设为 5 到 10 即可，太大了没必要
		paths:         []Path{{Sentence: "", LastChar: "", TotalScore: 0.0}}, // 初始状态：空路径
		patterns:      Patterns,
		statsAnalyzer: NewAnalyzer(20), // 引用全局的 Patterns
	}
}

// Step 核心迭代：接收一个新的信号片段，更新所有路径
// inputSignal: 归一化后的时长序列，如 [1.0, 1.1, 3.2]
func (bd *BeamDecoder) Step(inputSignal []float64) {
	var candidates []Path
	currentStats := bd.statsAnalyzer.Analyze()
	// 如果统计还没准备好（比如刚开机），手动造一个默认值
	if !currentStats.Valid {
		currentStats = StatsResult{
			DitStats: SignalStats{StdDev: 0.2}, // 默认比较准
			DahStats: SignalStats{StdDev: 0.4}, // 默认划比较宽容
		}
	}
	// --- 1. 扩展 (Expansion) ---
	// 对于上一轮保留下来的每一条路径...
	for _, prevPath := range bd.paths {

		// 尝试每一个可能的字符 (A-Z, 0-9)
		for _, pattern := range bd.patterns {

			// A. 计算发射分 (长得像不像?)
			emitScore := CalculateEmissionScore_Advanced(inputSignal, pattern.Sequence, currentStats)

			// 性能优化：如果这一步这就已经极其不像了，直接跳过，没必要查表了
			if emitScore < -50.0 {
				continue
			}

			// B. 计算转移分 (接在这个词后面合不合理?)
			transScore := bd.lm.GetTransitionScore(prevPath.LastChar, pattern.Char)

			// C. 生成新候选路径
			newScore := prevPath.TotalScore + emitScore + transScore
			newPath := Path{
				Sentence:   prevPath.Sentence + pattern.Char,
				LastChar:   pattern.Char,
				TotalScore: newScore,
			}
			candidates = append(candidates, newPath)
		}
	}
	// --- 2. 状态保护 (Crash Prevention) ---
	// [修复]：如果没有任何候选路径生成（可能是噪声导致所有匹配分都低于阈值），
	// 不要清空 bd.paths，而是忽略本次输入，保留上一轮的状态。
	if len(candidates) == 0 {
		// 可以在这里打个日志方便调试
		// fmt.Println("Warning: Signal matched nothing, ignoring noise.")
		return
	}

	bd.paths = bd.PrunePaths(candidates)
}

// GetResult 获取当前最优解
func (bd *BeamDecoder) GetResult() string {
	if len(bd.paths) == 0 {
		return ""
	}
	return bd.paths[0].Sentence
}

// CalculateEmissionScore_Advanced
// signal: 实际收到的归一化时长序列 (e.g. [1.1, 0.9, 3.2])
// pattern: 字符的标准模板序列 (e.g. [1.0, 1.0, 3.0])
// stats: 统计分析器给出的当前环境下的点划特征
func CalculateEmissionScore_Advanced(signal []float64, pattern []float64, stats StatsResult) float64 {

	// 1. 长度硬校验
	if len(signal) != len(pattern) {
		return -1000.0 // 极大的惩罚
	}

	totalScore := 0.0

	// 2. 逐个元素比对
	for i := 0; i < len(signal); i++ {
		observed := signal[i]  // 实际值
		expected := pattern[i] // 理论值 (1.0 或 3.0)

		var sigma float64

		// 3. 动态选择方差 (Sigma)
		// 这里假设 pattern 里 1.0 代表点(或空), 3.0 代表划
		if expected > 2.0 {
			// 这是一个“划”
			sigma = stats.DahStats.StdDev
		} else {
			// 这是一个“点” (或者是内部间隔 Gap)
			sigma = stats.DitStats.StdDev
		}

		// --- 鲁棒性保护 (Safety Clamp) ---
		// 极其重要！防止 sigma 为 0 (导致除零panic) 或 sigma 过小 (导致得分负无穷)
		// 尤其是在刚开始没统计到足够数据时
		if sigma < 0.25 {
			sigma = 0.25
		}

		// 如果信号质量极差，sigma 可能会变得巨大，导致所有分数都接近 0，无法区分
		// 可以设置一个上限
		if sigma > 5.0 {
			sigma = 5.0
		}

		// 4. 计算高斯对数概率
		// Log(P) ≈ - (x - μ)^2 / (2 * σ^2)
		// 注意：这里的 μ (mean) 其实就是 observed 和 expected 的差值概念
		// 但因为我们已经把 signal 归一化了，所以 expected 就是 1.0 或 3.0
		// 而 observed 是 实际值 / unitTime

		diff := observed - expected
		termScore := -(diff * diff) / (2.0 * sigma * sigma)

		// 还可以加上对数正规化项 -log(σ)，但在比较大小时可以省略，
		// 除非你要比较不同长度的序列。为了严谨建议加上：
		// termScore -= math.Log(sigma)

		totalScore += termScore
	}

	return totalScore
}

// Config 定义剪枝的参数
const (
	MaxBeamWidth   = 20   // K值：每一轮最多保留多少条路径 (建议 20-50)
	PruneThreshold = 10.0 // 阈值：允许落后第一名多少分 (Log Probability)
	// 解释：e^-10 ≈ 0.000045。也就是说，如果某条路径的概率不到第一名的万分之四，就杀掉。
)

// PrunePaths 执行剪枝操作
// candidates: 刚刚扩展出来的所有候选路径
// 返回值: 下一轮存活的路径
func (bd *BeamDecoder) PrunePaths(candidates []Path) []Path {

	// --- 0. 边界检查 ---
	if len(candidates) == 0 {
		return candidates
	}

	// --- 1. 排序 (Ranking) ---
	// 按照 TotalScore 从大到小排序 (注意：Score 是负的 LogProb，越接近 0 越大)
	// Go 的 sort.Slice 在数据量小的时候非常快 (使用了 pdqsort)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].TotalScore > candidates[j].TotalScore
	})
	bestScore := candidates[0].TotalScore
	// 使用 map 来记录已经存在的结尾状态
	// Key 是 "LastChar" (对于 Bigram) 或者 "LastTwoChars" (对于 Trigram)
	seenStates := make(map[string]bool)

	survivors := make([]Path, 0, MaxBeamWidth)

	for _, path := range candidates {
		// 1. 硬限额检查
		if len(survivors) >= MaxBeamWidth {
			break
		}

		// 2. 阈值检查
		if path.TotalScore < (bestScore - PruneThreshold) {
			break
		}

		// 3. [新增] 状态去重
		// 如果我们已经保留了一条以 'Q' 结尾的路径，且它的分比当前这条高(肯定的，因为排过序了)
		// 那么当前这条以 'Q' 结尾的更差路径就没必要留了。
		if seenStates[path.LastChar] {
			continue // 跳过，相当于合并了
		}

		seenStates[path.LastChar] = true
		survivors = append(survivors, path)
	}
	return survivors
}

// InjectSpace 强制插入单词间隔
// 逻辑：将当前所有路径延伸出一个 " " (空格)
func (bd *BeamDecoder) InjectSpace() {
	var newPaths []Path

	for _, p := range bd.paths {
		// 1. 防抖：如果这条路径已经是空格结尾了，就不要再加空格了
		// 防止超长静音导致输出 "HELLO     WORLD"
		if len(p.Sentence) > 0 && p.Sentence[len(p.Sentence)-1] == ' ' {
			newPaths = append(newPaths, p) // 保持原样
			continue
		}

		// 2. 如果路径是空的，也不加空格（防止开头出现空格）
		if p.Sentence == "" {
			newPaths = append(newPaths, p)
			continue
		}

		// 3. 计算转移分 P(Space | LastChar)
		// [重要]：你的 bigrams.json 必须包含 " " 键，或者在 LM 里对空格做特殊处理
		transScore := bd.lm.GetTransitionScore(p.LastChar, " ")

		// 4. 生成新路径
		newPath := Path{
			Sentence:   p.Sentence + " ", // 追加空格
			LastChar:   " ",              // 更新 LastChar，以便下一个字母计算 P(Char | Space)
			TotalScore: p.TotalScore + transScore,
		}
		newPaths = append(newPaths, newPath)
	}

	// 5. 再次剪枝 (虽然通常不需要，但为了保持 BeamWidth 恒定，建议做一下)
	bd.paths = bd.PrunePaths(newPaths)
}
