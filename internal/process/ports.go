package process

import (
	"errors"
	"fmt"
	"net"
	"sync"
)

var ErrNoFreePort = errors.New("no free port in configured range")

// PortAllocator hands out ports from a fixed range, skipping ports that are
// already reserved or in use on the host.
type PortAllocator struct {
	mu       sync.Mutex
	start    int
	end      int
	next     int
	reserved map[int]bool
	// probe reports whether a port can be bound; overridable for tests.
	probe func(host string, port int) bool
}

func NewPortAllocator(start, end int) *PortAllocator {
	return &PortAllocator{start: start, end: end, next: start, reserved: map[int]bool{}, probe: canBind}
}

func canBind(host string, port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

// Allocate reserves a free port on host.
func (a *PortAllocator) Allocate(host string) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	size := a.end - a.start + 1
	for i := 0; i < size; i++ {
		p := a.next
		a.next++
		if a.next > a.end {
			a.next = a.start
		}
		if a.reserved[p] || !a.probe(host, p) {
			continue
		}
		a.reserved[p] = true
		return p, nil
	}
	return 0, fmt.Errorf("%w (%d-%d)", ErrNoFreePort, a.start, a.end)
}

// Reserve marks a specific port as taken; it fails if unavailable.
func (a *PortAllocator) Reserve(host string, port int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.reserved[port] {
		return fmt.Errorf("port %d is already allocated", port)
	}
	if !a.probe(host, port) {
		return fmt.Errorf("port %d is in use", port)
	}
	a.reserved[port] = true
	return nil
}

func (a *PortAllocator) Release(port int) {
	a.mu.Lock()
	delete(a.reserved, port)
	a.mu.Unlock()
}
