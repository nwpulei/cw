package BeamDecoder

import (
	"fmt"
	"testing"
)

func TestNewCWDecoder(t *testing.T) {
	lm := NewLanguageModel()
	decoder := NewCWDecoder(DecoderConfig{
		InitialWPM:        20,   // 初始假设 20 WPM
		GlitchThresholdMs: 20.0, // 20ms 以下的空窗视为噪声并缝合
		UpdateAlpha:       0.25,
	},
		lm,
	)

	// 模拟输入序列：
	// 模拟发送 "A" (.-)
	// 正常应该是: On(60), Off(60), On(180)
	// 但我们加入噪声：把最后的长划(180) 打断成 On(100) + Off(10) + On(70)

	inputs := []struct {
		dur   float64
		state SignalState
	}{
		{60, StateOn},   // .
		{60, StateOff},  // gap
		{100, StateOn},  // - (前半段)
		{10, StateOff},  // NOISE GAP! (应该被缝合)
		{70, StateOn},   // - (后半段)
		{300, StateOff}, // Char Gap (结束)
	}

	fmt.Println("开始解码...")
	for _, in := range inputs {
		out := decoder.FeedNew(in.dur, in.state)
		if out != "" {
			fmt.Printf("输出: [%s]\n", out)
		}
	}
}

func TestLoadModel(t *testing.T) {
	model := NewLanguageModel()
	score := model.GetTransitionScore("0", "0")
	fmt.Printf("%f\r\n", score)
	score = model.GetTransitionScore("1", "V")
	fmt.Printf("%f\r\n", score)
}

func TestNewBeamDecoder(t *testing.T) {
	// 1. 初始化
	lm := NewLanguageModel() // 只有 Q->U 的概率很高
	decoder := NewBeamDecoder(lm)

	// ==========================================
	// 信号 1: 比较标准的 Q (--.-)
	// 结构: [划, 间, 划, 间, 点, 间, 划]
	// 标称: [3.0, 1.0, 3.0, 1.0, 1.0, 1.0, 3.0]
	// ==========================================
	signalQ := []float64{
		2.8, 1.1, // 划(稍短), 间
		3.1, 0.9, // 划(稍长), 间
		0.9, 1.0, // 点(稍短), 间
		2.9, // 划
	}

	// ==========================================
	// 信号 2: 严重畸变的 U (..-)
	// 结构: [点, 间, 点, 间, 划]
	// 标称: [1.0, 1.0, 1.0, 1.0, 3.0]
	//
	// 畸变模拟 (模拟之前的例子):
	// 第1个点: 1.2 (稍胖)
	// 第2个点: 2.8 (极其严重！像个划)
	// 第3个划: 1.1 (极其严重！像个点)
	//
	// 整体看起来像: . - . (R) 或者 - . . (D) 的变体
	// 如果不靠 Viterbi，这绝对会被判错。
	// ==========================================
	signalBadU := []float64{
		1.2, 0.8, // 点(胖), 间(短)
		2.8, 1.2, // 点(严重畸变像划), 间(长)
		1.1, // 划(严重畸变像点)
	}

	fmt.Println(">>> 收到信号 1 (带间隔的标准 Q)...")
	decoder.Step(signalQ)
	printPaths(decoder.paths)

	fmt.Println("\n>>> 收到信号 2 (带间隔的畸变 U)...")
	decoder.Step(signalBadU)
	printPaths(decoder.paths)

	fmt.Println("\n>>> 最终解码结果:")
	fmt.Println(decoder.GetResult())
}
func printPaths(paths []Path) {
	for i, p := range paths {
		fmt.Printf("#%d: [%s] Score: %.2f\n", i+1, p.Sentence, p.TotalScore)
	}
}

func TestNewIntegratedCWDecoder(t *testing.T) {
	lm := NewLanguageModel()
	cwDecoder := NewCWDecoder(DecoderConfig{
		InitialWPM:        20,   // 初始假设 20 WPM
		GlitchThresholdMs: 20.0, // 20ms 以下的空窗视为噪声并缝合
		UpdateAlpha:       0.25,
	},
		lm,
	)

	// 3. 模拟施密特触发器的输入流
	// 模拟发送 "CQ" ( --.-   --.- )
	// 假设 WPM=20 (1t = 60ms)
	// 信号流:
	// C: Dah(180), Gap(60), Dit(60), Gap(60), Dah(180), Gap(60), Dit(60) -> CharSpace(200)
	// Q: Dah(180), Gap(60), Dah(180), Gap(60), Dit(60), Gap(60), Dah(180) -> WordSpace(400)

	type Pulse struct {
		dur   float64
		state SignalState
	}

	inputData := []Pulse{
		// --- 字符 C ---
		{180, StateOn}, {61, StateOff}, // -
		{62, StateOn}, {61, StateOff}, // .
		{183, StateOn}, {61, StateOff}, // -
		{64, StateOn}, {201, StateOff}, // . + CharGap (触发 Step)

		// --- 字符 Q ---
		{180, StateOn}, {60, StateOff}, // -
		{180, StateOn}, {60, StateOff}, // -
		{62, StateOn}, {61, StateOff}, // .
		{180, StateOn}, {400, StateOff}, // - + WordGap (触发 Step + AddSpace)
		// - + WordGap (触发 Step + AddSpace)
	}

	fmt.Println("开始解码流...")

	for _, p := range inputData {
		// 喂数据
		cwDecoder.FeedNew(p.dur, p.state)

		// 实时打印当前的“最佳猜测”
		// 注意：BeamDecoder 是延迟决策的，所以结果可能会变（比如从 T 变成 Q）
		currentBest := cwDecoder.GetBestPath()
		fmt.Printf("\r当前解码: [%s]", currentBest)
	}
	out := cwDecoder.CheckTimeout()
	fmt.Printf("\r当前解码: [%s]", out)
	currentBest := cwDecoder.GetBestPath()
	fmt.Printf("\r当前解码: [%s]", currentBest)
	fmt.Println("\n完成。")
}

// TestInput 定义单个输入事件
type TestInput struct {
	Dur   float64
	State SignalState
}

// 辅助函数：根据 ".-" 字符串生成测试信号流
// wpm: 设定语速
// noise: 是否加入微量随机抖动 (可选)
func generateSignal(pattern string, wpm float64) []TestInput {
	unit := 1200.0 / wpm
	var inputs []TestInput

	for _, char := range pattern {
		switch char {
		case '.':
			inputs = append(inputs, TestInput{unit, StateOn})
			inputs = append(inputs, TestInput{unit, StateOff}) // 默认码元间隔
		case '-':
			inputs = append(inputs, TestInput{unit * 3.0, StateOn})
			inputs = append(inputs, TestInput{unit, StateOff})
		case ' ': // 字符间隔 (3t) - 前面已经有了1个码元间隔，补2个
			inputs = append(inputs, TestInput{unit * 2.0, StateOff})
		case '/': // 单词间隔 (7t) - 前面有了1个，补6个
			inputs = append(inputs, TestInput{unit * 6.0, StateOff})
		}
	}
	return inputs
}

func TestCWDecoder_Logic(t *testing.T) {
	// 1. 准备环境
	// 这是一个简单的 LM，只有 Q->U 的概率高，方便测试 Beam Search
	lm := NewLanguageModel()

	// 定义测试用例集
	tests := []struct {
		name           string
		cfg            DecoderConfig
		inputs         []TestInput
		expectedSuffix string // 我们期望最后解出的字符串包含这个后缀
		strictMode     bool   // 是否要求完全匹配
	}{
		// -------------------------------------------------------------------
		// Case 1: 标准信号测试 (Sanity Check)
		// -------------------------------------------------------------------
		{
			name: "Standard 'PARIS'",
			cfg:  DecoderConfig{InitialWPM: 20, GlitchThresholdMs: 20},
			// .--. .- .-. .. ...
			inputs:         generateSignal(".--. .- .-. .. ...", 20),
			expectedSuffix: "PARIS",
		},

		// -------------------------------------------------------------------
		// Case 2: 物理层 - 噪声过滤 (Glitch Removal)
		// 场景：在空窗期突然出现一个 10ms 的高电平毛刺
		// 预期：解码器应该完全忽略它，不输出 "E" 或 "T"
		// -------------------------------------------------------------------
		{
			name: "Noise Filtering",
			cfg:  DecoderConfig{InitialWPM: 20, GlitchThresholdMs: 20}, // 阈值 20ms
			inputs: []TestInput{
				{60, StateOn}, {61, StateOff}, // . (E)
				{10, StateOn}, {62, StateOff}, // 噪声! (应该被吃掉)
				{63, StateOn}, {180, StateOff}, // . (E) -> 结束
			},
			expectedSuffix: "I", // 中间的噪声不应变成E，所以是两个E
		},
		{
			name: "Noise Filtering",
			cfg:  DecoderConfig{InitialWPM: 20, GlitchThresholdMs: 20}, // 阈值 20ms
			inputs: []TestInput{
				{60, StateOn}, {61, StateOff}, // . (E)
				{10, StateOn}, {80, StateOff}, // 噪声! (应该被吃掉)
				{63, StateOn}, {180, StateOff}, // . (E) -> 结束
			},
			expectedSuffix: "EE", // 中间的噪声不应变成E，所以是两个E
		},

		// -------------------------------------------------------------------
		// Case 3: 物理层 - 信号缝合 (Gap Bridging)
		// 场景：一个长划 (Dah, 180ms) 被中间 10ms 的掉电打断
		// 原理：On(100) + Off(10) + On(70) = 180ms
		// 预期：应该被识别为一个划 (T)，而不是 ". ." (I) 或 ". E"
		// -------------------------------------------------------------------
		{
			name: "Signal Stitching (Broken Dah)",
			cfg:  DecoderConfig{InitialWPM: 20, GlitchThresholdMs: 20},
			inputs: []TestInput{
				{100, StateOn},
				{10, StateOff}, // 断裂！小于阈值，应该被缝合
				{70, StateOn},
				{300, StateOff}, // 字符结束
			},
			expectedSuffix: "T",
		},

		// -------------------------------------------------------------------
		// Case 4: 逻辑层 - 边界条件 (Boundary Conditions)
		// 场景：发送一个时长正好在 点(1t) 和 划(3t) 中间 (2t) 的信号
		// 预期：Beam Search 应该通过上下文来决定，而不是随机瞎猜
		// 我们发送 "Q (3,3,1,3)" 后面跟一个 "模糊的2.0t"
		// 如果是 Q U (..-)，最后一个应该是3t
		// 如果是 Q E (.)，最后一个应该是1t
		// 上下文 Q->U 概率大，所以这个 2.0t 应该被判定为划 (Dah) 的一部分?
		// 或者我们测试简单的：发送一个稍长的点 (1.6t)
		// -------------------------------------------------------------------
		{
			name: "Ambiguous Signal (Q -> U correction)",
			cfg:  DecoderConfig{InitialWPM: 20, GlitchThresholdMs: 20},
			inputs: func() []TestInput {
				// 先发一个标准的 Q (--.-)
				seq := generateSignal("--.- ", 20)
				// 再发 U (..-)，但是把最后的划(Dah)发得很短，只有 1.8倍unitTime
				// 这在几何距离上离 点(1.0) 和 划(3.0) 差不多
				// 但因为 Q 后面大概率是 U，Beam Search 应该把它拉回 划
				u_distorted := []TestInput{
					{60, StateOn}, {60, StateOff}, // .
					{60, StateOn}, {60, StateOff}, // .
					{110, StateOn}, {300, StateOff}, // 畸变的划 (110ms vs 60/180)
				}
				return append(seq, u_distorted...)
			}(),
			expectedSuffix: "QU",
		},

		// -------------------------------------------------------------------
		// Case 5: 单词间隔 (Word Spacing)
		// 场景：HELLO WORLD
		// 预期：中间必须有一个空格
		// -------------------------------------------------------------------
		{
			name:           "Word Spacing",
			cfg:            DecoderConfig{InitialWPM: 20, GlitchThresholdMs: 20},
			inputs:         generateSignal(".... . .-.. .-.. --- / .-- --- .-. .-.. -..", 20),
			expectedSuffix: "HELLO WORLD",
		},

		// -------------------------------------------------------------------
		// Case 6: 极速变化 (Speed Ramping)
		// 场景：从 20WPM 突然变成 30WPM
		// 预期：Decoder 的 WPM 跟踪应该能适应 (虽然可能前几个字会错，但后面要对)
		// -------------------------------------------------------------------
		{
			name: "Speed Ramp Up",
			cfg:  DecoderConfig{InitialWPM: 20, GlitchThresholdMs: 15, UpdateAlpha: 0.25},
			inputs: func() []TestInput {
				// 前面慢: T E S T (20 WPM)
				slow := generateSignal("- . ... - ", 20)
				// 后面快: T E S T (30 WPM)
				fast := generateSignal("- . ... -"+" - . ... -", 30)
				return append(slow, fast...)
			}(),
			// 注意：这里不做强匹配，因为变速瞬间可能解码错误，只要最后能稳住就行
			// 实际测试时可以打印出来观察
			expectedSuffix: "TEST",
		},
		// -------------------------------------------------------------------
		// Case 7: 字符粘连 (Run-together / Spacing Error)
		// 场景：句子 "LOOK AT IT"。
		// 陷阱："IT" (`.. -`) 如果中间间隔太短，会被听成 "U" (`..-`)。
		// 陷阱："AT" (`.- -`) 如果粘连，会被听成 "W" (`.--`)。
		// 我们故意把 "IT" 发得像 "U"。
		// 预期：因为上下文 "LOOK AT ...", 接 "IT" 的概率远大于 "U" (LOOK AT U? 也有可能，但 IT 更常见)
		// -------------------------------------------------------------------
		{
			name: "Run-together Correction (IT -> U)",
			cfg:  DecoderConfig{InitialWPM: 20, GlitchThresholdMs: 15},
			inputs: func() []TestInput {
				// 发送 "LOOK AT "
				prefix := generateSignal(".-.. --- --- -.- / .- - / ", 20)

				// 构造一个看起来像 U 的信号 (..-)
				// 标准 U: On, Off, On, Off, On(Long)
				// 我们原本想发 I(..) T(-)，中间间隔应该是 3t (180ms)
				// 但我们只给 1.5t (90ms)，处于模糊地带
				ambiguous := []TestInput{
					{60, StateOn}, {60, StateOff}, // .
					{60, StateOn}, {90, StateOff}, // . (Gap 只有90ms，像U的内部间隔)
					{180, StateOn}, {300, StateOff}, // -
				}
				return append(prefix, ambiguous...)
			}(),
			// 如果你的 Bigram 中 "AT IT" 概率高，或者 "LOOK AT IT" 常见，这里应该解出 IT
			// 如果 LM 没训练好，可能会解出 U。这是一个很好的调优测试。
			expectedSuffix: "IT",
		},

		// -------------------------------------------------------------------
		// Case 8: 信号占空比偏差 (Heavy Weighting / Fat Dots)
		// 场景：发报机的"点"太长了。标准点60ms，这里发90ms。
		// 标准划180ms，这里发200ms。间隔反而被压缩了。
		// 预期：基于距离的匹配算法应该能容忍这种整体性的偏移。
		// -------------------------------------------------------------------
		{
			name: "Heavy Weighting (Fat Dots)",
			cfg:  DecoderConfig{InitialWPM: 20, GlitchThresholdMs: 15},
			inputs: []TestInput{
				// 发送 "E I S H" (都是点，测试对点的容忍度)
				// 标准点 60ms。我们发 90ms。
				{90, StateOn}, {300, StateOff}, // E (长点)
				{90, StateOn}, {50, StateOff}, {90, StateOn}, {300, StateOff}, // I (胖I)
			},
			expectedSuffix: "EI",
		},

		// -------------------------------------------------------------------
		// Case 9: 丢点纠错 (Contextual Repair)
		// 场景：发送 "THE"，但是 H (`....`) 丢了一个点，变成了 S (`...`)。
		// 实际收到序列："T S E"。
		// 预期：Beam Search 发现 "TSE" 概率低，"THE" 概率高 (H和S只差一个点，Emission分数差距不大)，强行纠正为 THE。
		// -------------------------------------------------------------------
		{
			name: "Missing Dot Repair (THE -> TSE)",
			cfg:  DecoderConfig{InitialWPM: 20, GlitchThresholdMs: 15},
			inputs: func() []TestInput {
				// T (-)
				t := generateSignal("- ", 20)
				// S (...) -- 这是一个错误的 H
				s := generateSignal("... ", 20)
				// E (.)
				e := generateSignal(".", 20)

				// 拼接：T + S + E
				res := append(t, s...)
				return append(res, e...)
			}(),
			expectedSuffix: "THE", // 这里的关键看你的 LM 够不够强
		},

		// -------------------------------------------------------------------
		// Case 10: 极端快慢节奏 (Farnsworth)
		// 场景：字符本身是 30 WPM 的速度，但字符间隔像是 12 WPM 的速度。
		// 陷阱：解码器可能会把超长的字符间隔误判为“空格”。
		// -------------------------------------------------------------------
		{
			name: "Farnsworth Spacing",
			cfg:  DecoderConfig{InitialWPM: 30, GlitchThresholdMs: 10, UpdateAlpha: 0.25},
			inputs: func() []TestInput {
				// 30 WPM: unit = 40ms.
				// 正常 CharGap = 3t = 120ms. WordGap = 7t = 280ms.
				// 我们发送 "AB"，中间间隔 250ms (接近正常 WordGap，但其实是Farnsworth下的CharGap)

				// A (30 WPM)
				a := []TestInput{{40, StateOn}, {40, StateOff}, {120, StateOn}}

				// 超长间隔 250ms
				gap := []TestInput{{250, StateOff}}

				// B (30 WPM)
				b := []TestInput{{120, StateOn}, {40, StateOff}, {40, StateOn}, {40, StateOff}, {40, StateOn}}

				res := append(a, gap...)
				return append(res, b...)
			}(),
			// 期望输出 "AB" (连在一起)，而不是 "A B" (分开)
			// 这需要你的自适应 WPM 逻辑非常稳，或者 Beam Search 倾向于拼出双字母词
			expectedSuffix: "AD",
		},

		// -------------------------------------------------------------------
		// Case 11: 莫尔斯特殊符号 (Prosigns / Punctuation)
		// 场景：发送 "?" (..--..) 和 "/" (-..-.)
		// 预期：能够正确解码标点，且不会把 "?" 拆成 "I M I" (`.. -- ..`)
		// -------------------------------------------------------------------
		{
			name:           "Punctuation Handling",
			cfg:            DecoderConfig{InitialWPM: 20},
			inputs:         generateSignal("..--.. -..-.", 20),
			expectedSuffix: "?/",
		},
	}

	// 3. 执行循环
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewCWDecoder(tt.cfg, lm)
			var output string

			for _, in := range tt.inputs {
				// Feed 返回的是当前的 Best Path String
				// 我们只关心每一轮积累的结果，或者最后的输出
				res := decoder.FeedNew(in.Dur, in.State)
				if res != "" {
					output = res // Beam Decoder 通常返回完整句子，或者增量
				}
			}
			decoder.CheckTimeout()

			// 某些实现可能是流式返回增量，某些是返回全量
			// 这里假设 GetResult 返回的是全量 Sentence
			finalRes := decoder.beamDecoder.GetResult()

			fmt.Printf("Case [%s] Result: [%s] [%s]\n", tt.name, finalRes, output)

			if len(tt.expectedSuffix) > 0 {
				// 简单的后缀匹配检查
				if len(finalRes) < len(tt.expectedSuffix) ||
					finalRes[len(finalRes)-len(tt.expectedSuffix):] != tt.expectedSuffix {
					t.Errorf("期望结尾是 %q, 实际得到 %q", tt.expectedSuffix, finalRes)
				}
			}
		})
	}
}
