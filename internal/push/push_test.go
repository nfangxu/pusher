package push

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"pusher/internal/log"
	"pusher/internal/registry"
)

func init() {
	log.Init("debug", "/dev/null", 7)
}

func newTestConnection(channel, group, uuid string) *registry.Connection {
	return &registry.Connection{
		UserKey:   channel + ":" + group + ":" + uuid,
		Channel:   channel,
		Group:     group,
		UUID:      uuid,
		Conn:      httptest.NewRecorder(),
		Done:      make(chan struct{}),
		CreatedAt: time.Now(),
	}
}

func TestPushQueue_Push(t *testing.T) {
	reg := registry.New(4)
	q := New(reg, 100, 2)
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
	q := New(reg, 1, 2)
	q.Start()

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
	q := New(reg, 100, 2)

	reg.Register(newTestConnection("news", "admin", "u1"))
	reg.Register(newTestConnection("sports", "user", "u2"))

	conns := q.matchTarget("*")
	if len(conns) != 2 {
		t.Errorf("matchTarget(*) returned %d connections, want 2", len(conns))
	}
}

func TestPushQueue_MatchTarget_Channel(t *testing.T) {
	reg := registry.New(4)
	q := New(reg, 100, 2)

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
	q := New(reg, 100, 2)

	reg.Register(newTestConnection("news", "admin", "u1"))
	reg.Register(newTestConnection("news", "user", "u2"))

	conns := q.matchTarget("news:admin:*")
	if len(conns) != 1 {
		t.Errorf("matchTarget(news:admin:*) returned %d connections, want 1", len(conns))
	}
}

func TestPushQueue_MatchTarget_User(t *testing.T) {
	reg := registry.New(4)
	q := New(reg, 100, 2)

	reg.Register(newTestConnection("news", "admin", "u1"))
	reg.Register(newTestConnection("news", "admin", "u2"))

	conns := q.matchTarget("news:admin:u1")
	if len(conns) != 1 {
		t.Errorf("matchTarget(news:admin:u1) returned %d connections, want 1", len(conns))
	}
}
