package voicechain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMetrics(t *testing.T) {
	m := Metrics{}

	begin := time.Now()
	m.Add("test.ttfb", time.Since(begin))

	d := time.Second
	time.Sleep(d)

	m.Add("test.ttfb", time.Since(begin))

	json, err := m.MarshalJSON()
	assert.Nil(t, err)
	assert.NotNil(t, json)

	metricResults := m.Dump()
	assert.True(t, metricResults[0].Max > d)
	assert.True(t, metricResults[0].Avg < d)
	assert.True(t, metricResults[0].Min < d)

	m.Reset()
	assert.True(t, len(m.values) == 0)
}
