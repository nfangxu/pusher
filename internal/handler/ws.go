package handler

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"pusher/internal/log"
	"pusher/internal/registry"
	"pusher/internal/token"
	"pusher/internal/transport"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WsConn struct {
	conn    *websocket.Conn
	done    chan struct{}
	writeMu sync.Mutex
}

func (c *WsConn) Write(data []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (c *WsConn) Close() {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
	c.conn.Close()
}

func (c *WsConn) IsClosed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c *WsConn) Done() <-chan struct{} {
	return c.done
}

type WsHandler struct {
	registry  *registry.Registry
	validator *token.Validator
}

func NewWsHandler(reg *registry.Registry, v *token.Validator) *WsHandler {
	return &WsHandler{
		registry:  reg,
		validator: v,
	}
}

func (h *WsHandler) Connect(c *gin.Context) {
	tokenStr := c.Query("token")
	if tokenStr == "" {
		abortWithError(c, 1001, "token is required")
		return
	}

	claims, err := h.validator.Validate(tokenStr)
	if err != nil {
		tokenPreview := tokenStr
		if len(tokenStr) > 8 {
			tokenPreview = tokenStr[:8]
		}
		log.Errorw("Token校验失败", "error", err.Error(), "token", tokenPreview)

		switch {
		case errors.Is(err, token.ErrTokenExpired):
			abortWithError(c, 1004, "token expired")
		default:
			abortWithError(c, 1001, "invalid token")
		}
		return
	}

	userKey := claims.UserKey()

	if old := h.registry.GetByUser(userKey); old != nil {
		old.Close()
		h.registry.Unregister(userKey)
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Errorw("WebSocket升级失败", "error", err.Error())
		return
	}

	wsConn := &WsConn{
		conn: conn,
		done: make(chan struct{}),
	}

	connection := &registry.Connection{
		UserKey:   userKey,
		Channel:   claims.Channel,
		Group:     claims.Group,
		UUID:      claims.UUID,
		Protocol:  transport.ProtocolWS,
		Conn:      wsConn,
		CreatedAt: time.Now(),
	}

	h.registry.Register(connection)

	log.Infow("WebSocket连接建立",
		"channel", claims.Channel,
		"group", claims.Group,
		"uuid", claims.UUID,
		"client_ip", c.ClientIP(),
	)

	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	go h.readPump(wsConn, connection)
	go h.writePump(wsConn)
}

func (h *WsHandler) readPump(wsConn *WsConn, conn *registry.Connection) {
	defer func() {
		duration := time.Since(conn.CreatedAt)
		h.registry.Unregister(conn.UserKey)
		log.Infow("WebSocket连接断开",
			"channel", conn.Channel,
			"group", conn.Group,
			"uuid", conn.UUID,
			"duration", duration.Seconds(),
		)
	}()

	for {
		_, _, err := wsConn.conn.ReadMessage()
		if err != nil {
			wsConn.Close()
			return
		}
	}
}

func (h *WsHandler) writePump(wsConn *WsConn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-wsConn.done:
			return
		case <-ticker.C:
			wsConn.writeMu.Lock()
			err := wsConn.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second))
			wsConn.writeMu.Unlock()
			if err != nil {
				log.Warnw("WebSocket ping 失败，连接可能已断开",
					"error", err.Error(),
				)
				wsConn.Close()
				return
			}
		}
	}
}
