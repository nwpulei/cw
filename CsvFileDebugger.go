package cw

import (
	"bufio"
	"fmt"
	"os"
)

// SignalDebugger 定义调试器接口
// 解码器只依赖这个接口，不依赖具体的文件操作
type SignalDebugger interface {
	Record(raw, filtered, envelope, threshold float64, state bool)
	Close()
}

// CsvFileDebugger 是 SignalDebugger 的具体实现
// 它封装了文件句柄，不向外暴露
type CsvFileDebugger struct {
	file   *os.File
	writer *bufio.Writer
}

// NewCsvFileDebugger 创建一个新的 CSV 调试器
func NewCsvFileDebugger(filename string) (*CsvFileDebugger, error) {
	f, err := os.Create(filename)
	if err != nil {
		return nil, err
	}

	w := bufio.NewWriter(f)
	// 写入表头
	if _, err := w.WriteString("RawInput,Filtered,Envelope,Threshold,SignalState\n"); err != nil {
		f.Close()
		return nil, err
	}

	return &CsvFileDebugger{
		file:   f,
		writer: w,
	}, nil
}

// Record 记录单帧数据
func (d *CsvFileDebugger) Record(raw, filtered, envelope, threshold float64, state bool) {
	stateVal := 0.0
	if state {
		stateVal = 1.0
	}
	// 格式化写入缓冲区
	fmt.Fprintf(d.writer, "%f,%f,%f,%f,%f\n", raw, filtered, envelope, threshold, stateVal)
}

// Close 关闭文件并刷新缓冲区
func (d *CsvFileDebugger) Close() {
	fmt.Println("Closing CsvFileDebugger")
	if d.writer != nil {
		d.writer.Flush()
	}
	if d.file != nil {
		d.file.Close()
	}
}

// NoOpDebugger 是一个空实现，用于生产环境（不记录数据时使用）
// 这样可以避免在核心代码中写大量的 if d.debugger != nil check
type NoOpDebugger struct{}

func (d *NoOpDebugger) Record(raw, filtered, envelope, threshold float64, state bool) {}
func (d *NoOpDebugger) Close()                                                        {}
