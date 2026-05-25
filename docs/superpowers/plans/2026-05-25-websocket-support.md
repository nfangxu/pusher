# WebSocket 支持实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有 SSE 推送基础上，新增 WebSocket 连接方式，共用统一推送逻辑

**Architecture:** 通过 `ConnWriter` 接口抽象连接写入操作，SSE 实现 `SSEWriter`，WebSocket 实现 `WsConn`，推送层 `send()` 通过接口调用无需感知连接类型。`Connection` 增加 `Protocol` 字段区分协议，在 `send()` 中拼接不同格式。

**Tech Stack:** Go + gin + gorilla/websocket + zap

---

## 文件结构

| 操作 | 文件 | 职责 |
|------|------|------|
| 新增 | `internal/registry/connection.go` | `ConnWriter` 接口 + `Connection` 结构 |
| 新增 | `internal/handler/sse_writer.go` | `SSEWriter` 实现 `ConnWriter` |
| 新增 | `internal/handler/ws.go` | `WsConn` + `WsHandler` |
| 修改 | `internal/handler/sse.go` | 使用 `SSEWriter`，移除 `Conn` 字段直接赋值 |
| 修改 | `internal/push/push.go` | `send()` 增加 Protocol 判断，SSE/WS 格式分流 |
| 修改 | `cmd/server/main.go` | 注册 `/ws/connect` 路由 |
| 修改 | `go.mod` | 添加 `gorilla/websocket` 依赖 |

---

## 任务分解

### Task 1: 定义 ConnWriter 接口并修改 Connection 结构

**Files:**
- Create: `internal/registry/connection.go`（替换原 registry.go 中的 Connection 定义）

- [ ] **Step 1: 创建 `ConnWriter` 接口和 `Connection` 结构**

```go
package registry

import (
	"net/http"
	"sync"
	"time"
)

type ConnWriter interface {
	Write([]byte) (int, error)
	Close()
	IsClosed() bool
	Done() <-chan struct{}
}

type Connection struct {
	UserKey   string
	Channel   string
	Group     string
	UUID      string
	Protocol  string // "sse" 或 "ws"
	Conn      ConnWriter
	Done      chan struct{}
	CreatedAt time.Time
	mu        sync.Mutex
}

func (c *Connection) Close() {
	select {
	case <-c.Done:
	default:
		close(c.Done)
	}
}

func (c *Connection) IsClosed() bool {
	select {
	case <-c.Done:
		return true
	default:
		return false
	}
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./internal/registry/`
Expected: PASS（无报错，connection.go 是 registry 包内文件）

- [ ] **Step 3: 提交**

```bash
git add internal/registry/connection.go
git commit -m "feat: define ConnWriter interface and Connection struct"
```

---

### Task 2: 创建 SSEWriter 实现 ConnWriter

**Files:**
- Create: `internal/handler/sse_writer.go`

- [ ] **Step 1: 创建 SSEWriter**

```go
package handler

import (
	"net/http"
)

type SSEWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func NewSSEWriter(w http.ResponseWriter) *SSEWriter {
	return &SSEWriter{
		w: w,
		f: w.(http.Flusher),
	}
}

func (s *SSEWriter) Write(data []byte) (int, error) {
	return s.w.Write(data)
}

func (s *SSEWriter) Close() {
	// SSE 连接不通过此处关闭，由 waitForDisconnect 管理
}

func (s *SSEWriter) IsClosed() bool {
	return false
}

func (s *SSEWriter) Done() <-chan struct{} {
	return nil
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./internal/handler/`
Expected: PASS

- [ ] **Step 3: 提交**

```bash
git add internal/handler/sse_writer.go
git commit -m "feat: add SSEWriter implementing ConnWriter interface"
```

---

### Task 3: 修改 SSEHandler 使用 SSEWriter

**Files:**
- Modify: `internal/handler/sse.go:63-71`

- [ ] **Step 1: 修改 SSEHandler.Connect 中的 Connection 创建**

定位到 `conn := &registry.Connection{...}`，将：

```go
Conn: c.Writer,
```

替换为：

```go
Conn:      NewSSEWriter(c.Writer),
Protocol:  "sse",
```

完整 Connection 创建变为：

```go
conn := &registry.Connection{
    UserKey:   userKey,
    Channel:   claims.Channel,
    Group:     claims.Group,
    UUID:      claims.UUID,
    Protocol:  "sse",
    Conn:      NewSSEWriter(c.Writer),
    Done:      make(chan struct{}),
    CreatedAt: time.Now(),
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./cmd/server/`
Expected: PASS

- [ ] **Step 3: 提交**

```bash
git add internal/handler/sse.go
git commit -m "refactor: SSEHandler uses SSEWriter instead of http.ResponseWriter"
```

---

### Task 4: 添加 gorilla/websocket 依赖

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: 添加依赖**

Run: `go get github.com/gorilla/websocket@v1.5.1`

- [ ] **Step 2: 验证 go.mod**

Run: `grep websocket go.mod`
Expected: `github.com/gorilla/websocket v1.5.1`

- [ ] **Step 3: 提交**

```bash
git add go.mod go.sum
git commit -m "deps: add gorilla/websocket v1.5.1"
```

---

### Task 5: 创建 WebSocket 实现 WsConn + WsHandler

**Files:**
- Create: `internal/handler/ws.go`

- [ ] **Step 1: 创建 ws.go**

```go
package handler

import (
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
```

> **注意：** 当前设计不需要双向通信，`readPump` 只读取对端消息并忽略，主要用于检测客户端主动关闭。WebSocket 心跳通过 `writePump` 或服务端 ping 实现，见 Task 7。

- [ ] **Step 2: 验证编译**

Run: `go build ./internal/handler/`
Expected: PASS

- [ ] **Step 3: 提交**

```bash
git add internal/handler/ws.go
git commit -m "feat: add WebSocket support with WsConn and WsHandler"
```

---

### Task 6: 修改 push.go 支持 SSE/WS 格式分流

**Files:**
- Modify: `internal/push/push.go`

- [ ] **Step 1: 修改 send() 函数**

找到当前的 `send()` 函数，替换为：

```go
func (q *PushQueue) send(conn *registry.Connection, message json.RawMessage) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorw("推送 panic，连接可能已断开",
				"channel", conn.Channel,
				"group", conn.Group,
				"uuid", conn.UUID,
				"recover", fmt.Sprintf("%v", r),
			)
			conn.Close()
			q.registry.Unregister(conn.UserKey)
		}
	}()

	var data []byte
	if conn.Protocol == "sse" {
		data = q.formatSSE(conn, message)
	} else {
		data = q.formatWS(conn, message)
	}

	q.writeToConn(conn, data)
}

func (q *PushQueue) formatSSE(conn *registry.Connection, message json.RawMessage) []byte {
	var msgBuf bytes.Buffer
	if err := json.Compact(&msgBuf, message); err != nil {
		log.Errorw("消息压缩失败", "error", err.Error())
		return nil
	}
	return []byte(fmt.Sprintf("event: message\ndata: {\"channel\":\"%s\",\"group\":\"%s\",\"uuid\":\"%s\",\"message\":%s}\n\n",
		conn.Channel, conn.Group, conn.UUID, msgBuf.String()))
}

func (q *PushQueue) formatWS(conn *registry.Connection, message json.RawMessage) []byte {
	var msgBuf bytes.Buffer
	if err := json.Compact(&msgBuf, message); err != nil {
		log.Errorw("消息压缩失败", "error", err.Error())
		return nil
	}
	return []byte(fmt.Sprintf("{\"channel\":\"%s\",\"group\":\"%s\",\"uuid\":\"%s\",\"message\":%s}",
		conn.Channel, conn.Group, conn.UUID, msgBuf.String()))
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./internal/push/`
Expected: PASS

- [ ] **Step 3: 提交**

```bash
git add internal/push/push.go
git commit -m "feat: push send() supports SSE/WS format dispatching"
```

---

### Task 7: 添加 WebSocket 心跳支持

**Files:**
- Modify: `internal/handler/ws.go`

- [ ] **Step 1: 在 WsHandler.Connect 中启动 writePump 心跳**

在 `go h.readPump(wsConn, connection)` 后添加：

```go
go h.writePump(wsConn)
```

- [ ] **Step 2: 添加 writePump 方法**

```go
func (h *WsHandler) writePump(wsConn *WsConn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-wsConn.done:
			return
		case <-ticker.C:
			if err := wsConn.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				wsConn.Close()
				return
			}
		}
	}
}
```

- [ ] **Step 3: 验证编译**

Run: `go build ./internal/handler/`
Expected: PASS

- [ ] **Step 4: 提交**

```bash
git add internal/handler/ws.go
git commit -m "feat: add WebSocket ping/pong heartbeat"
```

---

### Task 8: 注册 WebSocket 路由

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: 添加 WebSocket 路由**

在 `sseHandler` 创建后、`gin.SetMode` 前添加：

```go
wsHandler := handler.NewWsHandler(reg, validator)
```

找到 `r.GET("/sse/connect", sseHandler.Connect)`，在其后添加：

```go
r.GET("/ws/connect", wsHandler.Connect)
```

完整路由设置部分应为：

```go
r.GET("/sse/connect", sseHandler.Connect)
r.GET("/ws/connect", wsHandler.Connect)
r.POST("/push", handler.RateLimitMiddleware(cfg.Push.RateLimit), pushHandler.Push)
r.GET("/health", healthHandler.Health)
```

- [ ] **Step 2: 验证编译**

Run: `go build -o server ./cmd/server/`
Expected: PASS

- [ ] **Step 3: 提交**

```bash
git add cmd/server/main.go
git commit -m "feat: register /ws/connect WebSocket route"
```

---

### Task 9: 单元测试（可选：Connection 构造变化需同步更新）

**Files:**
- Modify: `internal/push/push_test.go`

- [ ] **Step 1: 检查 push_test.go 是否需要更新**

Run: `go test ./internal/push/ -v 2>&1 | head -30`

如果报错 `Connection{} literal too few`，需要在测试中补充 `Protocol` 字段。

- [ ] **Step 2: 如有错误，补充 Protocol 字段**

在 `push_test.go` 的 `newTestConnection` 函数中，Connection 字面量添加 `Protocol: "sse"`

- [ ] **Step 3: 验证测试通过**

Run: `go test ./... 2>&1 | grep -E "(PASS|FAIL|ok|---)"`

- [ ] **Step 4: 提交**

```bash
git add internal/push/push_test.go
git commit -m "test: update push tests for Connection.Protocol field"
```

---

### Task 10: 端到端验证

**验证清单（手动）：**

- [ ] **Step 1: 编译 server**

Run: `go build -o server ./cmd/server/`
Expected: PASS

- [ ] **Step 2: 启动服务**

Run: `./server &`
Expected: `push queue started with 8 workers, fan-out workers 200` 日志

- [ ] **Step 3: SSE 连接测试**

```bash
# 生成 token（参考 backend.md）
# 连接 SSE
curl -s "http://localhost:8080/sse/connect?token=<token>" &
sleep 2
# 推送消息
curl -X POST http://localhost:8080/push \
  -H "Authorization: Bearer your-push-token-here" \
  -H "Content-Type: application/json" \
  -d '{"targets":["*"],"message":{"type":"test"}}'
# 观察 SSE 端收到消息
```

- [ ] **Step 4: WebSocket 连接测试**

使用 `wscat` 或浏览器控制台：

```javascript
// 浏览器
const ws = new WebSocket('ws://localhost:8080/ws/connect?token=<token>');
ws.onmessage = (e) => console.log('收到:', JSON.parse(e.data));
// 推送消息后观察控制台
```

- [ ] **Step 5: 混合推送验证**

同时建立 SSE 和 WebSocket 连接，推送到 `*` 目标，验证两边都能收到。

---

### Task 11: 更新文档

**Files:**
- Modify: `docs/deployment.md` — 添加 Nginx WebSocket 配置
- Modify: `docs/frontend.md` — 添加 WebSocket 客户端示例
- Modify: `README.md` — 更新架构图支持双协议

- [ ] **Step 1: 更新 deployment.md**

在 Nginx 配置部分添加 WebSocket 配置（见设计文档 Section 6）

- [ ] **Step 2: 更新 frontend.md**

添加 WebSocket 连接示例代码块

- [ ] **Step 3: 提交文档更新**

```bash
git add docs/deployment.md docs/frontend.md README.md
git commit -m "docs: add WebSocket support documentation"
```

---

## 自查清单

- [ ] 所有新增文件有编译结果验证
- [ ] `Connection` 的 `Protocol` 字段在 SSE 和 WS 两处都正确赋值
- [ ] `push.go` 的 `send()` 能正确区分 SSE/WS 格式
- [ ] WebSocket 心跳 ping 正确发送
- [ ] 单元测试通过
- [ ] 端到端 SSE + WS 混合推送验证

---

## 总结

| 任务 | 文件变更 | 核心改动 |
|------|---------|---------|
| T1 | `internal/registry/connection.go` (new) | `ConnWriter` 接口 + `Connection.Protocol` |
| T2 | `internal/handler/sse_writer.go` (new) | `SSEWriter` 实现 `ConnWriter` |
| T3 | `internal/handler/sse.go` | 使用 `SSEWriter`，设置 `Protocol: "sse"` |
| T4 | `go.mod` | 添加 `gorilla/websocket` |
| T5 | `internal/handler/ws.go` (new) | `WsConn` + `WsHandler` + `readPump` |
| T6 | `internal/push/push.go` | `send()` + `formatSSE()` + `formatWS()` |
| T7 | `internal/handler/ws.go` | 添加 `writePump` 心跳 |
| T8 | `cmd/server/main.go` | 注册 `/ws/connect` 路由 |
| T9 | `internal/push/push_test.go` | 补充 `Protocol` 字段 |
| T10 | — | 端到端验证 |
| T11 | `docs/*.md` | 更新文档 |