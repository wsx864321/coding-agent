package tui

import (
	"sync"

	"github.com/wsx864321/coding-agent/internal/event"
)

type TuiSink struct {
	mu sync.Mutex
	ch chan<- event.Event
}

func (s *TuiSink) SetChan(ch chan<- event.Event) {
	s.mu.Lock()
	s.ch = ch
	s.mu.Unlock()
}

func (s *TuiSink) Emit(e event.Event) {
	s.mu.Lock()
	ch := s.ch
	s.mu.Unlock()
	if ch != nil {
		ch <- e
	}
}
