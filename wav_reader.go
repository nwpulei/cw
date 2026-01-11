package cw

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// WavReader 简单的 WAV 文件读取器 (仅支持 16-bit PCM Mono/Stereo)
type WavReader struct {
	file       *os.File
	SampleRate int
	Channels   int
	DataSize   int
	dataStart  int64
}

func NewWavReader(filename string) (*WavReader, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	// 读取 RIFF 头
	riffHeader := make([]byte, 12)
	if _, err := f.Read(riffHeader); err != nil {
		f.Close()
		return nil, err
	}

	if string(riffHeader[0:4]) != "RIFF" || string(riffHeader[8:12]) != "WAVE" {
		f.Close()
		return nil, fmt.Errorf("invalid wav file")
	}

	var channels, sampleRate, bitsPerSample, dataSize int
	var dataStart int64
	foundFmt := false
	foundData := false

	for {
		chunkHeader := make([]byte, 8)
		if _, err := f.Read(chunkHeader); err != nil {
			if err == io.EOF {
				break
			}
			f.Close()
			return nil, err
		}

		chunkID := string(chunkHeader[0:4])
		chunkSize := binary.LittleEndian.Uint32(chunkHeader[4:8])

		// Pad byte if chunk size is odd
		padding := int64(chunkSize % 2)

		if chunkID == "fmt " {
			if chunkSize < 16 {
				f.Close()
				return nil, fmt.Errorf("fmt chunk too small")
			}
			fmtData := make([]byte, chunkSize)
			if _, err := f.Read(fmtData); err != nil {
				f.Close()
				return nil, err
			}
			if padding > 0 {
				f.Seek(padding, io.SeekCurrent)
			}

			channels = int(binary.LittleEndian.Uint16(fmtData[2:4]))
			sampleRate = int(binary.LittleEndian.Uint32(fmtData[4:8]))
			bitsPerSample = int(binary.LittleEndian.Uint16(fmtData[14:16]))
			foundFmt = true
		} else if chunkID == "data" {
			dataSize = int(chunkSize)
			pos, _ := f.Seek(0, io.SeekCurrent)
			dataStart = pos
			foundData = true

			if foundFmt {
				break
			}
			// Skip data
			if _, err := f.Seek(int64(chunkSize)+padding, io.SeekCurrent); err != nil {
				f.Close()
				return nil, err
			}
		} else {
			// Skip unknown chunk
			if _, err := f.Seek(int64(chunkSize)+padding, io.SeekCurrent); err != nil {
				f.Close()
				return nil, err
			}
		}
	}

	if !foundFmt || !foundData {
		f.Close()
		return nil, fmt.Errorf("invalid wav file: missing fmt or data chunk")
	}

	if bitsPerSample != 16 {
		f.Close()
		return nil, fmt.Errorf("only 16-bit wav supported, got %d", bitsPerSample)
	}

	// 确保文件指针指向 data 开始
	if _, err := f.Seek(dataStart, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}

	return &WavReader{
		file:       f,
		SampleRate: sampleRate,
		Channels:   channels,
		DataSize:   dataSize,
		dataStart:  dataStart,
	}, nil
}

// ReadSamples 读取音频采样数据并转换为 float32
// count: 要读取的采样点数 (每个通道)
func (r *WavReader) ReadSamples(count int) ([]float32, error) {
	// 每次读取 count * channels 个采样点
	totalSamples := count * r.Channels
	buf := make([]byte, totalSamples*2) // 16-bit = 2 bytes

	n, err := r.file.Read(buf)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if n == 0 {
		return nil, io.EOF
	}

	// 转换
	// 如果是立体声，我们只取左声道 (或者混合)
	// 这里简单起见，只取第一个通道

	numFrames := n / (2 * r.Channels)
	out := make([]float32, numFrames)

	for i := 0; i < numFrames; i++ {
		// 读取第一个通道的 16-bit 数据
		offset := i * 2 * r.Channels
		val := int16(binary.LittleEndian.Uint16(buf[offset : offset+2]))

		// 归一化到 -1.0 ~ 1.0
		out[i] = float32(val) / 32768.0
	}

	return out, nil
}

func (r *WavReader) Close() error {
	return r.file.Close()
}
