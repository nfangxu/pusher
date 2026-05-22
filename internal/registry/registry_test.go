package registry

import (
	"net/http/httptest"
	"testing"
)

func newTestConnection(channel, group, uuid string) *Connection {
	return &Connection{
		UserKey: channel + ":" + group + ":" + uuid,
		Channel: channel,
		Group:   group,
		UUID:    uuid,
		Conn:    httptest.NewRecorder(),
		Done:    make(chan struct{}),
	}
}

func TestRegistry_Register(t *testing.T) {
	r := New(4)
	conn := newTestConnection("news", "admin", "u1")

	r.Register(conn)

	got := r.GetByUser("news:admin:u1")
	if got == nil {
		t.Fatal("GetByUser() returned nil")
	}
	if got.UserKey != conn.UserKey {
		t.Errorf("UserKey = %v, want %v", got.UserKey, conn.UserKey)
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := New(4)
	conn := newTestConnection("news", "admin", "u1")

	r.Register(conn)
	r.Unregister("news:admin:u1")

	got := r.GetByUser("news:admin:u1")
	if got != nil {
		t.Error("GetByUser() should return nil after unregister")
	}
}

func TestRegistry_GetByGroup(t *testing.T) {
	r := New(4)

	r.Register(newTestConnection("news", "admin", "u1"))
	r.Register(newTestConnection("news", "admin", "u2"))
	r.Register(newTestConnection("news", "user", "u3"))

	conns := r.GetByGroup("news", "admin")
	if len(conns) != 2 {
		t.Errorf("GetByGroup() returned %d connections, want 2", len(conns))
	}
}

func TestRegistry_GetByChannel(t *testing.T) {
	r := New(4)

	r.Register(newTestConnection("news", "admin", "u1"))
	r.Register(newTestConnection("news", "user", "u2"))
	r.Register(newTestConnection("sports", "admin", "u3"))

	conns := r.GetByChannel("news")
	if len(conns) != 2 {
		t.Errorf("GetByChannel() returned %d connections, want 2", len(conns))
	}
}

func TestRegistry_GetAll(t *testing.T) {
	r := New(4)

	r.Register(newTestConnection("news", "admin", "u1"))
	r.Register(newTestConnection("sports", "user", "u2"))

	conns := r.GetAll()
	if len(conns) != 2 {
		t.Errorf("GetAll() returned %d connections, want 2", len(conns))
	}
}

func TestRegistry_ReplaceExisting(t *testing.T) {
	r := New(4)

	conn1 := newTestConnection("news", "admin", "u1")
	r.Register(conn1)

	conn2 := newTestConnection("news", "admin", "u1")
	r.Register(conn2)

	if !conn1.IsClosed() {
		t.Error("old connection should be closed")
	}

	got := r.GetByUser("news:admin:u1")
	if got != conn2 {
		t.Error("GetByUser() should return new connection")
	}
}

func TestRegistry_Stats(t *testing.T) {
	r := New(4)

	r.Register(newTestConnection("news", "admin", "u1"))
	r.Register(newTestConnection("news", "admin", "u2"))
	r.Register(newTestConnection("sports", "user", "u3"))

	conns, groups, channels := r.Stats()
	if conns != 3 {
		t.Errorf("connections = %d, want 3", conns)
	}
	if groups != 2 {
		t.Errorf("groups = %d, want 2", groups)
	}
	if channels != 2 {
		t.Errorf("channels = %d, want 2", channels)
	}
}

func TestConnection_Close(t *testing.T) {
	conn := newTestConnection("news", "admin", "u1")

	if conn.IsClosed() {
		t.Error("connection should not be closed initially")
	}

	conn.Close()

	if !conn.IsClosed() {
		t.Error("connection should be closed after Close()")
	}

	conn.Close()
}
