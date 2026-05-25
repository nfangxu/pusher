package push

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pusher/internal/log"
	"pusher/internal/registry"
	"pusher/internal/transport"
)

func init() {
	log.Init("debug", "/dev/null", 7)
}

type mockConnWriter struct {
	*httptest.ResponseRecorder
	done chan struct{}
}

func (m *mockConnWriter) Close() {
	m.done <- struct{}{}
}

func (m *mockConnWriter) IsClosed() bool {
	select {
	case <-m.done:
		return true
	default:
		return false
	}
}

func (m *mockConnWriter) Done() <-chan struct{} {
	return m.done
}

func newTestConnection(channel, group, uuid string) *registry.Connection {
	return &registry.Connection{
		UserKey:  channel + ":" + group + ":" + uuid,
		Channel:  channel,
		Group:    group,
		UUID:     uuid,
		Protocol: transport.ProtocolSSE,
		Conn: &mockConnWriter{
			ResponseRecorder: httptest.NewRecorder(),
			done:             make(chan struct{}),
		},
		CreatedAt: time.Now(),
	}
}

func TestPushQueue_Push(t *testing.T) {
	reg := registry.New(4)
	q := New(reg, 100, 2, 10)
	q.Start()

	task := PushTask{
		Targets: []string{"news:admin:u1"},
		Message: json.RawMessage(`{"type":"test"}`),
	}

	if err := q.Push(task); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
}

func TestPushQueue_PushFull(t *testing.T) {
	reg := registry.New(4)
	q := New(reg, 1, 2, 10)

	task := PushTask{
		Targets: []string{"news:admin:u1"},
		Message: json.RawMessage(`{"type":"test"}`),
	}

	q.Push(task)

	if err := q.Push(task); err == nil {
		t.Error("Push() expected error when queue is full")
	}
}

func TestPushQueue_MatchTarget_All(t *testing.T) {
	reg := registry.New(4)
	q := New(reg, 100, 2, 10)

	reg.Register(newTestConnection("news", "admin", "u1"))
	reg.Register(newTestConnection("sports", "user", "u2"))

	conns := q.matchTarget("*")
	if len(conns) != 2 {
		t.Errorf("matchTarget(*) returned %d connections, want 2", len(conns))
	}
}

func TestPushQueue_MatchTarget_Channel(t *testing.T) {
	reg := registry.New(4)
	q := New(reg, 100, 2, 10)

	reg.Register(newTestConnection("news", "admin", "u1"))
	reg.Register(newTestConnection("news", "user", "u2"))
	reg.Register(newTestConnection("sports", "admin", "u3"))

	conns := q.matchTarget("news:*:*")
	if len(conns) != 2 {
		t.Errorf("matchTarget(news:*:*) returned %d connections, want 2", len(conns))
	}

	conns = q.matchTarget("news:*")
	if len(conns) != 2 {
		t.Errorf("matchTarget(news:*) returned %d connections, want 2", len(conns))
	}
}

func TestPushQueue_MatchTarget_Group(t *testing.T) {
	reg := registry.New(4)
	q := New(reg, 100, 2, 10)

	reg.Register(newTestConnection("news", "admin", "u1"))
	reg.Register(newTestConnection("news", "user", "u2"))

	conns := q.matchTarget("news:admin:*")
	if len(conns) != 1 {
		t.Errorf("matchTarget(news:admin:*) returned %d connections, want 1", len(conns))
	}
}

func TestPushQueue_MatchTarget_User(t *testing.T) {
	reg := registry.New(4)
	q := New(reg, 100, 2, 10)

	reg.Register(newTestConnection("news", "admin", "u1"))
	reg.Register(newTestConnection("news", "admin", "u2"))

	conns := q.matchTarget("news:admin:u1")
	if len(conns) != 1 {
		t.Errorf("matchTarget(news:admin:u1) returned %d connections, want 1", len(conns))
	}
}

func TestPushQueue_NoDuplicatePush(t *testing.T) {
	reg := registry.New(4)
	q := New(reg, 100, 2, 10)
	q.Start()

	conn := newTestConnection("news", "admin", "u1")
	reg.Register(conn)

	task := PushTask{
		Targets: []string{"news:*", "news:admin:*", "news:admin:u1"},
		Message: json.RawMessage(`{"type":"test"}`),
	}

	q.process(task)

	recorder := conn.Conn.(*mockConnWriter).ResponseRecorder
	body := recorder.Body.String()

	count := 0
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data:") {
			count++
		}
	}

	if count != 1 {
		t.Errorf("expected 1 message delivery, got %d; body:\n%s", count, body)
	}
}

func TestPushQueue_SendFormatsWebSocketAsJSON(t *testing.T) {
	reg := registry.New(4)
	q := New(reg, 100, 2, 10)
	conn := newTestConnection("news", "admin", "u1")
	conn.Protocol = transport.ProtocolWS

	q.send(conn, json.RawMessage(`{"type":"test"}`))

	body := conn.Conn.(*mockConnWriter).ResponseRecorder.Body.String()
	if strings.Contains(body, "event: message") || strings.Contains(body, "data:") {
		t.Fatalf("websocket body should not contain SSE framing: %q", body)
	}
	if body != `{"channel":"news","group":"admin","uuid":"u1","message":{"type":"test"}}` {
		t.Fatalf("websocket body = %q", body)
	}
}

func TestPushQueue_SendBatchFormatsMixedProtocols(t *testing.T) {
	reg := registry.New(4)
	q := New(reg, 100, 2, 10)
	sseConn := newTestConnection("news", "admin", "u1")
	wsConn := newTestConnection("news", "admin", "u2")
	wsConn.Protocol = transport.ProtocolWS

	q.sendBatch([]*registry.Connection{sseConn, wsConn}, []byte(`{"type":"test"}`))

	sseBody := sseConn.Conn.(*mockConnWriter).ResponseRecorder.Body.String()
	if !strings.HasPrefix(sseBody, "event: message\ndata: ") || !strings.HasSuffix(sseBody, "\n\n") {
		t.Fatalf("sse body should use SSE framing: %q", sseBody)
	}

	wsBody := wsConn.Conn.(*mockConnWriter).ResponseRecorder.Body.String()
	if strings.Contains(wsBody, "event: message") || strings.Contains(wsBody, "data:") {
		t.Fatalf("websocket body should not contain SSE framing: %q", wsBody)
	}
	if wsBody != `{"channel":"news","group":"admin","uuid":"u2","message":{"type":"test"}}` {
		t.Fatalf("websocket body = %q", wsBody)
	}
}

func TestPushQueue_SendEscapesIdentityFields(t *testing.T) {
	reg := registry.New(4)
	q := New(reg, 100, 2, 10)
	conn := newTestConnection(`news"x`, `admin\y`, "u1")
	conn.Protocol = transport.ProtocolWS

	q.send(conn, json.RawMessage(`{"type":"test"}`))

	body := conn.Conn.(*mockConnWriter).ResponseRecorder.Body.Bytes()
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("body should be valid JSON: %v; body=%q", err, string(body))
	}
	if decoded["channel"] != `news"x` || decoded["group"] != `admin\y` {
		t.Fatalf("identity fields were not preserved: %#v", decoded)
	}
}
