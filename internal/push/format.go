package push

import (
	"bytes"
	"strconv"

	"pusher/internal/registry"
	"pusher/internal/transport"
)

func formatMessage(buf *bytes.Buffer, conn *registry.Connection, compactMsg []byte) {
	buf.Reset()
	if conn.Protocol == transport.ProtocolSSE {
		buf.WriteString("event: message\ndata: {")
	} else {
		buf.WriteByte('{')
	}
	buf.WriteString("\"channel\":")
	buf.Write(strconv.AppendQuote(nil, conn.Channel))
	buf.WriteString(",\"group\":")
	buf.Write(strconv.AppendQuote(nil, conn.Group))
	buf.WriteString(",\"uuid\":")
	buf.Write(strconv.AppendQuote(nil, conn.UUID))
	buf.WriteString(",\"message\":")
	buf.Write(compactMsg)
	if conn.Protocol == transport.ProtocolSSE {
		buf.WriteString("}\n\n")
	} else {
		buf.WriteByte('}')
	}
}
