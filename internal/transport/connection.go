package transport

import "time"

type ConnWriter interface {
	Write([]byte) (int, error)
	Close()
	IsClosed() bool
	Done() <-chan struct{}
}

type Protocol string

const (
	ProtocolSSE Protocol = "sse"
	ProtocolWS  Protocol = "ws"
)

type Connection struct {
	UserKey   string
	Channel   string
	Group     string
	UUID      string
	Protocol  Protocol
	Conn      ConnWriter
	CreatedAt time.Time
}

func (c *Connection) Close() {
	c.Conn.Close()
}

func (c *Connection) IsClosed() bool {
	return c.Conn.IsClosed()
}
