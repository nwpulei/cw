package main

import (
	"cw"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

// ============================================================================
// 1. 核心接口定义 (Core Interfaces)
// ============================================================================

// Decoder 是你的解码器需要实现的接口
// 在基准测试中，我们将模拟音频数据喂给 ProcessAudioChunk
type Decoder interface {
	// ProcessAudioChunk 处理一段音频数据
	ProcessAudioChunk(samples []float32)
	// GetDecodedText 返回当前缓冲区内的解码结果
	GetDecodedText() string
	// Reset 重置解码器状态
	Reset()
}

// ============================================================================
// 2. 摩尔斯电码音频生成器 (Audio Synthesizer)
// ============================================================================

type AudioConfig struct {
	WPM        float64 // Words Per Minute
	SampleRate int     // e.g., 48000
	Frequency  float64 // Tone frequency, e.g., 700Hz
	JitterPct  float64 // 0.0 to 1.0 (模拟手键误差)
}

type AudioGenerator struct {
	Config AudioConfig
	morse  map[rune]string
}

func NewAudioGenerator(cfg AudioConfig) *AudioGenerator {
	return &AudioGenerator{
		Config: cfg,
		morse:  getMorseTable(),
	}
}

// GenerateFromText 生成带包络的纯净 CW 音频
func (g *AudioGenerator) GenerateFromText(text string) []float32 {
	// 基础时序计算 (Paris standard: 50 dots = 1 word)
	// Dot duration (seconds) = 1.2 / WPM
	dotLen := 1.2 / g.Config.WPM
	sampleRate := float64(g.Config.SampleRate)

	var buffer []float32

	// 包络设置 (5ms 上升/下降沿，避免 Click 声)
	rampTime := 0.005
	rampSamples := int(rampTime * sampleRate)

	// 辅助函数：生成静音
	appendSilence := func(duration float64) {
		numSamples := int(duration * sampleRate)
		buffer = append(buffer, make([]float32, numSamples)...)
	}

	// 辅助函数：生成音频 (带梯形包络)
	appendTone := func(duration float64) {
		// 应用 Jitter (随机抖动)
		if g.Config.JitterPct > 0 {
			variance := (rand.Float64()*2 - 1) * g.Config.JitterPct // -pct to +pct
			duration = duration * (1 + variance)
		}

		numSamples := int(duration * sampleRate)
		offset := len(buffer)
		chunk := make([]float32, numSamples)

		omega := 2.0 * math.Pi * g.Config.Frequency / sampleRate

		for i := 0; i < numSamples; i++ {
			t := float64(i)
			val := float32(math.Sin(omega * t))

			// 应用包络 (Attack & Release)
			envelope := 1.0
			if i < rampSamples {
				envelope = float64(i) / float64(rampSamples)
			} else if i >= numSamples-rampSamples {
				envelope = float64(numSamples-1-i) / float64(rampSamples)
			}

			chunk[i] = val * float32(envelope)
		}
		buffer = append(buffer, chunk...)
		_ = offset
	}

	upperText := strings.ToUpper(text)
	for _, char := range upperText {
		if char == ' ' {
			appendSilence(dotLen * 7) // Word gap
			continue
		}

		code, exists := g.morse[char]
		if !exists {
			continue
		}

		for i, symbol := range code {
			if symbol == '.' {
				appendTone(dotLen)
			} else if symbol == '-' {
				appendTone(dotLen * 3)
			}

			// Symbol gap (dot length) within character
			if i < len(code)-1 {
				appendSilence(dotLen)
			}
		}
		// Inter-character gap (3 dots)
		appendSilence(dotLen * 3)
	}

	return buffer
}

// ============================================================================
// 3. 信道模拟器 (Channel Simulator)
// ============================================================================

type ChannelEffects struct {
	SNRdB    float64 // Signal-to-Noise Ratio
	QSBRate  float64 // Fading frequency (Hz), e.g., 0.5Hz
	QSBDepth float64 // Fading depth (0.0 - 1.0)
}

// ApplyEffects 在纯净信号上叠加噪声和衰落
func ApplyEffects(signal []float32, sampleRate int, fx ChannelEffects) []float32 {
	out := make([]float32, len(signal))
	copy(out, signal)

	// 1. Calculate Signal Power (RMS^2)
	// 既然我们要加噪声，需要先知道信号本身的强度
	var signalEnergy float64
	nonZeroSamples := 0
	for _, s := range signal {
		// 仅统计非静音部分以获得更准确的 CW 载波功率估算，
		// 或者统计整体平均功率。对于 CW，通常关注 "Mark" 状态的 SNR。
		// 这里简化为整体 RMS。
		signalEnergy += float64(s * s)
		if s != 0 {
			nonZeroSamples++
		}
	}
	if nonZeroSamples == 0 {
		return out // 全是静音，没法加 SNR
	}

	// 信号平均功率 P_signal
	pSignal := signalEnergy / float64(len(signal))

	// 2. Add Gaussian White Noise (AWGN)
	// SNR(dB) = 10 * log10(P_signal / P_noise)
	// P_noise = P_signal / 10^(SNR/10)
	pNoise := pSignal / math.Pow(10, fx.SNRdB/10.0)
	noiseScale := math.Sqrt(pNoise)

	// QSB Setup
	qsbPhase := 0.0
	qsbInc := 2.0 * math.Pi * fx.QSBRate / float64(sampleRate)

	for i := range out {
		// 3. Apply QSB (Fading) first
		if fx.QSBDepth > 0 {
			// 简单的正弦衰落模型: 1.0 - (depth * (1+sin)/2)
			// 让幅度在 (1-depth) 到 1.0 之间波动
			fading := 1.0 - (fx.QSBDepth * (0.5 + 0.5*math.Sin(qsbPhase)))
			out[i] *= float32(fading)
			qsbPhase += qsbInc
		}

		// 4. Add Noise
		noise := rand.NormFloat64() * noiseScale
		out[i] += float32(noise)
	}

	return out
}

// ============================================================================
// 4. 评分引擎 (Scoring Engine)
// ============================================================================

// CalculateCER 计算字符错误率 (Character Error Rate)
// Uses Levenshtein Distance
func CalculateCER(reference, hypothesis string) (float64, int) {
	refRunes := []rune(strings.TrimSpace(reference))
	hypRunes := []rune(strings.TrimSpace(hypothesis))

	lenRef := len(refRunes)
	lenHyp := len(hypRunes)

	// Matrix initialization
	d := make([][]int, lenRef+1)
	for i := range d {
		d[i] = make([]int, lenHyp+1)
	}

	for i := 0; i <= lenRef; i++ {
		d[i][0] = i
	}
	for j := 0; j <= lenHyp; j++ {
		d[0][j] = j
	}

	for i := 1; i <= lenRef; i++ {
		for j := 1; j <= lenHyp; j++ {
			cost := 0
			if refRunes[i-1] != hypRunes[j-1] {
				cost = 1
			}
			// min(deletion, insertion, substitution)
			d[i][j] = min(
				d[i-1][j]+1,      // deletion
				d[i][j-1]+1,      // insertion
				d[i-1][j-1]+cost, // substitution
			)
		}
	}

	distance := d[lenRef][lenHyp]
	if lenRef == 0 {
		if lenHyp == 0 {
			return 0.0, 0
		}
		return 100.0, distance
	}

	cer := (float64(distance) / float64(lenRef)) * 100.0
	return cer, distance
}

func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// ============================================================================
// 5. 基准测试套件 (Benchmark Harness)
// ============================================================================

// MockDecoder: 一个模拟解码器，用于展示如何接入
// 实际使用时，请替换为你真实的解码器结构体
type MockDecoder struct {
	buffer  strings.Builder
	decoder *cw.ExperimentalDecoder
}

func (m *MockDecoder) ProcessAudioChunk(samples []float32) {
	// 模拟：这里不做任何 DSP，只是占位
	// 在真实的解码器中，你会在这里做 FFT/Goertzel 和状态机逻辑
	m.decoder.ProcessAudioChunk(samples)
}
func (m *MockDecoder) GetDecodedText() string {
	// 模拟：为了演示CER，我们返回一个硬编码的错误结果
	m.decoder.Stop()
	return m.buffer.String() // 假设输入是 "PARIS PARIS PARIS"
}
func (m *MockDecoder) Reset() {
	m.buffer.Reset()
}

type TestCase struct {
	Name     string
	Text     string
	WPM      float64
	SNR      float64
	QSBRate  float64
	QSBDepth float64
	Jitter   float64
}

func RunBenchmark(decoder Decoder) {
	// 标准测试文本 (Paris standard)
	baseText := "PARIS PARIS PARIS 73 NI HAO HOW ARE YOU"
	sampleRate := 48000

	testCases := []TestCase{
		{
			Name: "Level 1 (Easy)",
			Text: baseText,
			WPM:  20, SNR: 25.0, QSBRate: 0, QSBDepth: 0, Jitter: 0,
		},
		{
			Name: "Level 2 (Medium)",
			Text: baseText,
			WPM:  25, SNR: 6.0, QSBRate: 0.2, QSBDepth: 0.3, Jitter: 0.05,
		},
		{
			Name: "Level 2 (Medium)",
			Text: baseText,
			WPM:  25, SNR: 6.0, QSBRate: 0, QSBDepth: 0.8, Jitter: 0.05,
		},
		{
			Name: "Level 2 (Medium)",
			Text: baseText,
			WPM:  25, SNR: 6.0, QSBRate: 1.0, QSBDepth: 0, Jitter: 0.05,
		},
		{
			Name: "Level 2 (Medium)",
			Text: baseText,
			WPM:  25, SNR: 6.0, QSBRate: 0, QSBDepth: 0, Jitter: 0.15,
		},

		{
			Name: "Level 3 (Hard)",
			Text: baseText,
			WPM:  30, SNR: 0.0, QSBRate: 1.0, QSBDepth: 0.8, Jitter: 0.15,
		},
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "LEVEL\tWPM\tSNR(dB)\tJITTER\tQSBRate\tQSBDepth\tCER(%)\tTIME(ms)\tSTATUS")
	fmt.Fprintln(w, "-----\t---\t-------\t------\t------\t------\t------\t--------\t------")

	for _, tc := range testCases {

		t := cw.NewExperimentalDecoder(float64(sampleRate), 700)
		md := MockDecoder{
			decoder: t,
		}
		fn := func(s string) {
			if len(s) > 0 {
				md.buffer.Reset()
				md.buffer.WriteString(s)
			}
		}
		t.SetOnDecoded(fn)
		decoder = &md
		//decoder = NewRealDecoderAdapter(48000, 700)
		// 1. Setup Generator
		gen := NewAudioGenerator(AudioConfig{
			WPM:        tc.WPM,
			SampleRate: sampleRate,
			Frequency:  700,
			JitterPct:  tc.Jitter,
		})

		// 2. Generate Audio
		cleanAudio := gen.GenerateFromText(tc.Text)

		// 3. Apply Channel Effects
		noisyAudio := ApplyEffects(cleanAudio, sampleRate, ChannelEffects{
			SNRdB:    tc.SNR,
			QSBRate:  tc.QSBRate,
			QSBDepth: tc.QSBDepth,
		})

		// 4. Run Decoder
		decoder.Reset()
		start := time.Now()

		// 模拟流式输入，每次喂给解码器 1024 个采样点
		chunkSize := 1024
		for i := 0; i < len(noisyAudio); i += chunkSize {
			end := i + chunkSize
			if end > len(noisyAudio) {
				end = len(noisyAudio)
			}
			decoder.ProcessAudioChunk(noisyAudio[i:end])
		}

		elapsed := time.Since(start)
		decodedText := decoder.GetDecodedText()

		// 5. Score
		cer, _ := CalculateCER(tc.Text, decodedText)

		// Output result
		status := "PASS"
		if cer > 10.0 {
			status = "FAIL"
		} // 假设 10% CER 是阈值

		fmt.Fprintf(w, "%s\t%.0f\t%.1f\t%.0f%%\t%.2f\t%.2f\t%.2f%%\t%d\t%s\n",
			tc.Name, tc.WPM, tc.SNR, tc.Jitter*100, tc.QSBRate, tc.QSBDepth, cer, elapsed.Milliseconds(), status)
	}
	w.Flush()
}

// ============================================================================
// Main Entry
// ============================================================================

func main() {
	rand.Seed(time.Now().UnixNano()) // Go 1.20+ 不需要这行，旧版本需要

	fmt.Println("Starting CW Decoder Benchmark Suite...")
	fmt.Println("========================================")

	// 这里注入你的 Mock Decoder 或者真实 Decoder
	// myRealDecoder := &MyRealDecoder{}
	mockDecoder := &MockDecoder{}

	RunBenchmark(mockDecoder)

	fmt.Println("\nBenchmark Complete.")
}

// ============================================================================
// Helpers
// ============================================================================

func getMorseTable() map[rune]string {
	return map[rune]string{
		'A': ".-", 'B': "-...", 'C': "-.-.", 'D': "-..", 'E': ".", 'F': "..-.",
		'G': "--.", 'H': "....", 'I': "..", 'J': ".---", 'K': "-.-", 'L': ".-..",
		'M': "--", 'N': "-.", 'O': "---", 'P': ".--.", 'Q': "--.-", 'R': ".-.",
		'S': "...", 'T': "-", 'U': "..-", 'V': "...-", 'W': ".--", 'X': "-..-",
		'Y': "-.--", 'Z': "--..",
		'0': "-----", '1': ".----", '2': "..---", '3': "...--", '4': "....-",
		'5': ".....", '6': "-....", '7': "--...", '8': "---..", '9': "----.",
		' ': " ", // Space handled separately but kept here for completeness
	}
}

// RealDecoderAdapter 是一个适配器，将你的 ExperimentalDecoder 包装成测试工具需要的样子
type RealDecoderAdapter struct {
	decoder *cw.ExperimentalDecoder // 或者 *cw.ClusterDecoder
	buffer  strings.Builder
}

// 构造函数
func NewRealDecoderAdapter(sampleRate float64, freq float64) *RealDecoderAdapter {
	// 初始化你的真实解码器
	// 注意：这里使用的是你想要测试的那个解码器版本
	realDecoder := cw.NewExperimentalDecoder(sampleRate, freq)

	adapter := &RealDecoderAdapter{
		decoder: realDecoder,
	}

	// 钩住回调函数，把输出的字符存到 buffer 里
	realDecoder.SetOnDecoded(func(text string) {
		adapter.buffer.WriteString(text)
	})

	return adapter
}

// 实现 Decoder 接口: 处理音频
func (a *RealDecoderAdapter) ProcessAudioChunk(samples []float32) {
	a.decoder.ProcessAudioChunk(samples)
}

// 实现 Decoder 接口: 获取结果
func (a *RealDecoderAdapter) GetDecodedText() string {
	// 停止解码器以刷新最后的缓存
	a.decoder.Stop()
	return a.buffer.String()
}

// 实现 Decoder 接口: 重置
func (a *RealDecoderAdapter) Reset() {
	a.buffer.Reset()
	// 注意：如果 ExperimentalDecoder 内部有状态（如 AGC 历史），最好这里重新 new 一个，或者给它加一个 Reset 方法
	// 为简单起见，我们在 RunBenchmark 循环里每次都 New 了一个新的 Adapter，所以这里可以留空
}
