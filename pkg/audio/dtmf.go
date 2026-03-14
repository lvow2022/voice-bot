package audio

import (
	"github.com/shenjinti/dtmfdecoder"
)

var Keypads = []rune{
	'1', '2', '3', 'A',
	'4', '5', '6', 'B',
	'7', '8', '9', 'C',
	'*', '0', '#', 'D',
}

type DTMFHandler func(sender, digit string)

type DTMFDetector struct {
	dt         *dtmfdecoder.DTMFDecoder
	WiggleRoom int
}

func NewDTMFDetector(energyThreshold float64, sampleRate int) *DTMFDetector {
	return &DTMFDetector{
		dt: dtmfdecoder.NewDTMFDecoder(energyThreshold, sampleRate),
	}
}

func (d *DTMFDetector) Process(samples []byte, handler DTMFHandler) {
	floatSamples := make([]float64, 0)
	for i := 0; i < len(samples); i += 2 {
		sample := int16(samples[i]) | int16(samples[i+1])<<8
		floatSamples = append(floatSamples, float64(sample)/32768.0)
	}
	digit, ok := d.dt.Decode(floatSamples)
	if ok {
		handler("detector", digit)
	}
}
