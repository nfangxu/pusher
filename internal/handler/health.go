package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"pusher/internal/registry"
)

type HealthHandler struct {
	registry *registry.Registry
}

func NewHealthHandler(reg *registry.Registry) *HealthHandler {
	return &HealthHandler{registry: reg}
}

func (h *HealthHandler) Health(c *gin.Context) {
	connections, groups, channels := h.registry.Stats()

	c.JSON(http.StatusOK, gin.H{
		"status":      "ok",
		"connections": connections,
		"groups":      groups,
		"channels":    channels,
	})
}
