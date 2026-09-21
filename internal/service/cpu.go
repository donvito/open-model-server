package service

import (
	"errors"
	"sync"
	"time"
)

const cpuSampleInterval = time.Second

// CPUInfo is the host CPU utilization reported by the most recent sample.
// UsagePercent is omitted when the platform cannot provide a sample or when
// the first sample has not yet established a baseline.
type CPUInfo struct {
	UsagePercent *float64 `json:"usage_percent,omitempty"`
	Error        string   `json:"error,omitempty"`
}

type cpuSample struct {
	total uint64
	idle  uint64
}

type cpuSampler struct {
	mu         sync.Mutex
	previous   cpuSample
	hasSample  bool
	sampledAt  time.Time
	last       CPUInfo
	now        func() time.Time
	readSample func() (cpuSample, error)
}

func (s *cpuSampler) read() CPUInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now
	if s.now != nil {
		now = s.now
	}
	currentTime := now()
	if s.hasSample && currentTime.Sub(s.sampledAt) < cpuSampleInterval {
		return cloneCPUInfo(s.last)
	}

	readSample := readCPUSample
	if s.readSample != nil {
		readSample = s.readSample
	}
	current, err := readSample()
	if err != nil {
		info := CPUInfo{Error: err.Error()}
		if s.hasSample {
			s.sampledAt = currentTime
			s.last = info
		}
		return info
	}
	if !s.hasSample {
		s.previous = current
		s.hasSample = true
		s.sampledAt = currentTime
		s.last = CPUInfo{Error: "CPU utilization unavailable until a second sample is collected"}
		return cloneCPUInfo(s.last)
	}

	usage, err := cpuUsageDelta(s.previous, current)
	s.previous = current
	s.sampledAt = currentTime
	if err != nil {
		s.last = CPUInfo{Error: err.Error()}
		return cloneCPUInfo(s.last)
	}
	s.last = CPUInfo{UsagePercent: &usage}
	return cloneCPUInfo(s.last)
}

func cloneCPUInfo(info CPUInfo) CPUInfo {
	if info.UsagePercent != nil {
		usage := *info.UsagePercent
		info.UsagePercent = &usage
	}
	return info
}

func cpuUsageDelta(previous, current cpuSample) (float64, error) {
	if current.total < previous.total || current.idle < previous.idle {
		return 0, errors.New("CPU utilization counters reset")
	}
	totalDelta := current.total - previous.total
	idleDelta := current.idle - previous.idle
	if totalDelta == 0 {
		return 0, errors.New("CPU utilization unavailable because no time elapsed")
	}
	if idleDelta > totalDelta {
		return 0, errors.New("CPU utilization counters are inconsistent")
	}

	usage := float64(totalDelta-idleDelta) * 100 / float64(totalDelta)
	if usage < 0 {
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}
	return usage, nil
}
