package transport

import "testing"

type mockWriter struct {
	closed bool
}

func (m *mockWriter) Write([]byte) (int, error) {
	return 0, nil
}

func (m *mockWriter) Close() {
	m.closed = true
}

func (m *mockWriter) IsClosed() bool {
	return m.closed
}

func (m *mockWriter) Done() <-chan struct{} {
	return nil
}

func TestConnectionCloseDelegatesToWriter(t *testing.T) {
	writer := &mockWriter{}
	conn := &Connection{Conn: writer}

	conn.Close()

	if !conn.IsClosed() {
		t.Fatal("connection should report closed after Close")
	}
	if !writer.closed {
		t.Fatal("writer should be closed")
	}
}

func TestProtocolConstants(t *testing.T) {
	if ProtocolSSE != "sse" {
		t.Fatalf("ProtocolSSE = %q", ProtocolSSE)
	}
	if ProtocolWS != "ws" {
		t.Fatalf("ProtocolWS = %q", ProtocolWS)
	}
}
