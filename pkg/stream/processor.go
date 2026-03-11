package stream

import (
	"fmt"
)

// ============ Stream Processor（AOP 处理器）============

// Processor 处理器接口（处理 []byte 数据）
type Processor interface {
	Process(data []byte) ([]byte, error)
}

// ProcessorFunc 函数式处理器（类似 http.HandlerFunc）
type ProcessorFunc func(data []byte) ([]byte, error)

// Process 实现 Processor 接口
func (f ProcessorFunc) Process(data []byte) ([]byte, error) {
	return f(data)
}

// ============ 基础处理器实现 ============

// LoggerProcessor 日志记录处理器
type LoggerProcessor struct {
	name string
}

// NewLoggerProcessor 创建日志处理器
func NewLoggerProcessor(name string) *LoggerProcessor {
	return &LoggerProcessor{
		name: name,
	}
}

// Process 记录数据通过
func (p *LoggerProcessor) Process(data []byte) ([]byte, error) {
	fmt.Printf("[Logger:%s] Processed %d bytes\n", p.name, len(data))
	return data, nil
}

// SimpleVolumeProcessor 简单的音量控制处理器（处理 []byte）
type SimpleVolumeProcessor struct {
	volume float64 // 音量倍数（0.0-2.0，1.0为原音量）
}

// NewSimpleVolumeProcessor 创建简单音量处理器
func NewSimpleVolumeProcessor(volume float64) *SimpleVolumeProcessor {
	return &SimpleVolumeProcessor{
		volume: volume,
	}
}

// Process 调整音频音量
func (p *SimpleVolumeProcessor) Process(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	// 16-bit PCM 数据
	result := make([]byte, len(data))
	for i := 0; i < len(data); i += 2 {
		// 读取 16-bit sample (little-endian)
		sample := int16(data[i]) | int16(data[i+1])<<8

		// 调整音量
		adjusted := int16(float64(sample) * p.volume)

		// 写回 (little-endian)
		result[i] = byte(adjusted)
		result[i+1] = byte(adjusted >> 8)
	}

	return result, nil
}

// SetVolume 动态设置音量
func (p *SimpleVolumeProcessor) SetVolume(volume float64) {
	p.volume = volume
}

// SimpleResamplerProcessor 简单的重采样处理器（处理 []byte）
type SimpleResamplerProcessor struct {
	fromRate int
	toRate   int
	ratio    float64
}

// NewSimpleResamplerProcessor 创建简单重采样处理器
func NewSimpleResamplerProcessor(fromRate, toRate int) *SimpleResamplerProcessor {
	return &SimpleResamplerProcessor{
		fromRate: fromRate,
		toRate:   toRate,
		ratio:    float64(toRate) / float64(fromRate),
	}
}

// Process 简单的降采样/升采样
// 注意：这是简化实现，生产环境建议使用专业的音频库
func (p *SimpleResamplerProcessor) Process(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	// 简单的线性插值重采样
	// 16-bit PCM, 单声道
	samples := bytesToInt16(data)
	var resampled []int16

	if p.ratio > 1.0 {
		// 升采样（16k -> 48k）
		for i := 0; i < len(samples)-1; i++ {
			resampled = append(resampled, samples[i])
			// 线性插值
			steps := int(1.0 / (1.0 / p.ratio))
			for j := 1; j < steps && i+j < len(samples); j++ {
				interpolated := int16(float64(samples[i]) + float64(samples[i+1]-samples[i])*float64(j)/float64(steps))
				resampled = append(resampled, interpolated)
			}
		}
	} else if p.ratio < 1.0 {
		// 降采样（48k -> 16k）
		step := int(1.0 / p.ratio)
		for i := 0; i < len(samples); i += step {
			if i+step < len(samples) {
				// 取平均值
				sum := int32(0)
				count := 0
				for j := 0; j < step && i+j < len(samples); j++ {
					sum += int32(samples[i+j])
					count++
				}
				if count > 0 {
					resampled = append(resampled, int16(sum/int32(count)))
				}
			}
		}
	} else {
		// 相同采样率，直接返回
		return data, nil
	}

	return int16ToBytes(resampled), nil
}

// StreamProcessorChain 流处理器链（可复用）
type StreamProcessorChain struct {
	processors []Processor
}

// NewStreamProcessorChain 创建处理器链
func NewStreamProcessorChain() *StreamProcessorChain {
	return &StreamProcessorChain{
		processors: make([]Processor, 0),
	}
}

// Add 添加处理器到链
func (pc *StreamProcessorChain) Add(processor Processor) *StreamProcessorChain {
	pc.processors = append(pc.processors, processor)
	return pc
}

// Process 实现 Processor 接口
func (pc *StreamProcessorChain) Process(data []byte) ([]byte, error) {
	result := data
	for _, p := range pc.processors {
		var err error
		result, err = p.Process(result)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// ============ 辅助函数 ============

// bytesToInt16 将字节切片转换为 int16 切片（little-endian）
func bytesToInt16(b []byte) []int16 {
	samples := make([]int16, len(b)/2)
	for i := range samples {
		samples[i] = int16(b[i*2]) | int16(b[i*2+1])<<8
	}
	return samples
}

// int16ToBytes 将 int16 切片转换为字节切片（little-endian）
func int16ToBytes(samples []int16) []byte {
	b := make([]byte, len(samples)*2)
	for i, s := range samples {
		b[i*2] = byte(s)
		b[i*2+1] = byte(s >> 8)
	}
	return b
}
