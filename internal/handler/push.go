package handler

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"pusher/internal/log"
	"pusher/internal/push"
)

type PushHandler struct {
	queue *push.PushQueue
	token string
}

func NewPushHandler(queue *push.PushQueue, token string) *PushHandler {
	return &PushHandler{
		queue: queue,
		token: token,
	}
}

func (h *PushHandler) Push(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" || authHeader != "Bearer "+h.token {
		abortWithError(c, 3001, "unauthorized")
		return
	}

	var req struct {
		Targets []string       `json:"targets" binding:"required"`
		Message interface{}    `json:"message" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		abortWithError(c, 1002, "invalid request body")
		return
	}

	if len(req.Targets) == 0 {
		abortWithError(c, 1003, "targets is required")
		return
	}

	message, err := json.Marshal(req.Message)
	if err != nil {
		abortWithError(c, 1002, "invalid message format")
		return
	}

	task := push.PushTask{
		Targets: req.Targets,
		Message: message,
	}

	if err := h.queue.Push(task); err != nil {
		log.Errorw("推送失败", "targets", req.Targets, "error", err.Error())
		abortWithError(c, 2002, "push queue full")
		return
	}

	log.Infow("推送消息", "targets", req.Targets, "target_count", len(req.Targets))

	successResponse(c)
}
