# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 构建与运行

```bash
# 构建服务端二进制
go build -o server ./cmd/server

# 构建压测工具二进制
go build -o stress ./cmd/stress

# 运行服务（默认监听 :8080，ppprof 在 localhost:6060）
./server

# 运行全部测试
go test ./...

# 运行单个包的测试
go test ./internal/config/

# 运行单个测试
go test ./internal/config/ -run TestLoad

# 压测（需要先启动 server）
./stress connect --num 1000 --salt your-salt
./stress push --num 100 --salt your-salt --token your-push-token
./stress stability --num 1000 --duration 1m --salt your-salt --token your-push-token
```

## 架构概览

基于 Gin 的 Go 推送服务。单二进制运行，分片内存注册表管理连接，异步推送队列进行 fan-out，支持 SSE 和 WebSocket。

### 请求流程

1. **客户端连接**：`GET /sse/connect?token=...` 或 `GET /ws/connect?token=...` → handler 校验 token → 注册到分片注册表 → 维持长连接
2. **后端推送**：`POST /push`（Bearer 鉴权）→ 写入缓冲队列 → worker pool 匹配目标连接 → fan-out

### 关键包

- **`cmd/server/`** — 入口：配置加载、注册表、handler、路由、优雅关闭的组装
- **`cmd/stress/`** — 压测 CLI：`connect`、`push`、`stability` 子命令
- **`internal/config/`** — YAML 配置加载、校验、默认值填充
- **`internal/handler/`** — HTTP handler：SSE/WS 连接与心跳、推送 API、限流、健康检查
- **`internal/push/`** — 异步推送队列、fan-out worker、消息格式化
- **`internal/registry/`** — 分片连接注册表（FNV-32a 哈希选 shard，每 shard RWMutex）
- **`internal/transport/`** — `ConnWriter` 接口、`Connection` 结构体、`Protocol` 枚举
- **`internal/token/`** — HMAC-SHA256 token 生成与校验（`base64(channel=...&group=...&uuid=...&ts=...&sign=hex(...))`）
- **`internal/log/`** — Zap 日志 + lumberjack 轮转，双输出（文件+stdout）

### 核心接口

`transport.ConnWriter` 是 SSE 和 WebSocket 共享推送逻辑的关键抽象：

```go
type ConnWriter interface {
    Write([]byte) (int, error)
    Close()
    IsClosed() bool
    Done() <-chan struct{}
}
```

SSE 通过 `SSEWriter`（封装 `http.ResponseWriter` + Flush）实现，WS 通过 `WsConn`（封装 `gorilla/websocket.Conn` + 写锁 `sync.Mutex`）实现。

### 推送目标通配符

| 模式 | 匹配范围 |
|------|----------|
| `*` | 全部连接 |
| `{channel}:*` | 指定 channel 下全部连接 |
| `{channel}:{group}:*` | 指定 channel + group 下全部连接 |
| `{channel}:{group}:{uuid}` | 指定单个连接 |

连接通过 `UserKey = "{channel}:{group}:{uuid}"` 标识。相同 UserKey 重连时替换旧连接。

## 配置

配置文件硬编码为 `config.yaml`（`cmd/server/main.go`），无环境变量支持。关键字段：

- `app.host` / `app.port` — 监听地址
- `token.salt` — 连接 token HMAC-SHA256 密钥（仅校验非空，生产需高强度随机值）
- `token.expire_seconds` — 连接 token 有效期
- `push.token` — `/push` API Bearer 鉴权 token（仅校验非空）
- `push.rate_limit` — 推送接口全局限流（`golang.org/x/time/rate`）
- `push.worker_num` — 推送队列消费 worker 数
- `push.queue_capacity` — 推送队列容量
- `push.fan_out_workers` — 大规模 fan-out 并发 worker 数，默认 200
- `registry.shard_num` — 注册表 shard 数
- `sse.heartbeat_interval` — SSE 心跳间隔（秒）
- `sse.read_timeout` — SSE 心跳超时时间（秒）
- `sse.cors_origins` — CORS 允许的来源

## 测试

- 测试与代码同目录（`*_test.go`），无外部 mock 框架，使用手写 mock（实现 `ConnWriter` 接口）
- handler 和 push 测试的 `init()` 设置 `gin.TestMode` 并初始化 logger 到 `/dev/null`
- 注册表测试使用 `newTestConnection()` 辅助函数，push 测试使用 `newTestPushQueue()`
- token 包使用哨兵错误（`ErrTokenExpired`、`ErrInvalidSignature` 等），handler 通过 `errors.Is()` 分支判断

## 错误处理与响应

- **HTTP 始终返回 200**，业务错误码放在 JSON body：`{"code": 1001, "msg": "..."}`
- 常见业务码：`0`=成功，`1001`=token 无效，`2001`=推送目标未找到，`2002`=队列满，`2003`=限流
- 连接 Write/Flush 路径必须有 `defer recover()` 保护（关闭连接时可能 panic）
- `PushQueue.writeToConn()` 和 `SSEHandler.safeHeartbeat()` 已有 recover 保护

## 代码规范

- Go module：`pusher`；Go 版本以 `go.mod` 为准
- 日志消息使用中文，结构化字段使用 zap 风格（`log.Infow("消息", "key", value, ...)`）
- 默认不写注释，除非解释不明显的原因、约束或规避方案
- SSE `data:` 行不能包含未处理换行，嵌入消息前使用 `json.Compact`
- 心跳通过 channel 信号重置超时 timer，不使用一次性 timer 假设
- Fan-out 策略：连接数 `<=100` 顺序推送，`>100` 并发推送
- 大批量 fan-out 中 `json.Compact` 应在同一批连接内共享一次结果
- WebSocket 写操作必须持 `sync.Mutex`（`gorilla/websocket.Conn` 非并发写安全）
- Registry `Unregister` 当 `delCount >= 1000 && delCount >= len(byUser)/2` 时重分配 map，避免内存泄漏
