package cw

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/tarm/serial"
)

const (
	CIV_PREAMBLE  = 0xFE
	CIV_END       = 0xFD
	CIV_ADDR_7300 = 0x94 // ICOM 7300 默认地址
	CIV_ADDR_PC   = 0xE0 // 控制器(PC) 默认地址
)

// SerialPort 定义串口操作接口，方便测试 Mock
type SerialPort interface {
	io.ReadWriteCloser
}

// CIVClient 处理与 ICOM 电台的通信
type CIVClient struct {
	Port     string
	BaudRate int
	conn     SerialPort
}

// NewCIVClient 创建新的 CI-V 客户端
func NewCIVClient(port string, baudRate int) *CIVClient {
	return &CIVClient{
		Port:     port,
		BaudRate: baudRate,
	}
}

// Open 打开串口连接
func (c *CIVClient) Open() error {
	config := &serial.Config{
		Name:        c.Port,
		Baud:        c.BaudRate,
		ReadTimeout: time.Millisecond * 500,
	}
	s, err := serial.OpenPort(config)
	if err != nil {
		return err
	}
	c.conn = s
	return nil
}

// Close 关闭串口连接
func (c *CIVClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// SendCommand 发送 CI-V 命令
func (c *CIVClient) SendCommand(cmd byte, subCmd []byte) error {
	if c.conn == nil {
		return fmt.Errorf("connection not open")
	}
	// 构造帧: FE FE [To] [From] [Cmd] [SubCmd...] FD
	frame := []byte{CIV_PREAMBLE, CIV_PREAMBLE, CIV_ADDR_7300, CIV_ADDR_PC, cmd}
	if len(subCmd) > 0 {
		frame = append(frame, subCmd...)
	}
	frame = append(frame, CIV_END)

	_, err := c.conn.Write(frame)
	return err
}

// SendText 发送 CW 文本 (ICOM 7300 Cmd 0x17)
// text: 要发送的字符串 (最大 30 字符)
func (c *CIVClient) SendText(text string) error {
	if len(text) > 30 {
		return fmt.Errorf("text too long (max 30 chars)")
	}
	
	// Cmd 0x17: Send CW Message
	// 数据部分直接是 ASCII 字符
	data := []byte(text)
	
	// 发送指令
	// 注意：发送 CW 指令后，电台通常不会立即返回数据，除非配置了 Echo
	// 我们这里只负责发送
	return c.SendCommand(0x17, data)
}

// ReadFrequency 读取当前频率 (Hz)
func (c *CIVClient) ReadFrequency() (int, error) {
	// Cmd 0x03: Read operating frequency
	if err := c.SendCommand(0x03, nil); err != nil {
		return 0, err
	}

	resp, err := c.readResponse(0x03)
	if err != nil {
		return 0, err
	}

	// 解析 BCD 编码的频率数据
	// 响应格式: FE FE E0 94 03 [d1 d2 d3 d4 d5] FD
	// 数据部分是 5 字节 BCD，低位在前
	// 例如 7.050.00 MHz -> 00 00 50 07 00
	if len(resp) < 5 {
		return 0, fmt.Errorf("invalid frequency data length")
	}

	data := resp // 已经是数据部分
	freq := 0
	multiplier := 1

	// ICOM 频率数据通常是 5 字节，从低位到高位
	for i := 0; i < 5 && i < len(data); i++ {
		val := bcdToDecimal(data[i])
		freq += val * multiplier
		multiplier *= 100
	}

	return freq, nil
}

// ReadMode 读取当前模式 (LSB, USB, CW, etc.)
func (c *CIVClient) ReadMode() (string, error) {
	// Cmd 0x04: Read operating mode
	if err := c.SendCommand(0x04, nil); err != nil {
		return "", err
	}

	resp, err := c.readResponse(0x04)
	if err != nil {
		return "", err
	}

	if len(resp) < 1 {
		return "", fmt.Errorf("invalid mode data")
	}

	// 模式映射表
	modes := map[byte]string{
		0x00: "LSB", 0x01: "USB", 0x02: "AM", 0x03: "CW",
		0x04: "RTTY", 0x05: "FM", 0x07: "CW-R", 0x08: "RTTY-R",
		0x17: "DV",
	}

	modeByte := resp[0]
	if name, ok := modes[modeByte]; ok {
		return name, nil
	}
	return fmt.Sprintf("Unknown(0x%02X)", modeByte), nil
}

// readResponse 读取并解析响应
func (c *CIVClient) readResponse(expectedCmd byte) ([]byte, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("connection not open")
	}
	buf := make([]byte, 1024)
	n, err := c.conn.Read(buf)
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("connection closed")
		}
		// 串口读取超时也可能返回 err，视库实现而定
		// 这里简单处理
	}
	if n == 0 {
		return nil, fmt.Errorf("timeout or no data")
	}

	data := buf[:n]
	// 简单的帧查找逻辑
	// 寻找 FE FE E0 94 [Cmd] ... FD
	// 注意：串口可能会回显我们发送的指令，需要过滤

	// 查找目标帧头: FE FE [To=PC] [From=7300] [Cmd]
	header := []byte{CIV_PREAMBLE, CIV_PREAMBLE, CIV_ADDR_PC, CIV_ADDR_7300, expectedCmd}
	idx := bytes.Index(data, header)
	if idx == -1 {
		// 可能是回显，或者分包了（这里简化处理，假设一次读完或主要部分在buffer里）
		// 实际生产代码需要更健壮的 buffer 缓存机制
		return nil, fmt.Errorf("response header not found in: %s", hex.EncodeToString(data))
	}

	// 截取从 header 开始的数据
	frame := data[idx:]
	endIdx := bytes.IndexByte(frame, CIV_END)
	if endIdx == -1 {
		return nil, fmt.Errorf("frame end not found")
	}

	// 提取数据部分: Header(5 bytes) ... Data ... End(1 byte)
	// Header: FE FE E0 94 Cmd
	if endIdx <= 5 {
		return []byte{}, nil // 无数据
	}

	return frame[5:endIdx], nil
}

func bcdToDecimal(b byte) int {
	return int((b >> 4) * 10 + (b & 0x0F))
}

// AutoDetectPort 尝试列出可能的串口 (仅作占位，实际需要系统调用或库支持)
func AutoDetectPort() string {
	// MacOS 常见 USB 串口名
	return "/dev/tty.SLAB_USBtoUART"
}
