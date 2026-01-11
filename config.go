package cw

import "time"

// Config 结构体用于集中管理解码器的所有可调参数和阈值
type Config struct {
	// --- 频谱监控 (SpectrumMonitor) ---
	// 负责在后台分析频谱，提取主频，并进行自适应静噪
	Monitor struct {
		Enabled        bool          // 是否启用后台频谱监控 (true: 开启, false: 关闭)
		UpdateInterval time.Duration // 分析周期 (例如 200ms)，决定了频率更新的频率
		FFTSize        int           // FFT 点数 (例如 4096)，决定了频率分辨率。越大分辨率越高，但计算量越大
		MinFrequency   float64       // 频率搜索下限 (Hz)，用于屏蔽低频底噪 (例如 600Hz)
		MaxFrequency   float64       // 频率搜索上限 (Hz)，用于限制搜索范围 (例如 900Hz)
		RequiredSNR    float64       // 触发频率更新所需的最小信噪比 (线性值)。例如 10.0 代表信号功率需是底噪的 10 倍 (10dB)
		AlphaBase      float64       // 频率平滑的基础学习率 (0.0 - 1.0)。值越小，频率变化越平滑；值越大，响应越快
		AlphaGain      float64       // 频率平滑的学习率增益。随 SNR 增加而增加，使强信号能更快拉动频率
		AlphaMax       float64       // 频率平滑的最大学习率，防止频率跳变过快
	}

	// --- SDR 解调 ---
	// 负责将音频信号混频、滤波并提取包络
	SDR struct {
		LpfAlpha    float64 // I/Q 低通滤波器的系数 (0.0 - 1.0)。值越小，平滑度越高，抗噪越好，但对快速信号响应变慢。0.05 适合 40WPM
		AfcEnabled  bool    // 是否启用 AFC (自动频率控制)，用于微调相位漂移
		AfcGain     float64 // AFC 增益，决定了 AFC 跟踪频率的速度
		AfcDeadband float64 // AFC 死区 (Hz)，频率误差小于此值时不进行调整，防止抖动
		FilterBW    float64 // 低通滤波器截止频率 (Hz)。决定了接收带宽 (BW = 2 * Cutoff)。例如 50.0 代表 100Hz 带宽
	}

	// --- 解码逻辑 (ClusterDecoder) ---
	// 负责将包络信号转换为点划序列，并解码为文本
	Decoder struct {
		// AGC / 阈值
		AgcEnabled   bool    // 是否启用 AGC (自动增益/阈值控制)
		AgcPeakDecay float64 // 信号峰值的衰减率 (例如 0.99995)。值越接近 1，峰值保持时间越长
		AgcPeakFloor float64 // 信号峰值的最低值，防止在完全静音时阈值降得太低
		AgcHighRatio float64 // 动态阈值高位 = 峰值 * 此比例 (例如 0.5)。施密特触发器的开启阈值
		AgcLowRatio  float64 // 动态阈值低位 = 高位 * 此比例 (例如 0.85)。施密特触发器的关闭阈值，较高的值有助于防止字符粘连
		AgcMinHigh   float64 // 动态阈值高位的最小值，防止锁定到微弱底噪

		// 统计和聚类
		MarkWindowSize  int     // Mark (信号) 统计窗口大小 (例如 16)。用于 K-Means 聚类的样本数量
		SpaceWindowSize int     // Space (静音) 统计窗口大小 (例如 16)
		MinDotLen       float64 // 最小点长 (秒)。0.024s 对应约 50 WPM。用于限制自适应范围
		MaxDotLen       float64 // 最大点长 (秒)。0.24s 对应约 5 WPM。用于限制自适应范围

		// 时序判定
		MarkGlitchMs  int     // Mark Glitch 过滤时长 (毫秒)。小于此长度的信号被视为噪声忽略
		SpaceGlitchMs int     // Space Glitch 过滤时长 (毫秒)。小于此长度的静音被视为信号抖动忽略
		DotDashRatio  float64 // 点划分割阈值系数。Threshold = dotLen * 此比例 (例如 2.2)。小于为点，大于为划
		CharGapRatio  float64 // 字符分割阈值系数。Threshold = dotLen * 此比例 (例如 1.5)。大于此间隔被视为字符结束
		CharGapMinMs  int     // 最小字符分割时长 (毫秒)。硬性兜底，防止在高码率下字符粘连 (例如 60ms)
		WordGapRatio  float64 // 单词分割阈值系数。Threshold = dotLen * 此比例 (例如 5.0)。大于此间隔输出空格
	}
}

// DefaultConfig 返回一个包含当前最佳实践的默认配置
func DefaultConfig() *Config {
	cfg := &Config{}

	// --- 频谱监控 ---
	cfg.Monitor.Enabled = true // 默认开启，以自动锁定频率
	cfg.Monitor.UpdateInterval = 200 * time.Millisecond
	cfg.Monitor.FFTSize = 4096
	cfg.Monitor.MinFrequency = 600.0
	cfg.Monitor.MaxFrequency = 900.0
	cfg.Monitor.RequiredSNR = 40.0 // 10dB
	cfg.Monitor.AlphaBase = 0.02
	cfg.Monitor.AlphaGain = 0.005
	cfg.Monitor.AlphaMax = 0.5

	// --- SDR 解调 ---
	cfg.SDR.LpfAlpha = 0.05
	cfg.SDR.AfcEnabled = true
	cfg.SDR.AfcGain = 0.0002
	cfg.SDR.AfcDeadband = 1.0
	cfg.SDR.FilterBW = 50.0 // 恢复为 50Hz 截止频率 (100Hz 带宽)

	// --- 解码逻辑 ---
	cfg.Decoder.AgcEnabled = true
	cfg.Decoder.AgcPeakDecay = 0.9995
	cfg.Decoder.AgcPeakFloor = 0.01
	cfg.Decoder.AgcHighRatio = 0.5
	cfg.Decoder.AgcLowRatio = 0.85
	cfg.Decoder.AgcMinHigh = 0.005

	cfg.Decoder.MarkWindowSize = 16
	cfg.Decoder.SpaceWindowSize = 16
	cfg.Decoder.MinDotLen = 0.024 // 50 WPM
	cfg.Decoder.MaxDotLen = 0.24  // 5 WPM

	cfg.Decoder.MarkGlitchMs = 20
	cfg.Decoder.SpaceGlitchMs = 20
	cfg.Decoder.DotDashRatio = 2.2
	cfg.Decoder.CharGapRatio = 1.5
	cfg.Decoder.CharGapMinMs = 60 // 60ms, 对应 50 WPM
	cfg.Decoder.WordGapRatio = 5.0

	return cfg
}
