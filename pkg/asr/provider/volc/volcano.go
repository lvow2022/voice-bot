package volc

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"voicebot/pkg/asr/types"
)

// VolcanoProvider 火山引擎 ASR 协议适配器
type VolcanoProvider struct {
	config types.ProviderConfig
	conn   *websocket.Conn

	ctx    context.Context
	cancel context.CancelFunc

	seqNum  uint32
	closed  atomic.Bool
	writeMu sync.Mutex

	// 读通道
	recvCh chan []byte

	// 事件队列，用于缓冲多个事件并逐个返回
	eventQueue []types.AsrEvent
	eventMu    sync.Mutex

	wg       sync.WaitGroup
	closeOnce sync.Once
}

func NewVolcanoAdapter(cfg types.ProviderConfig) (*VolcanoProvider, error) {
	ctx, cancel := context.WithCancel(context.Background())
	return &VolcanoProvider{
		config:  cfg,
		ctx:     ctx,
		cancel:  cancel,
		recvCh:  make(chan []byte, 128),
	}, nil
}

func (a *VolcanoProvider) Connect(ctx context.Context, opts types.SessionOptions) error {
	// 更新配置
	if opts.SampleRate > 0 {
		a.config.SampleRate = opts.SampleRate
	}
	if opts.Format != "" {
		a.config.Format = opts.Format
	}

	connectID := generateUUID()

	headers := http.Header{}
	headers.Set("X-Api-App-Key", a.config.AppID)
	headers.Set("X-Api-Access-Key", a.config.APIKey)
	headers.Set("X-Api-Resource-Id", a.config.ResourceID)
	headers.Set("X-Api-Connect-Id", connectID)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(a.ctx, volcanoWSURL, headers)
	if err != nil {
		return fmt.Errorf("dial volcano asr: %w", err)
	}

	a.conn = conn

	slog.Info("volcano asr connected", "connectId", connectID)

	if err := a.sendFullClientRequest(); err != nil {
		_ = a.conn.Close()
		return fmt.Errorf("send full connect request: %w", err)
	}

	// 启动读取循环
	a.wg.Add(1)
	go a.readLoop()

	return nil
}

func (a *VolcanoProvider) SendAudio(data []byte, isLast bool) error {
	if a.closed.Load() {
		return fmt.Errorf("provider is closed")
	}

	compressed, err := gzipCompress(data)
	if err != nil {
		return fmt.Errorf("gzip compress: %w", err)
	}

	flag := byte(flagPositiveSeq)
	if isLast {
		flag = byte(flagLastPacket)
	}

	header := a.buildHeader(msgTypeAudioOnlyRequest, flag, serializeNone, compressGzip)

	seq := atomic.AddUint32(&a.seqNum, 1)
	msg := make([]byte, 4+4+4+len(compressed))
	copy(msg[0:4], header)
	binary.BigEndian.PutUint32(msg[4:8], seq)
	binary.BigEndian.PutUint32(msg[8:12], uint32(len(compressed)))
	copy(msg[12:], compressed)

	a.writeMu.Lock()
	defer a.writeMu.Unlock()

	return a.conn.WriteMessage(websocket.BinaryMessage, msg)
}

// RecvEvent 接收并解析识别事件，每次返回一个事件
func (a *VolcanoProvider) RecvEvent() (*types.AsrEvent, error) {
	// 先检查队列中是否有缓存事件
	a.eventMu.Lock()
	if len(a.eventQueue) > 0 {
		event := a.eventQueue[0]
		a.eventQueue = a.eventQueue[1:]
		a.eventMu.Unlock()
		return &event, nil
	}
	a.eventMu.Unlock()

	// 队列为空，从通道读取新数据
	select {
	case <-a.ctx.Done():
		return nil, a.ctx.Err()
	case data := <-a.recvCh:
		if data == nil {
			return nil, fmt.Errorf("connection closed")
		}

		events, err := a.parseEvents(data)
		if err != nil {
			return nil, err
		}

		if len(events) == 0 {
			return nil, nil
		}

		// 将除第一个外的所有事件放入队列
		if len(events) > 1 {
			a.eventMu.Lock()
			a.eventQueue = append(a.eventQueue, events[1:]...)
			a.eventMu.Unlock()
		}

		// 返回第一个事件
		return &events[0], nil
	}
}

// Close 关闭连接
func (a *VolcanoProvider) Close() error {
	a.closeOnce.Do(func() {
		a.closed.Store(true)
		a.cancel()

		if a.conn != nil {
			_ = a.conn.Close()
		}

		a.wg.Wait()
		close(a.recvCh)
	})
	return nil
}

// readLoop 读取循环
func (a *VolcanoProvider) readLoop() {
	defer a.wg.Done()

	for {
		select {
		case <-a.ctx.Done():
			return
		default:
			_, data, err := a.conn.ReadMessage()
			if err != nil {
				return
			}

			select {
			case <-a.ctx.Done():
				return
			case a.recvCh <- data:
			}
		}
	}
}

func (a *VolcanoProvider) sendFullClientRequest() error {
	enableITN := true
	enablePunc := true
	endWindowSize := 800
	resultType := "full"

	if a.config.Options != nil {
		if v, ok := a.config.Options["enableItn"].(bool); ok {
			enableITN = v
		}
		if v, ok := a.config.Options["enablePunc"].(bool); ok {
			enablePunc = v
		}
		if v, ok := a.config.Options["endWindowSize"].(int); ok {
			endWindowSize = v
		}
		if v, ok := a.config.Options["resultType"].(string); ok {
			resultType = v
		}
	}

	req := volcanoRequest{
		User: volcanoUser{
			UID: "voicebot",
		},
		Audio: volcanoAudio{
			Format:  a.config.Format,
			Rate:    a.config.SampleRate,
			Bits:    16,
			Channel: 1,
		},
		Request: volcanoReqCfg{
			ModelName:     "bigmodel",
			EnableITN:     enableITN,
			EnablePunc:    enablePunc,
			EndWindowSize: endWindowSize,
			ResultType:    resultType,
		},
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	compressed, err := gzipCompress(payload)
	if err != nil {
		return fmt.Errorf("gzip compress: %w", err)
	}

	header := a.buildHeader(msgTypeFullClientRequest, flagNoSequence, serializeJSON, compressGzip)

	msg := make([]byte, 4+4+len(compressed))
	copy(msg[0:4], header)
	binary.BigEndian.PutUint32(msg[4:8], uint32(len(compressed)))
	copy(msg[8:], compressed)

	a.writeMu.Lock()
	defer a.writeMu.Unlock()

	return a.conn.WriteMessage(websocket.BinaryMessage, msg)
}

func (a *VolcanoProvider) buildHeader(msgType, flag, serialize, compress byte) []byte {
	b0 := byte((protocolVersion << 4) | headerSize)
	b1 := byte((msgType << 4) | flag)
	b2 := byte((serialize << 4) | compress)
	b3 := byte(0x00)

	return []byte{b0, b1, b2, b3}
}

func (a *VolcanoProvider) parseEvents(data []byte) ([]types.AsrEvent, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("message too short: %d bytes", len(data))
	}

	header := data[0:4]
	msgType := (header[1] >> 4) & 0x0F
	flag := header[1] & 0x0F
	compress := header[2] & 0x0F

	switch msgType {
	case msgTypeFullServerResponse:
		if len(data) < 12 {
			return nil, fmt.Errorf("server response too short")
		}
		payloadSize := binary.BigEndian.Uint32(data[8:12])
		payload := data[12 : 12+payloadSize]

		payloadData, err := decompressPayload(payload, compress)
		if err != nil {
			return nil, err
		}

		return a.parseServerResponse(payloadData, flag)

	case msgTypeErrorFromServer:
		if len(data) < 12 {
			return nil, fmt.Errorf("error message too short")
		}
		errorCode := binary.BigEndian.Uint32(data[4:8])
		errorSize := binary.BigEndian.Uint32(data[8:12])
		errorMsg := string(data[12 : 12+errorSize])

		return []types.AsrEvent{{
			Type: types.EventError,
			Text: fmt.Sprintf("server error %d: %s", errorCode, errorMsg),
		}}, nil

	default:
		slog.Warn("volcano asr unknown message type", "type", msgType, "flag", flag)
		return nil, nil
	}
}

func (a *VolcanoProvider) parseServerResponse(data []byte, flag byte) ([]types.AsrEvent, error) {
	var resp volcanoResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if resp.Code != 0 && resp.Code != 20000000 {
		return []types.AsrEvent{{
			Type: types.EventError,
			Text: fmt.Sprintf("response error %d: %s", resp.Code, resp.Message),
		}}, nil
	}

	if resp.Result == nil {
		return nil, nil
	}

	var events []types.AsrEvent
	isFinal := flag == flagNegativeSeq || flag == flagLastPacket || resp.IsFinal

	if len(resp.Result.Utterances) > 0 {
		for _, utt := range resp.Result.Utterances {
			if utt.Text == "" {
				continue
			}

			event := types.AsrEvent{
				Type:       types.EventPartial,
				Text:       utt.Text,
				Confidence: 1.0,
				IsFinal:    utt.Definite,
			}

			if utt.Definite {
				event.Type = types.EventFinal
			}

			events = append(events, event)
		}
	} else if resp.Result.Text != "" {
		event := types.AsrEvent{
			Type:       types.EventPartial,
			Text:       resp.Result.Text,
			Confidence: 1.0,
			IsFinal:    isFinal,
		}

		if isFinal {
			event.Type = types.EventFinal
		}

		events = append(events, event)
	}

	return events, nil
}

// Helper functions

func gzipCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		gz.Close()
		return nil, err
	}
	gz.Close()
	return buf.Bytes(), nil
}

func decompressPayload(payload []byte, compress byte) ([]byte, error) {
	if compress == compressGzip {
		gz, err := gzip.NewReader(bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("create gzip reader: %w", err)
		}
		data, err := io.ReadAll(gz)
		gz.Close()
		if err != nil {
			return nil, fmt.Errorf("decompress: %w", err)
		}
		return data, nil
	}
	return payload, nil
}

func generateUUID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().Nanosecond())
}
