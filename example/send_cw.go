package main

import (
	"bufio"
	"cw"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	// 1. 配置串口参数
	// 请根据实际情况修改串口设备名
	portName := "/dev/tty.SLAB_USBtoUART"
	baudRate := 115200

	fmt.Printf("Connecting to ICOM 7300 on %s...\n", portName)

	// 2. 创建客户端实例
	client := cw.NewCIVClient(portName, baudRate)

	// 3. 打开连接
	if err := client.Open(); err != nil {
		log.Fatalf("Failed to open serial port: %v\n", err)
	}
	defer client.Close()
	fmt.Println("Connected. Type text and press Enter to send CW.")
	fmt.Println("Type 'exit' or 'quit' to stop.")

	// 4. 循环读取控制台输入
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
		input = strings.TrimSpace(input)

		if input == "" {
			continue
		}

		if strings.ToLower(input) == "exit" || strings.ToLower(input) == "quit" {
			break
		}

		// 转换为大写 (CW 通常只支持大写)
		textToSend := strings.ToUpper(input)

		// ICOM 7300 单次最多发送 30 字符
		// 如果输入过长，需要分段发送 (这里简单截断或报错，实际可优化为队列)
		if len(textToSend) > 30 {
			fmt.Println("Warning: Text too long, truncating to 30 chars.")
			textToSend = textToSend[:30]
		}

		fmt.Printf("Sending: %s\n", textToSend)
		if err := client.SendText(textToSend); err != nil {
			log.Printf("Error sending text: %v\n", err)
		}
	}

	fmt.Println("Bye.")
}
