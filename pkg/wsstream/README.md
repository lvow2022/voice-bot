# WSStream - 泛型 WebSocket 流式基础设施

## 设计概述

WSStream 是一套基于泛型的 WebSocket 双向流式通信基础设施，旨在统一 ASR/TTS 等 WebSocket 服务的实现。

## 核心设计

### 1. 统一的事件模型

```go
type StreamEvent struct {
    Type string // "delta" | "final" | "error"

    Text  string  // 文本数据
    Audio []byte  // 音频数据
    Err   error   // 错误信息
}
```

**设计要点：**
- 保持简单：单一结构体承载所有事件类型
- 类型明确：通过 `Type` 字段区分事件类型
- 扩展性强：可以根据需要添加更多字段

### 2. Codec 抽象

```go
type Codec[Req any] interface {
    Encode(req Req) ([]byte, error)
    Decode(data []byte) (StreamEvent, error)
    MessageType() int
}
```

**优势：**
- **协议解耦**：每个服务实现自己的编解码器
- **类型安全**：泛型保证类型安全
- **灵活扩展**：支持半关闭等特殊操作

### 3. 双向流支持

```go
type WSStream[Req any] struct {
    // 发送端
    Send(ctx context.Context, req Req) error

    // 接收端
    Recv() <-chan StreamEvent

    // 生命周期
    Close() error
    Done() <-chan struct{}
    Err() error
}
```

**特点：**
- **双向并发**：读写独立循环
- **异步模式**：通过 channel 传递事件
- **资源管理**：自动处理连接关闭

### 4. Half-Close 支持

```go
// Minimax TTS
type TTSRequest struct {
    Text string
    EOF  bool // 标记文本发送完毕
}

// Volcano ASR
type AsrRequest struct {
    Audio  []byte
    IsLast bool // 标记音频发送完毕
}
```

**实现方式：**
- 通过扩展 Req 结构体支持半关闭
- 无需新增接口方法
- 保持接口简洁

## 使用示例

### Minimax TTS

```go
// 创建 provider
provider, _ := minimax.NewProvider(cfg)
provider.Connect(ctx, opts)

// 发送文本
go func() {
    provider.SendText("你好", nil)
    provider.SendText("世界", nil)
    provider.SendEOF() // half-close
}()

// 接收音频
for evt := range provider.Stream().Recv() {
    switch evt.Type {
    case "delta":
        play(evt.Audio)
    case "final":
        return
    case "error":
        log.Println(evt.Err)
        return
    }
}
```

### Volcano ASR

```go
// 创建 provider
provider, _ := volc.NewProvider(cfg)
provider.Connect(ctx, opts)

// 发送音频
go func() {
    for audio := range audioSource {
        provider.SendAudio(audio, false)
    }
    provider.SendAudio(nil, true) // half-close
}()

// 接收识别结果
for evt := range provider.Stream().Recv() {
    switch evt.Type {
    case "delta":
        fmt.Println("识别中:", evt.Text)
    case "final":
        fmt.Println("最终结果:", evt.Text)
        return
    case "error":
        log.Println(evt.Err)
        return
    }
}
```

## 架构对比

### vs OpenAI Stream

| 特性 | OpenAI Stream | WSStream |
|------|--------------|----------|
| **协议** | HTTP/SSE | WebSocket |
| **方向** | 单向（响应流） | 双向（请求+响应） |
| **API 风格** | 迭代器 (`Next()`) | Channel (`Recv()`) |
| **半关闭** | 不支持 | 支持（通过 EOF 字段） |
| **协议抽象** | 无 | Codec 接口 |

**选择理由：**
- ASR/TTS 需要真正的双向流式通信
- Go 的 channel 模式更适合并发场景
- Codec 抽象支持多种协议实现

### vs 原有 Provider 实现

| 特性 | 原有实现 | WSStream 实现 |
|------|---------|--------------|
| **代码复用** | 每个服务独立实现 | 统一的流管理 |
| **错误处理** | 分散在各处 | 统一的事件流 |
| **资源管理** | 手动管理 | 流自动管理 |
| **扩展性** | 较难扩展 | 插件式增强 |

**优势：**
- 减少重复代码 60%+
- 统一的错误处理模式
- 更容易添加新 provider

## 错误处理策略

```go
// 网络错误
if err := conn.Read(); err != nil {
    s.recvCh <- StreamEvent{
        Type: "error",
        Err:  fmt.Errorf("network: %w", err),
    }
    return // 结束流
}

// 解码错误
if err := codec.Decode(); err != nil {
    s.recvCh <- StreamEvent{
        Type: "error",
        Err:  fmt.Errorf("decode: %w", err),
    }
    continue // 继续读取
}

// 业务错误
if resp.Code != 0 {
    s.recvCh <- StreamEvent{
        Type: "error",
        Err:  fmt.Errorf("business: %w", resp.Message),
    }
    return // 或 continue 取决于错误严重程度
}
```

**分类处理：**
- **网络错误**：结束流（不可恢复）
- **解码错误**：继续读取（可恢复）
- **业务错误**：根据严重程度决定

## 扩展点

### 1. 重连机制

```go
type ReconnectableStream[Req any] struct {
    inner     *WSStream[Req]
    dialer    func() (Conn, error)
    maxRetry  int
}

func (s *ReconnectableStream[Req]) Send(ctx context.Context, req Req) error {
    err := s.inner.Send(ctx, req)
    if isNetworkError(err) && s.maxRetry > 0 {
        // 重连逻辑
    }
    return err
}
```

### 2. 背压控制

```go
type BackpressureStream[Req any] struct {
    inner *WSStream[Req]
    rate  time.Duration
}

func (b *BackpressureStream[Req]) Send(ctx context.Context, req Req) error {
    time.Sleep(b.rate)
    return b.inner.Send(ctx, req)
}
```

### 3. 监控指标

```go
type MonitoredStream[Req any] struct {
    inner  *WSStream[Req]
    metrics chan Metric
}

func (m *MonitoredStream[Req]) Send(ctx context.Context, req Req) error {
    start := time.Now()
    err := m.inner.Send(ctx, req)
    m.metrics <- Metric{
        Operation: "send",
        Duration:  time.Since(start),
        Size:      sizeOf(req),
    }
    return err
}
```

## 总结

WSStream 通过以下设计实现了统一和简洁：

1. **泛型设计**：支持不同类型的请求
2. **Codec 抽象**：协议无关的编解码
3. **Channel 模式**：Go 惯用的异步模式
4. **Half-close**：通过扩展 Req 支持
5. **错误分类**：网络错误 vs 解码错误 vs 业务错误

这套基础设施让 ASR/TTS 的实现变得简洁且统一，同时保持了足够的灵活性来支持不同的协议细节。
