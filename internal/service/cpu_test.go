package service

import (
	"sync"
	"testing"
	"time"
)

func TestCPUUsageDelta(t *testing.T) {
	usage, err := cpuUsageDelta(cpuSample{total: 100, idle: 60}, cpuSample{total: 200, idle: 110})
	if err != nil {
		t.Fatalf("cpuUsageDelta returned error: %v", err)
	}
	if usage != 50 {
		t.Fatalf("usage = %v, want 50", usage)
	}
}

func TestCPUUsageDeltaRejectsCounterReset(t *testing.T) {
	if _, err := cpuUsageDelta(cpuSample{total: 200, idle: 100}, cpuSample{total: 150, idle: 75}); err == nil {
		t.Fatal("cpuUsageDelta accepted a counter reset")
	}
}

func TestCPUSamplerCachesForConcurrentClients(t *testing.T) {
	currentTime := time.Unix(100, 0)
	reads := 0
	sampler := cpuSampler{
		now: func() time.Time { return currentTime },
		readSample: func() (cpuSample, error) {
			reads++
			if reads == 1 {
				return cpuSample{total: 100, idle: 60}, nil
			}
			return cpuSample{total: 200, idle: 110}, nil
		},
	}

	first := sampler.read()
	if first.Error == "" || first.UsagePercent != nil {
		t.Fatalf("first sample = %#v, want an honest unavailable result", first)
	}

	const clients = 16
	results := make(chan CPUInfo, clients)
	var group sync.WaitGroup
	group.Add(clients)
	for i := 0; i < clients; i++ {
		go func() {
			defer group.Done()
			results <- sampler.read()
		}()
	}
	group.Wait()
	close(results)

	if reads != 1 {
		t.Fatalf("underlying samples before interval = %d, want 1", reads)
	}
	for result := range results {
		if result.Error == "" || result.UsagePercent != nil {
			t.Fatalf("cached result = %#v, want the first result", result)
		}
	}

	currentTime = currentTime.Add(cpuSampleInterval)
	result := sampler.read()
	if result.Error != "" || result.UsagePercent == nil || *result.UsagePercent != 50 {
		t.Fatalf("second interval result = %#v, want 50%% usage", result)
	}
	if reads != 2 {
		t.Fatalf("underlying samples after interval = %d, want 2", reads)
	}

	currentTime = currentTime.Add(100 * time.Millisecond)
	result = sampler.read()
	if result.Error != "" || result.UsagePercent == nil || *result.UsagePercent != 50 {
		t.Fatalf("repeated cached result = %#v, want 50%% usage", result)
	}
	if reads != 2 {
		t.Fatalf("underlying samples after repeated client = %d, want 2", reads)
	}
}
