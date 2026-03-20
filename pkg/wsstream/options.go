package wsstream

// Options 流配置选项
type Options struct {
	SendBufferSize int
	RecvBufferSize int
}

// DefaultOptions 默认配置
func DefaultOptions() Options {
	return Options{
		SendBufferSize: 128,
		RecvBufferSize: 128,
	}
}

// Option 配置函数
type Option func(*Options)

// WithSendBufferSize 设置发送缓冲区大小
func WithSendBufferSize(size int) Option {
	return func(o *Options) {
		o.SendBufferSize = size
	}
}

// WithRecvBufferSize 设置接收缓冲区大小
func WithRecvBufferSize(size int) Option {
	return func(o *Options) {
		o.RecvBufferSize = size
	}
}
