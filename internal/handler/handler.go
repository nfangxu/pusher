package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func abortWithError(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, gin.H{"code": code, "msg": msg})
	c.Abort()
}

func successResponse(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
}
