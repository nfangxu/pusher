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

type PushRequest struct {
	Targets []string        `json:"targets" binding:"required"`
	Message json.RawMessage `json:"message" binding:"required"`
}

func (h *PushHandler) Push(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" || authHeader != "Bearer "+h.token {
		abortWithError(c, 3001, "unauthorized")
		return
	}

	var req PushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWithError(c, 1002, "invalid request body")
		return
	}

	if len(req.Targets) == 0 {
		abortWithError(c, 1003, "targets is required")
		return
	}

	if len(req.Message) == 0 {
		abortWithError(c, 1002, "message is required")
		return
	}

	task := push.PushTask{
		Targets: req.Targets,
		Message: req.Message,
	}

	if err := h.queue.Push(task); err != nil {
		log.Errorw("推送失败", "targets", req.Targets, "error", err.Error())
		abortWithError(c, 2002, "push queue full")
		return
	}

	log.Infow("推送消息", "targets", req.Targets, "target_count", len(req.Targets))

	successResponse(c)
}
