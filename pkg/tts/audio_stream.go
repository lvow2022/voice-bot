package tts

import "sync"

type audioStream struct {
	ch        chan AudioFrame
	cur       AudioFrame
	err       error
	closeOnce sync.Once
}

func newAudioStream() *audioStream {
	return &audioStream{
		ch: make(chan AudioFrame, 32),
	}
}

func (s *audioStream) Next() bool {
	cur, ok := <-s.ch
	s.cur = cur
	return ok
}

func (s *audioStream) Frame() AudioFrame {
	return s.cur
}

func (s *audioStream) Error() error {
	return s.err
}

func (s *audioStream) Close() error {
	s.closeOnce.Do(func() {
		close(s.ch)
	})
	return nil
}
