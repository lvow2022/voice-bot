package stream

import (
	"fmt"
)

// ============ 基础过滤器实现 ============

// LogFilter 日志过滤器
type LogFilter struct {
	name string
}

// NewLogFilter 创建日志过滤器
func NewLogFilter(name string) *LogFilter {
	return &LogFilter{name: name}
}

// Filter 记录数据通过
func (f *LogFilter) Filter(data []byte) ([]byte, error) {
	fmt.Printf("[%s] %d bytes\n", f.name, len(data))
	return data, nil
}

// VolumeFilter 音量过滤器
type VolumeFilter struct {
	volume float64 // 音量倍数（0.0-2.0，1.0为原音量）
}

// NewVolumeFilter 创建音量过滤器
func NewVolumeFilter(volume float64) *VolumeFilter {
	return &VolumeFilter{volume: volume}
}

// Filter 调整音频音量
func (f *VolumeFilter) Filter(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	// 16-bit PCM 数据
	result := make([]byte, len(data))
	for i := 0; i < len(data); i += 2 {
		// 读取 16-bit sample (little-endian)
		sample := int16(data[i]) | int16(data[i+1])<<8

		// 调整音量
		adjusted := int16(float64(sample) * f.volume)

		// 写回 (little-endian)
		result[i] = byte(adjusted)
		result[i+1] = byte(adjusted >> 8)
	}

	return result, nil
}

// SetVolume 动态设置音量
func (f *VolumeFilter) SetVolume(volume float64) {
	f.volume = volume
}

// ResampleFilter 重采样过滤器
type ResampleFilter struct {
	fromRate int
	toRate   int
	ratio    float64
}

// NewResampleFilter 创建重采样过滤器
func NewResampleFilter(fromRate, toRate int) *ResampleFilter {
	return &ResampleFilter{
		fromRate: fromRate,
		toRate:   toRate,
		ratio:    float64(toRate) / float64(fromRate),
	}
}

// Filter 简单的降采样/升采样
// 注意：这是简化实现，生产环境建议使用专业的音频库
func (f *ResampleFilter) Filter(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	// 简单的线性插值重采样
	// 16-bit PCM, 单声道
	samples := bytesToInt16(data)
	var resampled []int16

	if f.ratio > 1.0 {
		// 升采样（16k -> 48k）
		for i := 0; i < len(samples)-1; i++ {
			resampled = append(resampled, samples[i])
			// 线性插值
			steps := int(1.0 / (1.0 / f.ratio))
			for j := 1; j < steps && i+j < len(samples); j++ {
				interpolated := int16(float64(samples[i]) + float64(samples[i+1]-samples[i])*float64(j)/float64(steps))
				resampled = append(resampled, interpolated)
			}
		}
	} else if f.ratio < 1.0 {
		// 降采样（48k -> 16k）
		step := int(1.0 / f.ratio)
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

// FilterChain 过滤器链（可复用）
type FilterChain struct {
	filters []Filter
}

// NewFilterChain 创建过滤器链
func NewFilterChain(filters ...Filter) *FilterChain {
	return &FilterChain{filters: filters}
}

// Add 添加过滤器到链
func (c *FilterChain) Add(filters ...Filter) *FilterChain {
	c.filters = append(c.filters, filters...)
	return c
}

// Filter 实现 Filter 接口
func (c *FilterChain) Filter(data []byte) ([]byte, error) {
	result := data
	for _, f := range c.filters {
		var err error
		result, err = f.Filter(result)
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
