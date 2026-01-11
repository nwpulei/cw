package cw

import (
	"bytes"
	"testing"
)

// MockSerialPort 模拟串口
type MockSerialPort struct {
	ReadBuffer  *bytes.Buffer
	WriteBuffer *bytes.Buffer
	Closed      bool
}

func NewMockSerialPort() *MockSerialPort {
	return &MockSerialPort{
		ReadBuffer:  new(bytes.Buffer),
		WriteBuffer: new(bytes.Buffer),
	}
}

func (m *MockSerialPort) Read(p []byte) (n int, err error) {
	return m.ReadBuffer.Read(p)
}

func (m *MockSerialPort) Write(p []byte) (n int, err error) {
	return m.WriteBuffer.Write(p)
}

func (m *MockSerialPort) Close() error {
	m.Closed = true
	return nil
}

// 辅助函数：生成 CI-V 响应帧
func makeResponseFrame(cmd byte, data []byte) []byte {
	// FE FE E0 94 Cmd [Data...] FD
	frame := []byte{CIV_PREAMBLE, CIV_PREAMBLE, CIV_ADDR_PC, CIV_ADDR_7300, cmd}
	if len(data) > 0 {
		frame = append(frame, data...)
	}
	frame = append(frame, CIV_END)
	return frame
}

func TestSendCommand(t *testing.T) {
	mockPort := NewMockSerialPort()
	client := &CIVClient{conn: mockPort}

	// 测试发送指令 0x03 (读取频率)
	err := client.SendCommand(0x03, nil)
	if err != nil {
		t.Fatalf("SendCommand failed: %v", err)
	}

	// 验证发送的数据
	expected := []byte{0xFE, 0xFE, 0x94, 0xE0, 0x03, 0xFD}
	if !bytes.Equal(mockPort.WriteBuffer.Bytes(), expected) {
		t.Errorf("Expected command frame %X, got %X", expected, mockPort.WriteBuffer.Bytes())
	}
}

func TestReadFrequency(t *testing.T) {
	mockPort := NewMockSerialPort()
	client := &CIVClient{conn: mockPort}

	// 模拟电台响应: 7.050.00 MHz -> 00 00 50 07 00 (BCD)
	// 注意：ReadFrequency 会先发送指令，然后读取响应
	// 我们需要预先填充 ReadBuffer
	
	// 构造响应帧
	freqData := []byte{0x00, 0x00, 0x50, 0x07, 0x00}
	respFrame := makeResponseFrame(0x03, freqData)
	mockPort.ReadBuffer.Write(respFrame)

	freq, err := client.ReadFrequency()
	if err != nil {
		t.Fatalf("ReadFrequency failed: %v", err)
	}

	expectedFreq := 7050000
	if freq != expectedFreq {
		t.Errorf("Expected frequency %d, got %d", expectedFreq, freq)
	}
}

func TestReadMode(t *testing.T) {
	mockPort := NewMockSerialPort()
	client := &CIVClient{conn: mockPort}

	// 模拟电台响应: CW 模式 -> 0x03
	modeData := []byte{0x03}
	respFrame := makeResponseFrame(0x04, modeData)
	mockPort.ReadBuffer.Write(respFrame)

	mode, err := client.ReadMode()
	if err != nil {
		t.Fatalf("ReadMode failed: %v", err)
	}

	expectedMode := "CW"
	if mode != expectedMode {
		t.Errorf("Expected mode %s, got %s", expectedMode, mode)
	}
}

func TestReadMode_Unknown(t *testing.T) {
	mockPort := NewMockSerialPort()
	client := &CIVClient{conn: mockPort}

	// 模拟未知模式 -> 0xFF
	modeData := []byte{0xFF}
	respFrame := makeResponseFrame(0x04, modeData)
	mockPort.ReadBuffer.Write(respFrame)

	mode, err := client.ReadMode()
	if err != nil {
		t.Fatalf("ReadMode failed: %v", err)
	}

	expectedMode := "Unknown(0xFF)"
	if mode != expectedMode {
		t.Errorf("Expected mode %s, got %s", expectedMode, mode)
	}
}

func TestReadResponse_EchoFilter(t *testing.T) {
	mockPort := NewMockSerialPort()
	client := &CIVClient{conn: mockPort}

	// 模拟回显 + 真实响应
	// 回显: FE FE 94 E0 03 FD (PC -> Radio)
	// 响应: FE FE E0 94 03 00 00 50 07 00 FD (Radio -> PC)
	
	echoFrame := []byte{0xFE, 0xFE, 0x94, 0xE0, 0x03, 0xFD}
	freqData := []byte{0x00, 0x00, 0x50, 0x07, 0x00}
	respFrame := makeResponseFrame(0x03, freqData)
	
	mockPort.ReadBuffer.Write(echoFrame)
	mockPort.ReadBuffer.Write(respFrame)

	// 直接测试 readResponse 内部逻辑比较困难，因为它不是公开的
	// 但我们可以通过 ReadFrequency 间接测试
	
	// 注意：ReadFrequency 内部会先 Write 一次，这会清空我们上面的 WriteBuffer (如果我们在测试 SendCommand)
	// 但这里我们只关心 ReadBuffer
	
	freq, err := client.ReadFrequency()
	if err != nil {
		t.Fatalf("ReadFrequency with echo failed: %v", err)
	}

	expectedFreq := 7050000
	if freq != expectedFreq {
		t.Errorf("Expected frequency %d, got %d", expectedFreq, freq)
	}
}

func TestClose(t *testing.T) {
	mockPort := NewMockSerialPort()
	client := &CIVClient{conn: mockPort}

	err := client.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if !mockPort.Closed {
		t.Error("Expected port to be closed")
	}
}
