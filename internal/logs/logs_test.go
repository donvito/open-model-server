package logs

import (
	"testing"
	"time"
)

func TestBufferRingAndTail(t *testing.T) {
	b := NewBuffer(3)
	for _, s := range []string{"a", "b", "c", "d"} {
		b.Append("stdout", s)
	}
	all := b.Tail(0)
	if len(all) != 3 || all[0].Text != "b" || all[2].Text != "d" {
		t.Fatalf("ring buffer contents = %+v", all)
	}
	last := b.Tail(2)
	if len(last) != 2 || last[0].Text != "c" {
		t.Fatalf("tail(2) = %+v", last)
	}
	b.Clear()
	if len(b.Tail(0)) != 0 {
		t.Fatal("clear should empty buffer")
	}
}

func TestBufferSubscribe(t *testing.T) {
	b := NewBuffer(10)
	ch, cancel := b.Subscribe()
	b.Systemf("hello %d", 1)
	select {
	case l := <-ch:
		if l.Source != "system" || l.Text != "hello 1" {
			t.Fatalf("got %+v", l)
		}
	case <-time.After(time.Second):
		t.Fatal("no line delivered")
	}
	cancel()
	b.Append("stdout", "after cancel")
}

func TestBufferSubscribeWithReplayHasNoGap(t *testing.T) {
	b := NewBuffer(10)
	b.Append("stdout", "before")

	replay, ch, cancel := b.SubscribeWithReplay(10)
	defer cancel()
	if len(replay) != 1 || replay[0].Text != "before" {
		t.Fatalf("replay = %+v", replay)
	}

	b.Append("stderr", "after")
	select {
	case got := <-ch:
		if got.Text != "after" {
			t.Fatalf("live line = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("replayed subscription missed live line")
	}
}

func TestStore(t *testing.T) {
	s := NewStore(5)
	s.Get("m1").Append("stdout", "x")
	if got := s.Get("m1"); len(got.Tail(0)) != 1 {
		t.Fatal("Get should return the same buffer")
	}
	s.Remove("m1")
	if len(s.Get("m1").Tail(0)) != 0 {
		t.Fatal("Remove should drop the buffer")
	}
}

func TestRuntimeLogs(t *testing.T) {
	s := NewStore(2)
	a := s.BindModel("a", "gemma", "llamacpp")
	b := s.BindModel("b", "other", "llamacpp")
	onnx := s.BindModel("c", "classifier", "onnx")
	a.Append("stderr", "loading")
	modelLine := a.Tail(1)[0]
	aggregate := s.Runtime("llamacpp").Tail(1)[0]
	if aggregate.Model != "gemma" || aggregate.Source != "stderr" || aggregate.Time != modelLine.Time || modelLine.Model != "" {
		t.Fatalf("incorrect mirrored line: %+v (original: %+v)", aggregate, modelLine)
	}
	b.Systemf("ready")
	a.Append("stdout", "request")
	onnx.Systemf("session created")
	lines := s.Runtime("llamacpp").Tail(0)
	if len(lines) != 2 || lines[0].Model != "other" || lines[1].Text != "request" {
		t.Fatalf("aggregate should be bounded and ordered: %+v", lines)
	}
	if lines := s.Runtime("onnx").Tail(0); len(lines) != 1 || lines[0].Model != "classifier" {
		t.Fatalf("runtimes must be isolated: %+v", lines)
	}
	s.Remove("a")
	if len(s.Runtime("llamacpp").Tail(0)) != 2 {
		t.Fatal("removing a model must preserve runtime history")
	}
	s.BindModel("b", "renamed", "onnx").Systemf("moved")
	if lines := s.Runtime("onnx").Tail(1); lines[0].Model != "renamed" {
		t.Fatalf("rebind did not update destination and label: %+v", lines)
	}
}
