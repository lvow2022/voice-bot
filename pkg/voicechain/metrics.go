package voicechain

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type Metrics struct {
	values map[string][]time.Duration
	mtx    sync.Mutex
}

type MetricResult struct {
	Key   string        `json:"key"`
	Count int           `json:"count"`
	Max   time.Duration `json:"max"`
	Min   time.Duration `json:"min"`
	Avg   time.Duration `json:"avg"`
}

func (r *MetricResult) String() string {
	return fmt.Sprintf("%s: %s / %s / %s", r.Key, r.Min.String(), r.Avg.String(), r.Max.String())
}

func (r *Metrics) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.Dump())
}

func (m *Metrics) Add(key string, duration time.Duration) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if m.values == nil {
		m.values = make(map[string][]time.Duration)
	}
	m.values[key] = append(m.values[key], duration)
}

func (m *Metrics) Dump() []MetricResult {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	var results []MetricResult
	for key, durations := range m.values {
		if len(durations) == 0 {
			continue
		}
		var sum time.Duration
		min := durations[0]
		max := durations[0]

		for _, duration := range durations {
			sum += duration
			if duration < min {
				min = duration
			}
			if duration > max {
				max = duration
			}
		}
		avg := sum / time.Duration(len(durations))
		results = append(results, MetricResult{
			Key:   key,
			Count: len(durations),
			Max:   max,
			Min:   min,
			Avg:   avg,
		})
	}
	return results
}

func (m *Metrics) Reset() {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.values = make(map[string][]time.Duration)
}
