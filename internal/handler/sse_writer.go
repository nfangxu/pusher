package handler

import (
	"net/http"
)

type SSEWriter struct {
	w    http.ResponseWriter
	f    http.Flusher
	done chan struct{}
}

func NewSSEWriter(w http.ResponseWriter) *SSEWriter {
	return &SSEWriter{
		w:    w,
		f:    w.(http.Flusher),
		done: make(chan struct{}),
	}
}

func (s *SSEWriter) Write(data []byte) (int, error) {
	return s.w.Write(data)
}

func (s *SSEWriter) Flush() {
	s.f.Flush()
}

func (s *SSEWriter) Close() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
}

func (s *SSEWriter) IsClosed() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *SSEWriter) Done() <-chan struct{} {
	return s.done
}
