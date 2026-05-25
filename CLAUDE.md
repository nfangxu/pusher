# CLAUDE.md

本文件为 Claude Code（claude.ai/code）在本仓库中工作时提供指导。

## 构建与运行

```bash
# 构建服务端二进制
go build -o server ./cmd/server

# 构建压测工具二进制
go build -o stress ./cmd/stress

# 运行服务（默认监听 :8080）
./server

# 运行全部测试
go test ./...

# 运行单个测试
go test ./internal/config/ -run TestLoad

# 压测（需要先启动 server）
./stress connect --num 1000 --salt your-salt
./stress push --num 100 --salt your-salt --token your-push-token
./stress stability --num 1000 --duration 1m --salt your-salt --token your-push-token
```

## 架构概览

这是一个基于 Gin 的 Go 推送服务。服务以单二进制运行，使用分片内存注册表管理连接，通过异步推送队列进行 fan-out，支持 SSE 和 WebSocket 两种连接方式。

### 请求流程

1. **客户端连接**：通过 `GET /sse/connect?token=...` 或 `GET /ws/connect?token=...` 建立连接。`internal/handler/` 校验 token，将连接注册到分片注册表，并维持 SSE/WebSocket 连接生命周期。
2. **后端推送**：通过 `POST /push`（Bearer 鉴权）提交推送请求。`internal/handler/push.go` 将任务写入缓冲队列，`internal/push/push.go` 的 worker pool 匹配目标连接并 fan-out。

### 关键包

- **`cmd/server/`** — 服务入口，负责配置加载、注册表、handler、路由和优雅关闭的组装。
- **`cmd/stress/`** — 压测 CLI，包含 `connect`、`push`、`stability` 子命令。
- **`internal/config/`** — YAML 配置加载与校验。
- **`internal/handler/`** — HTTP handler：SSE 连接与心跳、WebSocket 连接、推送 API、健康检查。
- **`internal/push/`** — 异步推送队列、fan-out worker、SSE/WS 消息格式化。
- **`internal/registry/`** — 分片连接注册表，基于 channel 使用 FNV-32a 哈希，每个 shard 使用 RWMutex。
- **`internal/transport/`** — 共享连接抽象、协议常量和连接生命周期接口。
- **`internal/token/`** — 类 JWT token：`base64(channel=...&group=...&uuid=...&ts=...&sign=hex(hmac_sha256(secret=salt, message={channel}{group}{uuid}{ts})))`。
- **`internal/log/`** — Zap 日志，使用 lumberjack 做日志轮转。

### 推送目标通配符

| 模式 | 匹配范围 |
|------|----------|
| `*` | 全部连接 |
| `{channel}:*` | 指定 channel 下全部连接 |
| `{channel}:{group}:*` | 指定 channel + group 下全部连接 |
| `{channel}:{group}:{uuid}` | 指定单个连接 |

### 连接身份

每个连接通过 `UserKey = "{channel}:{group}:{uuid}"` 标识。相同 UserKey 重新连接时，会替换旧连接。

## 配置

编辑 `config.yaml`。关键字段：

- `app.host` / `app.port` — 服务监听地址。
- `token.salt` — 连接 token 的 HMAC-SHA256 签名密钥；代码仅校验非空，生产环境建议使用高强度随机值。
- `token.expire_seconds` — 连接 token 有效期。
- `push.token` — `/push` API 的 Bearer 鉴权 token；代码仅校验非空，生产环境建议使用高强度随机值。
- `push.rate_limit` — 推送接口全局限流。
- `sse.worker_num` — 推送队列消费 worker 数量。
- `sse.shard_num` — 连接注册表 shard 数量。
- `sse.fan_out_workers` — 大规模 fan-out 并发 worker 数量。

## 代码规范

- Go module：`pusher`；Go 版本以 `go.mod` 中的 `go` 指令为准。
- 日志消息使用中文，结构化字段使用 zap 风格。
- 默认不写注释，除非需要解释不明显的原因、约束或规避方案。
- 所有连接 Write/Flush 路径必须有 `defer recover()` 保护，关闭连接时可能 panic。
- SSE 的 `data:` 行不能包含未处理换行；嵌入消息前使用 `json.Compact`。
- 心跳通过 channel 信号重置连接超时 timer，不使用一次性 timer 假设。
- Fan-out 策略：连接数 `<=100` 走顺序推送，`>100` 走并发推送；并发度通过 `fan_out_workers` 配置，默认 200。
- 大批量 fan-out 中，`json.Compact` 应在同一批连接内共享一次结果。
