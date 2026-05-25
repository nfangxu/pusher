# WebSocket 支持设计方案

**日期：** 2026-05-25
**需求：** 在现有 SSE 推送基础上，新增 WebSocket 连接方式，使用统一推送逻辑

---

## 1. 背景与目标

当前服务仅支持 SSE 长连接推送。部分客户端环境（如某些移动端 H5、部分游戏引擎）WebSocket 支持度更好或更符合使用习惯。需要在不改变推送流程的前提下，支持 WebSocket 作为替代连接方式。

**约束：**
- SSE 和 WebSocket 共用同一套推送逻辑（`PushQueue`、`process`、`fanOut`、`send`）
- 不需要双向通信，WebSocket 仅接收服务端推送
- WebSocket 使用独立端点 `/ws/connect`
- 推送消息格式：WebSocket 直接发送 JSON，SSE 发送 SSE 帧格式

---

## 2. 接口抽象

### 2.1 ConnWriter 接口

```go
// internal/registry/connection.go

type ConnWriter interface {
    Write([]byte) (int, error)
    Close()
    IsClosed() bool
    Done() <-chan struct{}
}
```

### 2.2 Connection 结构（保持不变）

```go
type Connection struct {
    UserKey   string
    Channel   string
    Group     string
    UUID      string
    Conn      ConnWriter  // 接口，不再绑定 http.ResponseWriter
    Done      chan struct{}
    CreatedAt time.Time
}
```

---

## 3. SSE 实现：SSEWriter

封装 `http.ResponseWriter`，实现 `ConnWriter` 接口。

**文件：** `internal/handler/sse_writer.go`（新增）

```go
type SSEWriter struct {
    w   http.ResponseWriter
    f   http.Flusher
}

func (s *SSEWriter) Write(data []byte) (int, error) {
    return s.w.Write(data)
}

func (s *SSEWriter) Close() {
    // SSE 不需要 Close，连接由 waitForDisconnect 管理
}

func (s *SSEWriter) IsClosed() bool {
    return false // SSE 由 Done channel 控制
}

func (s *SSEWriter) Done() <-chan struct{} {
    return nil // SSE 不走此接口
}
```

> `waitForDisconnect` 仍通过 `conn.Done` channel 感知断开，无需通过 `ConnWriter`。

---

## 4. WebSocket 实现：WsConn

封装 `*websocket.Conn`，实现 `ConnWriter` 接口。

**文件：** `internal/handler/ws.go`（新增）

```go
import "github.com/gorilla/websocket"

type WsConn struct {
    conn    *websocket.Conn
    done    chan struct{}
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
```

### WsHandler.Connect

```go
func (h *WsHandler) Connect(c *gin.Context) {
    // 1. 验证 token（复用现有 validator）
    // 2. 获取 userKey/channel/group/uuid
    // 3. 升级到 WebSocket
    upgrader := websocket.Upgrader{}
    conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
    // 4. 创建 WsConn + Connection，注册到 registry
    // 5. 启动读goroutine：监听对端关闭 + 心跳 ping
}
```

### WebSocket 心跳

WebSocket 有内置 ping/pong 机制，服务端周期性发送 ping，客户端自动回复 pong。无需自定义心跳。读写超时由 `conn.SetReadDeadline` / `conn.SetWriteDeadline` 控制。

---

## 5. 推送流程（无变化）

```go
// push.go send()
conn.Conn.Write(data)  // SSE: 写 SSE 帧；WS: 写纯 JSON
```

推送层不需要知道连接类型，只调用 `ConnWriter.Write()`。SSE 的帧包装在 `process()` 中处理：

```go
// push.go process() — 对于 SSE 连接，send() 内联拼接 SSE 格式
// 对于 WS 连接，直接发送原始 message
```

两种连接的 `send()` 需要区分处理：

```go
// 方案：在 Connection 中增加 Protocol 字段
type Connection struct {
    Protocol string  // "sse" 或 "ws"
    // ...
}
```

```go
func (q *PushQueue) send(conn *Connection, message json.RawMessage) {
    var data []byte
    if conn.Protocol == "sse" {
        data = formatSSE(conn, message) // event: message\ndata: {...}\n\n
    } else {
        data = formatWS(conn, message) // {"channel":...,"message":...}
    }
    q.writeToConn(conn, data)
}
```

---

## 6. 路由与注册

**`cmd/server/main.go`：**

```go
import "github.com/gorilla/websocket"

wsHandler := handler.NewWsHandler(reg, validator)
r.GET("/ws/connect", wsHandler.Connect)
```

**Nginx 配置（WebSocket 端）：**

```nginx
location /ws/ {
    proxy_pass http://pusher_backend;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "Upgrade";
    proxy_read_timeout 86400;
}
```

---

## 7. 文件变更清单

| 操作 | 文件 |
|------|------|
| 新增 | `internal/handler/sse_writer.go` — SSEWriter 实现 ConnWriter |
| 新增 | `internal/handler/ws.go` — WsConn + WsHandler |
| 修改 | `internal/registry/connection.go` — Connection.Conn 改为 ConnWriter 接口 |
| 修改 | `internal/handler/sse.go` — SSEHandler.Connect 使用 SSEWriter |
| 修改 | `internal/push/push.go` — send() 增加 Protocol 判断，SSE/WS 格式分流 |
| 修改 | `cmd/server/main.go` — 注册 /ws/connect 路由 |
| 修改 | `go.mod` — 添加 gorilla/websocket 依赖 |

---

## 8. 消息格式对比

| 协议 | 推送消息格式 |
|------|------------|
| SSE | `event: message\ndata: {"channel":"ch","group":"g","uuid":"u","message":{...}}\n\n` |
| WebSocket | `{"channel":"ch","group":"g","uuid":"u","message":{...}}` |

---

## 9. 客户端 SDK 说明

- **SSE 客户端：** 监听 `message` 事件，解析 `event.data`
- **WebSocket 客户端：** `onmessage` 回调直接解析 JSON

两者 `message` 内容结构一致，客户端可根据环境选择连接方式。

---

## 10. 依赖

```go
github.com/gorilla/websocket v1.5.1
```

---

## 11. 验证计划

1. `go build ./cmd/server` — 编译通过
2. `go test ./...` — 单元测试通过
3. SSE 连接 + 推送验证
4. WebSocket 连接 + 推送验证
5. SSE / WS 共存时全量推送（`*` target）验证两边都能收到