package main

import (
	"bufio"
	"cw"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	// 1. 解析命令行参数
	recordAudio := flag.Bool("record", false, "Record audio to capture.wav")
	inputFile := flag.String("file", "", "Input wav file for replay testing")
	flag.Parse()

	// 2. 初始化系统
	system := cw.NewCWSystem()
	//a := "/Users/leilei/work/goProject/src/cw/testData/test1.wav"
	//inputFile = &a
	if *inputFile != "" {
		system.SetReplayFile(*inputFile)
	}
	if *recordAudio {
		system.EnableRecording("capture.wav")
	}

	// 3. 启动系统
	if err := system.Start(); err != nil {
		log.Fatalf("System start failed: %v", err)
	}
	defer system.Stop()

	// 4. 主循环 (处理信号和控制台输入)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// 启动控制台输入监听
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		fmt.Println("System Ready. (Type 'exit' to quit)")

		for scanner.Scan() {
			input := scanner.Text()
			input = strings.TrimSpace(input)
			if input == "" {
				continue
			}

			if strings.ToLower(input) == "exit" || strings.ToLower(input) == "quit" {
				sigChan <- os.Interrupt
				return
			}

			// 将输入传递给系统处理
			system.HandleInput(input)
			fmt.Print("> ")
		}
	}()

	// 阻塞等待退出信号
	<-sigChan
	fmt.Println("\nShutting down...")
}
