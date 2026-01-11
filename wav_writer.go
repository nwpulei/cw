package cw

import (
	"encoding/binary"
	"os"
)

// WavWriter 简单的 WAV 文件写入器
type WavWriter struct {
	file       *os.File
	sampleRate int
	dataSize   int
}

// NewWavWriter 创建新的 WAV 写入器
func NewWavWriter(filename string, sampleRate int) (*WavWriter, error) {
	f, err := os.Create(filename)
	if err != nil {
		return nil, err
	}

	// 写入占位符头 (44字节)
	// 稍后在 Close 时我们会回写正确的大小
	header := make([]byte, 44)
	if _, err := f.Write(header); err != nil {
		f.Close()
		return nil, err
	}

	return &WavWriter{
		file:       f,
		sampleRate: sampleRate,
		dataSize:   0,
	}, nil
}

// WriteSamples 写入音频采样数据 (float32)
func (w *WavWriter) WriteSamples(samples []float32) error {
	// 将 float32 (-1.0 ~ 1.0) 转换为 int16
	buf := make([]byte, len(samples)*2)
	for i, s := range samples {
		// 简单的限幅
		if s > 1.0 {
			s = 1.0
		} else if s < -1.0 {
			s = -1.0
		}
		val := int16(s * 32767)
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(val))
	}

	n, err := w.file.Write(buf)
	if err != nil {
		return err
	}
	w.dataSize += n
	return nil
}

// Close 关闭文件并回写 WAV 头
func (w *WavWriter) Close() error {
	// 回写 WAV 头
	// RIFF chunk
	// Format: WAVE
	// fmt chunk
	// data chunk

	totalSize := 36 + w.dataSize
	header := make([]byte, 44)

	// RIFF header
	copy(header[0:], []byte("RIFF"))
	binary.LittleEndian.PutUint32(header[4:], uint32(totalSize))
	copy(header[8:], []byte("WAVE"))

	// fmt chunk
	copy(header[12:], []byte("fmt "))
	binary.LittleEndian.PutUint32(header[16:], 16) // Subchunk1Size (16 for PCM)
	binary.LittleEndian.PutUint16(header[20:], 1)  // AudioFormat (1 for PCM)
	binary.LittleEndian.PutUint16(header[22:], 1)  // NumChannels (1 for Mono)
	binary.LittleEndian.PutUint32(header[24:], uint32(w.sampleRate)) // SampleRate
	binary.LittleEndian.PutUint32(header[28:], uint32(w.sampleRate*2)) // ByteRate (SampleRate * NumChannels * BitsPerSample/8)
	binary.LittleEndian.PutUint16(header[32:], 2)  // BlockAlign (NumChannels * BitsPerSample/8)
	binary.LittleEndian.PutUint16(header[34:], 16) // BitsPerSample

	// data chunk
	copy(header[36:], []byte("data"))
	binary.LittleEndian.PutUint32(header[40:], uint32(w.dataSize))

	// Seek 到开头并写入
	if _, err := w.file.Seek(0, 0); err != nil {
		return err
	}
	if _, err := w.file.Write(header); err != nil {
		return err
	}

	return w.file.Close()
}
