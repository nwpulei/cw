package BeamDecoder

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
)

// LanguageModel 管理转移概率
type LanguageModel struct {
	// LogProbs 存储 log(P(Next|Current))
	// 使用对数是为了防止概率连乘导致下溢，且加法比乘法快
	LogProbs    map[string]map[string]float64
	DefaultProb float64 // 遇到未知组合时的惩罚分
}

// NewLanguageModel 初始化
func NewLanguageModel() *LanguageModel {
	lm := &LanguageModel{
		LogProbs:    make(map[string]map[string]float64),
		DefaultProb: math.Log(1e-6), // 极小的概率
	}
	// 在真实项目中，这里应该从文件加载 huge_bigrams.json
	lm.loadDummyData()
	return lm
}

// GetTransitionScore 获取从 prevChar -> nextChar 的转移得分
func (lm *LanguageModel) GetTransitionScore(prevChar, nextChar string) float64 {
	if nextMap, ok := lm.LogProbs[prevChar]; ok {
		if prob, ok := nextMap[nextChar]; ok {
			return prob
		}
	}
	// 如果是单词开头（假设 prevChar 为空），给予均等概率或特定概率
	if prevChar == "" {
		return math.Log(0.05)
	}
	return lm.DefaultProb
}

// loadDummyData 仅作演示，硬编码一些常见组合
func (lm *LanguageModel) loadDummyData() {
	//// 辅助函数：设置概率
	//set := func(prev, next string, prob float64) {
	//	if _, ok := lm.LogProbs[prev]; !ok {
	//		lm.LogProbs[prev] = make(map[string]float64)
	//	}
	//	lm.LogProbs[prev][next] = math.Log(prob)
	//}
	//
	//// 常见的莫尔斯电码组合
	//set("Q", "U", 0.95) // Q后面几乎必跟U
	//set("T", "H", 0.40) // TH 组合
	//set("H", "E", 0.30) // HE 组合
	//set("I", "N", 0.25) // IN
	//// ... 你需要用脚本统计几本英文小说来填充这里

	content, err := os.ReadFile("/Users/leilei/work/goProject/src/cw/BuildModel/ham_bigrams.json")
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
	json.Unmarshal(content, &lm.LogProbs)
}
