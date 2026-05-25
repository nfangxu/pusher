package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"pusher/internal/log"
	"pusher/internal/registry"
	"pusher/internal/token"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WsConn struct {
	conn *websocket.Conn
	done chan struct{}
}

func (c *WsConn) Write(data []byte) (int, error) {
	return len(data), c.conn.WriteMessage(websocket.TextMessage, data)
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
		Protocol:  "ws",
		Conn:      wsConn,
		Done:      make(chan struct{}),
		CreatedAt: time.Now(),
	}

	h.registry.Register(connection)

	log.Infow("WebSocket连接建立",
		"channel", claims.Channel,
		"group", claims.Group,
		"uuid", claims.UUID,
		"client_ip", c.ClientIP(),
	)

	go h.readPump(wsConn, connection)
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