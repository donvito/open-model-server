//go:build linux

package service

import "testing"

func TestParseCPUStat(t *testing.T) {
	sample, err := parseCPUStat("cpu  100 20 30 400 10 5 15 20 7 3")
	if err != nil {
		t.Fatalf("parseCPUStat returned error: %v", err)
	}
	if sample.total != 600 {
		t.Fatalf("total = %d, want 600", sample.total)
	}
	if sample.idle != 410 {
		t.Fatalf("idle = %d, want 410", sample.idle)
	}
}

func TestParseCPUStatRejectsMalformedInput(t *testing.T) {
	if _, err := parseCPUStat("cpu 100 nope 30 400"); err == nil {
		t.Fatal("parseCPUStat accepted malformed input")
	}
}
