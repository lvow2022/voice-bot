package voicechain

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

type TransportFile struct {
	file      *os.File
	FileExt   string
	ChunkSize int
}

func LoadFromFile(name string) (*TransportFile, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	return NewTransportFile(file, name, "", 0), nil
}

func WriteToFile(name string) (*TransportFile, error) {
	file, err := os.Create(name)
	if err != nil {
		return nil, err
	}
	return NewTransportFile(file, name, "", 0), nil
}

func NewTransportFile(file *os.File, name string, frameType string, chunkSize int) *TransportFile {
	if chunkSize == 0 {
		chunkSize = 4000
	}
	return &TransportFile{
		file:      file,
		ChunkSize: chunkSize,
		FileExt:   strings.ToLower(filepath.Ext(name)),
	}
}

func (tf *TransportFile) String() string {
	return tf.file.Name()
}

func (tf *TransportFile) Close() error {
	if tf.file == nil {
		return nil
	}
	err := tf.file.Close()
	tf.file = nil
	return err
}

func (tf *TransportFile) Next(ctx context.Context) (Frame, error) {
	if tf.file == nil {
		return nil, ErrTransportIsClosed
	}
	chunkSize := tf.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 4000
	}
	data := make([]byte, chunkSize)
	result := make(chan struct {
		n   int
		err error
	})

	// is it possible to use a goroutine here?
	go func() {
		n, err := tf.file.Read(data)
		result <- struct {
			n   int
			err error
		}{n, err}
	}()

	select {
	case res := <-result:
		if res.err != nil {
			return nil, res.err
		}
		data = data[:res.n]
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	var frame Frame
	switch tf.FileExt {
	case ".txt", ".log", ".json", ".xml", ".html", ".md":
		frame = &TextFrame{
			Text: string(data),
		}
	default:
		frame = &AudioFrame{
			Payload: data,
		}
	}
	return frame, nil
}

func (tf *TransportFile) Send(ctx context.Context, frame Frame) (int, error) {
	if tf.file == nil {
		return 0, ErrTransportIsClosed
	}
	body := frame.Body()
	if len(body) == 0 {
		return 0, nil
	}
	return tf.file.Write(body)
}

func (tf *TransportFile) Attach(_ *Session) {
	// do nothing
}

func (tf *TransportFile) Codec() CodecOption {
	return DefaultCodecOption()
}
