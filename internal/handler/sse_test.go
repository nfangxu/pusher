package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"pusher/internal/log"
	"pusher/internal/registry"
	"pusher/internal/token"
)

func init() {
	gin.SetMode(gin.TestMode)
	log.Init("debug", os.DevNull, 7)
}

func TestSSEHandler_Connect_Success(t *testing.T) {
	reg := registry.New(4)
	v := token.NewValidator("test-salt", 3600)
	h := NewSSEHandler(reg, v, 1, 10)

	tokenStr := token.Generate("test-salt", "news", "admin", "u1")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/sse/connect?token="+tokenStr, nil)

	go func() {
		h.Connect(c)
	}()

	time.Sleep(100 * time.Millisecond)

	conn := reg.GetByUser("news:admin:u1")
	if conn == nil {
		t.Fatal("connection should be registered")
	}

	conn.Close()
	time.Sleep(100 * time.Millisecond)
}

func TestSSEHandler_Connect_NoToken(t *testing.T) {
	reg := registry.New(4)
	v := token.NewValidator("test-salt", 3600)
	h := NewSSEHandler(reg, v, 1, 10)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/sse/connect", nil)

	h.Connect(c)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestSSEHandler_Connect_InvalidToken(t *testing.T) {
	reg := registry.New(4)
	v := token.NewValidator("test-salt", 3600)
	h := NewSSEHandler(reg, v, 1, 10)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/sse/connect?token=invalid", nil)

	h.Connect(c)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
