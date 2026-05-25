package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"pusher/internal/log"
	"pusher/internal/registry"
	"pusher/internal/token"
)

type SSEHandler struct {
	registry          *registry.Registry
	validator         *token.Validator
	heartbeatInterval int
	readTimeout       int
}

func NewSSEHandler(reg *registry.Registry, v *token.Validator, heartbeatInterval, readTimeout int) *SSEHandler {
	return &SSEHandler{
		registry:          reg,
		validator:         v,
		heartbeatInterval: heartbeatInterval,
		readTimeout:       readTimeout,
	}
}

func (h *SSEHandler) Connect(c *gin.Context) {
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

		switch err {
		case token.ErrTokenExpired:
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

	conn := &registry.Connection{
		UserKey:   userKey,
		Channel:   claims.Channel,
		Group:     claims.Group,
		UUID:      claims.UUID,
		Conn:      c.Writer,
		Done:      make(chan struct{}),
		CreatedAt: time.Now(),
	}

	h.registry.Register(conn)

	log.Infow("SSE连接建立",
		"channel", claims.Channel,
		"group", claims.Group,
		"uuid", claims.UUID,
		"client_ip", c.ClientIP(),
	)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	c.Writer.Flush()

	h.waitForDisconnect(c, conn)
}

func (h *SSEHandler) heartbeat(conn *registry.Connection, beat chan<- struct{}) {
	ticker := time.NewTicker(time.Duration(h.heartbeatInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-conn.Done:
			return
		case <-ticker.C:
			conn.Conn.(http.Flusher).Flush()
			fmt.Fprintf(conn.Conn, ":heartbeat\n\n")
			conn.Conn.(http.Flusher).Flush()
			select {
			case beat <- struct{}{}:
			default:
			}
		}
	}
}

func (h *SSEHandler) waitForDisconnect(c *gin.Context, conn *registry.Connection) {
	clientGone := c.Request.Context().Done()

	beat := make(chan struct{}, 1)
	go h.heartbeat(conn, beat)

	timeout := time.NewTimer(time.Duration(h.readTimeout) * time.Second)
	defer timeout.Stop()

	for {
		select {
		case <-clientGone:
			h.disconnect(conn)
			return
		case <-conn.Done:
			h.disconnect(conn)
			return
		case <-beat:
			// 收到心跳，重置超时定时器
			if !timeout.Stop() {
				select {
				case <-timeout.C:
				default:
				}
			}
			timeout.Reset(time.Duration(h.readTimeout) * time.Second)
		case <-timeout.C:
			h.disconnect(conn)
			return
		}
	}
}

func (h *SSEHandler) disconnect(conn *registry.Connection) {
	duration := time.Since(conn.CreatedAt)
	conn.Close()
	h.registry.Unregister(conn.UserKey)

	log.Infow("SSE连接断开",
		"channel", conn.Channel,
		"group", conn.Group,
		"uuid", conn.UUID,
		"duration", fmt.Sprintf("%.0fs", duration.Seconds()),
	)
}
