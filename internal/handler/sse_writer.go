package handler

import (
	"net/http"
)

type SSEWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func NewSSEWriter(w http.ResponseWriter) *SSEWriter {
	return &SSEWriter{
		w: w,
		f: w.(http.Flusher),
	}
}

func (s *SSEWriter) Write(data []byte) (int, error) {
	return s.w.Write(data)
}

func (s *SSEWriter) Close() {
	// SSE 连接不通过此处关闭，由 waitForDisconnect 管理
}

func (s *SSEWriter) IsClosed() bool {
	return false
}

func (s *SSEWriter) Done() <-chan struct{} {
	return nil
}