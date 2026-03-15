package volc

const (
	// volcanoWSURL is the WebSocket URL for Volcano Engine ASR.
	volcanoWSURL = "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel"

	// Protocol header constants
	protocolVersion = 0x01
	headerSize      = 0x01 // header size = 1 * 4 = 4 bytes

	// Message types
	msgTypeFullClientRequest  = 0x01
	msgTypeAudioOnlyRequest   = 0x02
	msgTypeFullServerResponse = 0x09
	msgTypeErrorFromServer    = 0x0F

	// Message type specific flags
	flagNoSequence  = 0x00
	flagPositiveSeq = 0x01
	flagLastPacket  = 0x02
	flagNegativeSeq = 0x03

	// Serialization methods
	serializeNone = 0x00
	serializeJSON = 0x01

	// Compression methods
	compressNone = 0x00
	compressGzip = 0x01
)

// volcanoRequest is the full client request payload.
type volcanoRequest struct {
	User    volcanoUser   `json:"user"`
	Audio   volcanoAudio  `json:"audio"`
	Request volcanoReqCfg `json:"request"`
}

type volcanoUser struct {
	UID string `json:"uid,omitempty"`
}

type volcanoAudio struct {
	Format  string `json:"format"`
	Rate    int    `json:"rate"`
	Bits    int    `json:"bits"`
	Channel int    `json:"channel"`
}

type volcanoReqCfg struct {
	ModelName     string `json:"model_name"`
	EnableITN     bool   `json:"enable_itn"`
	EnablePunc    bool   `json:"enable_punc"`
	EnableDDC     bool   `json:"enable_ddc"`
	EndWindowSize int    `json:"end_window_size,omitempty"`
	ResultType    string `json:"result_type,omitempty"`
}

// volcanoResponse is the server response payload.
type volcanoResponse struct {
	IsFinal   bool              `json:"is_final"`
	Result    *volcanoResult    `json:"result"`
	AudioInfo *volcanoAudioInfo `json:"audio_info"`
	Code      int               `json:"code"`
	Message   string            `json:"message"`
}

type volcanoResult struct {
	Text       string             `json:"text"`
	Utterances []volcanoUtterance `json:"utterances"`
}

type volcanoUtterance struct {
	Text      string `json:"text"`
	Definite  bool   `json:"definite"`
	StartTime int    `json:"start_time"`
	EndTime   int    `json:"end_time"`
}

type volcanoAudioInfo struct {
	Duration int `json:"duration"`
}
