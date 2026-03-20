package volc

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"
)

// ============ Constants ============

const volcanoWSURL = "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel"

// Protocol constants
const (
	protocolVersion = 0x01
	headerSize      = 0x01
)

// Message types
const (
	msgTypeFullClientRequest  byte = 0x01
	msgTypeFullServerResponse byte = 0x09
	msgTypeAudioOnlyRequest   byte = 0x0B
	msgTypeErrorFromServer    byte = 0x0F
)

// Serialization types
const (
	serializeNone byte = 0x00
	serializeJSON byte = 0x01
)

// Compression types
const (
	compressNone byte = 0x00
	compressGzip byte = 0x01
)

// Message flags
const (
	flagNoSequence  byte = 0x00
	flagPositiveSeq byte = 0x01
	flagLastPacket  byte = 0x02
	flagNegativeSeq byte = 0x03
)

// ============ Request & Event ============

// AsrRequest ASR 请求
type AsrRequest struct {
	Audio  []byte
	IsLast bool // half-close 标记
}

// AsrEventType ASR 事件类型
type AsrEventType int

const (
	AsrEventPartial AsrEventType = iota
	AsrEventFinal
	AsrEventError
)

func (t AsrEventType) String() string {
	switch t {
	case AsrEventPartial:
		return "partial"
	case AsrEventFinal:
		return "final"
	case AsrEventError:
		return "error"
	default:
		return "unknown"
	}
}

// AsrEvent ASR 事件
type AsrEvent struct {
	Type       AsrEventType
	Text       string
	Confidence float64
	Err        error
}

// IsFinal 实现 FinalEvent 接口
func (e AsrEvent) IsFinal() bool {
	return e.Type == AsrEventFinal
}

// ============ Protocol Messages ============

// VolcanoRequest 火山引擎 ASR 请求
type VolcanoRequest struct {
	User    VolcanoUser   `json:"user"`
	Audio   VolcanoAudio  `json:"audio"`
	Request VolcanoReqCfg `json:"request"`
}

// VolcanoUser 用户信息
type VolcanoUser struct {
	UID string `json:"uid"`
}

// VolcanoAudio 音频配置
type VolcanoAudio struct {
	Format  string `json:"format"`
	Rate    int    `json:"rate"`
	Bits    int    `json:"bits"`
	Channel int    `json:"channel"`
}

// VolcanoReqCfg 请求配置
type VolcanoReqCfg struct {
	ModelName     string `json:"model_name"`
	EnableITN     bool   `json:"enable_itn"`
	EnablePunc    bool   `json:"enable_punc"`
	EndWindowSize int    `json:"end_window_size"`
	ResultType    string `json:"result_type"`
}

// VolcanoResponse 火山引擎 ASR 响应
type VolcanoResponse struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	IsFinal bool           `json:"is_final"`
	Result  *VolcanoResult `json:"result,omitempty"`
}

// VolcanoResult 识别结果
type VolcanoResult struct {
	Text       string             `json:"text"`
	Utterances []VolcanoUtterance `json:"utterances,omitempty"`
}

// VolcanoUtterance 话语片段
type VolcanoUtterance struct {
	Text     string `json:"text"`
	Definite bool   `json:"definite"`
}

// ============ Codec ============

// Codec 火山引擎 ASR 编解码器
type Codec struct {
	cfg    Config
	seqNum uint32
}

// NewCodec 创建编解码器
func NewCodec(cfg Config) *Codec {
	return &Codec{cfg: cfg}
}

// Encode 编码请求
func (c *Codec) Encode(req any) ([]byte, error) {
	asrReq, ok := req.(AsrRequest)
	if !ok {
		return nil, fmt.Errorf("expected AsrRequest, got %T", req)
	}

	// Half-close: 发送 last packet
	if asrReq.IsLast {
		return c.encodeLastPacket()
	}

	// 普通音频包
	compressed, err := gzipCompress(asrReq.Audio)
	if err != nil {
		return nil, fmt.Errorf("gzip compress: %w", err)
	}

	// 构建消息头
	header := c.buildHeader(msgTypeAudioOnlyRequest, flagPositiveSeq, serializeNone, compressGzip)

	seq := atomic.AddUint32(&c.seqNum, 1)
	msg := make([]byte, 4+4+4+len(compressed))
	copy(msg[0:4], header)
	binary.BigEndian.PutUint32(msg[4:8], seq)
	binary.BigEndian.PutUint32(msg[8:12], uint32(len(compressed)))
	copy(msg[12:], compressed)

	return msg, nil
}

// Decode 解码响应
func (c *Codec) Decode(data []byte) (any, error) {
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

		return c.parseServerResponse(payloadData, flag)

	case msgTypeErrorFromServer:
		if len(data) < 12 {
			return AsrEvent{
				Type: AsrEventError,
				Err:  fmt.Errorf("error message too short"),
			}, nil
		}
		errorCode := binary.BigEndian.Uint32(data[4:8])
		errorSize := binary.BigEndian.Uint32(data[8:12])
		errorMsg := string(data[12 : 12+errorSize])

		return AsrEvent{
			Type: AsrEventError,
			Err:  fmt.Errorf("server error %d: %s", errorCode, errorMsg),
		}, nil

	default:
		return nil, nil
	}
}

// MessageType 返回消息类型
func (c *Codec) MessageType() int {
	return 2 // websocket.BinaryMessage
}

// EncodeFullClientRequest 编码完整客户端请求
func (c *Codec) EncodeFullClientRequest() ([]byte, error) {
	enableITN := true
	enablePunc := true
	endWindowSize := 800
	resultType := "full"

	if c.cfg.Options != nil {
		if v, ok := c.cfg.Options["enableItn"].(bool); ok {
			enableITN = v
		}
		if v, ok := c.cfg.Options["enablePunc"].(bool); ok {
			enablePunc = v
		}
		if v, ok := c.cfg.Options["endWindowSize"].(int); ok {
			endWindowSize = v
		}
		if v, ok := c.cfg.Options["resultType"].(string); ok {
			resultType = v
		}
	}

	req := VolcanoRequest{
		User: VolcanoUser{
			UID: "voicebot",
		},
		Audio: VolcanoAudio{
			Format:  c.cfg.Format,
			Rate:    c.cfg.SampleRate,
			Bits:    16,
			Channel: 1,
		},
		Request: VolcanoReqCfg{
			ModelName:     "bigmodel",
			EnableITN:     enableITN,
			EnablePunc:    enablePunc,
			EndWindowSize: endWindowSize,
			ResultType:    resultType,
		},
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	compressed, err := gzipCompress(payload)
	if err != nil {
		return nil, fmt.Errorf("gzip compress: %w", err)
	}

	header := c.buildHeader(msgTypeFullClientRequest, flagNoSequence, serializeJSON, compressGzip)

	msg := make([]byte, 4+4+len(compressed))
	copy(msg[0:4], header)
	binary.BigEndian.PutUint32(msg[4:8], uint32(len(compressed)))
	copy(msg[8:], compressed)

	return msg, nil
}

// parseServerResponse 解析服务器响应
func (c *Codec) parseServerResponse(data []byte, flag byte) (any, error) {
	var resp VolcanoResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if resp.Code != 0 && resp.Code != 20000000 {
		return AsrEvent{
			Type: AsrEventError,
			Err:  fmt.Errorf("response error %d: %s", resp.Code, resp.Message),
		}, nil
	}

	if resp.Result == nil {
		return nil, nil
	}

	isFinal := flag == flagNegativeSeq || flag == flagLastPacket || resp.IsFinal

	// 解析 utterances
	if len(resp.Result.Utterances) > 0 {
		for _, utt := range resp.Result.Utterances {
			if utt.Text == "" {
				continue
			}

			evt := AsrEvent{
				Type: AsrEventPartial,
				Text: utt.Text,
			}

			if utt.Definite {
				evt.Type = AsrEventFinal
				return evt, nil
			}

			return evt, nil
		}
	} else if resp.Result.Text != "" {
		evt := AsrEvent{
			Type: AsrEventPartial,
			Text: resp.Result.Text,
		}

		if isFinal {
			evt.Type = AsrEventFinal
		}

		return evt, nil
	}

	return nil, nil
}

// buildHeader 构建消息头
func (c *Codec) buildHeader(msgType, flag, serialize, compress byte) []byte {
	b0 := byte((protocolVersion << 4) | headerSize)
	b1 := byte((msgType << 4) | flag)
	b2 := byte((serialize << 4) | compress)
	b3 := byte(0x00)

	return []byte{b0, b1, b2, b3}
}

// encodeLastPacket 编码最后一个音频包
func (c *Codec) encodeLastPacket() ([]byte, error) {
	header := c.buildHeader(msgTypeAudioOnlyRequest, flagLastPacket, serializeNone, compressGzip)

	seq := atomic.AddUint32(&c.seqNum, 1)
	msg := make([]byte, 4+4+4)
	copy(msg[0:4], header)
	binary.BigEndian.PutUint32(msg[4:8], seq)
	binary.BigEndian.PutUint32(msg[8:12], 0) // size = 0

	return msg, nil
}

// gzipCompress gzip 压缩
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

// decompressPayload 解压负载
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
