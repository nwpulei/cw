package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"strings"
	"unicode"
)

// BigramStats 用于统计
type BigramStats struct {
	Counts map[string]map[string]int
	Totals map[string]int
}

func main() {
	// 1. 读取你的“黄金样本”文件 (比如 qso_practice.txt)
	//content, err := ioutil.ReadFile("qso_practice.txt")
	content, err := ioutil.ReadFile("all.txt")

	if err != nil {
		panic(err)
	}

	text := string(content)
	// 转大写，因为 CW 不分大小写
	text = strings.ToUpper(text)

	stats := &BigramStats{
		Counts: make(map[string]map[string]int),
		Totals: make(map[string]int),
	}

	// 2. 预处理：只保留 CW 能发的字符
	// 把连续空格合并为一个
	cleanText := preProcess(text)

	// 3. 统计频率
	// 我们的 key 是 string，因为可能是 "A" 也可能是 "SPACE"
	runes := []rune(cleanText)
	for i := 0; i < len(runes)-1; i++ {
		curr := string(runes[i])
		next := string(runes[i+1])

		if _, ok := stats.Counts[curr]; !ok {
			stats.Counts[curr] = make(map[string]int)
		}
		stats.Counts[curr][next]++
		stats.Totals[curr]++
	}

	// 4. 计算对数概率 (Log Probability) 并输出 JSON
	// 结构: {"A": {"B": -2.5, "C": -4.1}, ...}
	logProbs := make(map[string]map[string]float64)

	for curr, nextMap := range stats.Counts {
		logProbs[curr] = make(map[string]float64)
		total := float64(stats.Totals[curr])

		for next, count := range nextMap {
			// P(next|curr) = count / total
			// LogProb = log(count/total)

			//prob := float64(count) / total
			prob := math.Log(float64(count)) - math.Log(float64(total))
			logProbs[curr][next] = 0.0 + (1.0 * prob) // 这里可以直接存概率，或者存 log
			// 注意：为了方便 JSON 阅读，这里先存直观的概率
			// 实际加载到 Go 程序里时再由 Load 函数转成 Math.Log
		}
	}

	jsonData, _ := json.MarshalIndent(logProbs, "", "  ")
	ioutil.WriteFile("ham_bigrams.json", jsonData, 0644)
	fmt.Println("模型构建完成！生成了 ham_bigrams.json")
}

// A B C D E F G H I J K L M N O P Q R S T U V W X Y Z É 0 1 2 3 4 5 6 7
// 8 9 . , ? ' ! / ( ) & : ; = + - _ " $ @
func preProcess(input string) string {
	var sb strings.Builder
	lastWasSpace := false

	// 用户指定的允许字符集 (除了 A-Z, 0-9)
	// . , ? ' ! / ( ) & : ; = + - _ " $ @
	const specialChars = " .,?'!/()&:;=+-_$@"

	for _, r := range input {
		// 允许的字符：A-Z, 0-9 以及指定的特殊符号
		isValid := (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune(specialChars, r)

		if isValid {
			sb.WriteRune(r)
			lastWasSpace = false
		} else if unicode.IsSpace(r) {
			if !lastWasSpace {
				sb.WriteRune(' ') // 统一用单空格
				lastWasSpace = true
			}
		}
	}
	return sb.String()
}
