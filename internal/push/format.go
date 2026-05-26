package push

import (
	"bytes"

	"pusher/internal/registry"
	"pusher/internal/transport"
)

func formatMessage(buf *bytes.Buffer, conn *registry.Connection, compactMsg []byte) {
	buf.Reset()
	if conn.Protocol == transport.ProtocolSSE {
		buf.WriteString("event: message\ndata: ")
	}
	buf.Write(compactMsg)
	if conn.Protocol == transport.ProtocolSSE {
		buf.WriteString("\n\n")
	}
}
