// Package logs keeps a bounded in-memory log per model with live subscribers.
package logs

import (
	"fmt"
	"sync"
	"time"
)

type Line struct {
	Time   time.Time `json:"time"`
	Source string    `json:"source"` // stdout, stderr, system
	Text   string    `json:"text"`
	Model  string    `json:"model,omitempty"`
}

// Buffer is a ring buffer of log lines with fan-out to subscribers.
type Buffer struct {
	mu     sync.Mutex
	cap    int
	lines  []Line
	subs   map[chan Line]struct{}
	mirror *Buffer
	model  string
}

func NewBuffer(capacity int) *Buffer {
	if capacity <= 0 {
		capacity = 1000
	}
	return &Buffer{cap: capacity, subs: map[chan Line]struct{}{}}
}

func (b *Buffer) Append(source, text string) {
	l := Line{Time: time.Now(), Source: source, Text: text}
	b.appendLine(l)
}

func (b *Buffer) appendLine(l Line) {
	b.mu.Lock()
	if len(b.lines) >= b.cap {
		copy(b.lines, b.lines[1:])
		b.lines = b.lines[:len(b.lines)-1]
	}
	b.lines = append(b.lines, l)
	for ch := range b.subs {
		select {
		case ch <- l:
		default: // slow subscriber; drop rather than block the producer
		}
	}
	if b.mirror != nil {
		l.Model = b.model
		b.mirror.appendLine(l)
	}
	b.mu.Unlock()
}

// Systemf appends a formatted line from the application itself.
func (b *Buffer) Systemf(format string, args ...any) {
	b.Append("system", fmt.Sprintf(format, args...))
}

// Tail returns up to n most recent lines (all if n <= 0).
func (b *Buffer) Tail(n int) []Line {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n <= 0 || n > len(b.lines) {
		n = len(b.lines)
	}
	out := make([]Line, n)
	copy(out, b.lines[len(b.lines)-n:])
	return out
}

// Subscribe returns a channel of new lines and a cancel function.
func (b *Buffer) Subscribe() (<-chan Line, func()) {
	ch := make(chan Line, 256)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
}

func (b *Buffer) Clear() {
	b.mu.Lock()
	b.lines = b.lines[:0]
	b.mu.Unlock()
}

// Store holds one Buffer per model ID.
type Store struct {
	mu       sync.Mutex
	capacity int
	buffers  map[string]*Buffer
}

func NewStore(capacityPerModel int) *Store {
	return &Store{capacity: capacityPerModel, buffers: map[string]*Buffer{}}
}

// Get returns the buffer for id, creating it on first use.
func (s *Store) Get(id string) *Buffer {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.buffers[id]
	if !ok {
		b = NewBuffer(s.capacity)
		s.buffers[id] = b
	}
	return b
}

func (s *Store) Remove(id string) {
	s.mu.Lock()
	delete(s.buffers, id)
	s.mu.Unlock()
}

// Runtime returns the bounded aggregate log for an inference backend.
func (s *Store) Runtime(name string) *Buffer {
	return s.Get("runtime:" + name)
}

// BindModel mirrors subsequent model output into its runtime's log, retaining
// the original timestamp and source. Runtime buffers never mirror other buffers.
func (s *Store) BindModel(id, name, runtime string) *Buffer {
	b := s.Get(id)
	destination := s.Runtime(runtime)
	b.mu.Lock()
	b.model = name
	b.mirror = destination
	b.mu.Unlock()
	return b
}
