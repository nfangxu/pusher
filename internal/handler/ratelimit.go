package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

func RateLimitMiddleware(rps int) gin.HandlerFunc {
	limiter := rate.NewLimiter(rate.Limit(rps), rps)

	return func(c *gin.Context) {
		if !limiter.Allow() {
			c.JSON(http.StatusOK, gin.H{"code": 2003, "msg": "rate limit exceeded"})
			c.Abort()
			return
		}
		c.Next()
	}
}
