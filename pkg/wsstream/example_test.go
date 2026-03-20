package wsstream_test

import (
	"context"
	"fmt"
	 "time"

	"voicebot/pkg/wsstream"
)

// ExampleBasicUsage 示例：基本使用模式
func ExampleBasicUsage() {
    // 创建一个简单的 codec
    codec := &stringCodec{}

    // 模拟 WebSocket 连接
    conn := &mockConn{
        sendCh: make(chan []byte, 10),
        recvCh: make(chan []byte, 10),
    }

    // 创建流
    stream := wsstream.NewWSStream[string](conn, codec)

    // 发送数据
    go func() {
        for i := 0; i < 5; i++ {
            if err := stream.Send(context.Background(), fmt.Sprintf("message-%d", i)); err != nil {
                fmt.Printf("send error: %v\n", err)
            }
        }
        // 发送结束标记
        stream.Send(context.Background(), "EOF")
    } }()

    // 接收数据
    for evt := range stream.Recv() {
        switch evt.Type {
        case "delta":
            fmt.Printf("received: %s\n", evt.Text)
        case "final":
            fmt.Println("stream completed")
            return
        case "error":
            fmt.Printf("error: %v\n", evt.Err)
            return
        }
    }

    // 等待结束
    <-stream.Done()
    if err := stream.Err(); err != nil {
        fmt.Printf("stream error: %v\n", err)
    }
}

    stream.Close()
}

// stringCodec 简单的字符串编解码器
type stringCodec struct{}

func (c *stringCodec) Encode(req string) ([]byte, error) {
    return []byte(req), nil
}

func (c *stringCodec) Decode(data []byte) (wsstream.StreamEvent, error) {
    msg := string(data)

    if msg == "EOF" {
        return wsstream.StreamEvent{
            Type: "final",
        }, nil
    }

    return wsstream.StreamEvent{
        Type: "delta",
        Text: msg,
    }, nil
}

func (c *stringCodec) MessageType() int {
    return 1 // TextMessage
}

// mockConn 模拟连接
type mockConn struct {
    sendCh chan []byte
    recvCh chan []byte
}

func (c *mockConn) Read() (int, []byte, error) {
    data, ok := <-c.recvCh
    if !ok {
        return 0, nil, fmt.Errorf("connection closed")
    }
    return 1, data, nil
}

func (c *mockConn) Write(messageType int, data []byte) error {
    select {
    case c.sendCh <- data:
        return nil
    default:
        return fmt.Errorf("connection closed")
    }
}

func (c *mockConn) Close() error {
    close(c.sendCh)
    close(c.recvCh)
    return nil
}
