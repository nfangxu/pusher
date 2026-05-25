package registry

import (
	"sync"
	"time"
)

type ConnWriter interface {
	Write([]byte) (int, error)
	Close()
	IsClosed() bool
	Done() <-chan struct{}
}

type Connection struct {
	UserKey   string
	Channel   string
	Group     string
	UUID      string
	Protocol  string // "sse" 或 "ws"
	Conn      ConnWriter
	Done      chan struct{}
	CreatedAt time.Time
	mu        sync.Mutex
}

func (c *Connection) Close() {
	select {
	case <-c.Done:
	default:
		close(c.Done)
	}
}

func (c *Connection) IsClosed() bool {
	select {
	case <-c.Done:
		return true
	default:
		return false
	}
}