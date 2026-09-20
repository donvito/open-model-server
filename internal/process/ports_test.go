package process

import (
	"errors"
	"testing"
)

func TestPortAllocator(t *testing.T) {
	a := NewPortAllocator(100, 102)
	busy := map[int]bool{101: true}
	a.probe = func(host string, port int) bool { return !busy[port] }

	p1, err := a.Allocate("127.0.0.1")
	if err != nil || p1 != 100 {
		t.Fatalf("first allocate = %d, %v", p1, err)
	}
	p2, err := a.Allocate("127.0.0.1")
	if err != nil || p2 != 102 {
		t.Fatalf("should skip busy port 101: got %d, %v", p2, err)
	}
	if _, err := a.Allocate("127.0.0.1"); !errors.Is(err, ErrNoFreePort) {
		t.Fatalf("expected ErrNoFreePort, got %v", err)
	}
	a.Release(100)
	if p, err := a.Allocate("127.0.0.1"); err != nil || p != 100 {
		t.Fatalf("released port should be reusable: %d, %v", p, err)
	}
}

func TestPortAllocatorReserve(t *testing.T) {
	a := NewPortAllocator(100, 110)
	a.probe = func(string, int) bool { return true }
	if err := a.Reserve("127.0.0.1", 105); err != nil {
		t.Fatal(err)
	}
	if err := a.Reserve("127.0.0.1", 105); err == nil {
		t.Fatal("double reserve should fail")
	}
	a.probe = func(string, int) bool { return false }
	if err := a.Reserve("127.0.0.1", 106); err == nil {
		t.Fatal("reserving an in-use port should fail")
	}
}
