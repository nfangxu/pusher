package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"pusher/internal/push"
	"pusher/internal/registry"
)

func newTestPushQueue() *push.PushQueue {
	reg := registry.New(4)
	q := push.New(reg, 100, 2, 10)
	q.Start()
	return q
}

func TestPushHandler_Push_Success(t *testing.T) {
	q := newTestPushQueue()
	h := NewPushHandler(q, "test-token")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := map[string]interface{}{
		"targets": []string{"news:admin:u1"},
		"message": map[string]string{"type": "test"},
	}
	jsonBody, _ := json.Marshal(body)

	c.Request, _ = http.NewRequest("POST", "/push", bytes.NewBuffer(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer test-token")

	h.Push(c)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestPushHandler_Push_Unauthorized(t *testing.T) {
	q := newTestPushQueue()
	h := NewPushHandler(q, "test-token")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := map[string]interface{}{
		"targets": []string{"news:admin:u1"},
		"message": map[string]string{"type": "test"},
	}
	jsonBody, _ := json.Marshal(body)

	c.Request, _ = http.NewRequest("POST", "/push", bytes.NewBuffer(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Push(c)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"].(float64) != 3001 {
		t.Errorf("code = %v, want 3001", resp["code"])
	}
}

func TestPushHandler_Push_EmptyTargets(t *testing.T) {
	q := newTestPushQueue()
	h := NewPushHandler(q, "test-token")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := map[string]interface{}{
		"targets": []string{},
		"message": map[string]string{"type": "test"},
	}
	jsonBody, _ := json.Marshal(body)

	c.Request, _ = http.NewRequest("POST", "/push", bytes.NewBuffer(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer test-token")

	h.Push(c)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"].(float64) != 1003 {
		t.Errorf("code = %v, want 1003", resp["code"])
	}
}

func TestHealthHandler_Health(t *testing.T) {
	reg := registry.New(4)
	h := NewHealthHandler(reg)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/health", nil)

	h.Health(c)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
}
