# Pusher 推送服务设计文档

## 概述

基于 Go 语言实现的高并发 Server-Sent Events (SSE) 推送服务，支持 100 万同时在线用户。

## 架构图

```
                    ┌─────────────┐
                    │   Client    │
                    │  (Browser) │
                    └──────┬──────┘
                           │ SSE + token
                           ▼
                    ┌──────────┐      ┌─────────────────┐
                    │   HTTP    │      │   SSE Manager   │
                    │   Server  │──────│ (conn registry) │
                    │   (gin)   │      └────────┬────────┘
                    └───────────┘               │
                           │                    │ async fan-out
                           ▼                    │
                    ┌──────────────┐     ┌──────┴──────┐
                    │  In-Memory   │◄────│ Push Queue  │
                    │  Connection  │     │  (channel)  │
                    │   Registry   │     └─────────────┘
                    └──────────────┘
```

## 客户端标识格式

```
{channel}:{group}:{uuid}
```

- `channel`：业务频道
- `group`：用户分组
- `uuid`：用户唯一标识

## 推送路由

`push` 接口的 `targets` 数组支持通配符匹配：

| 目标 | 格式 | 含义 |
|------|------|------|
| 全部用户 | `*` | 广播给所有在线用户 |
| Channel | `{channel}:*` | Channel 下所有用户 |
| Group | `{channel}:{group}:*` | Group 下所有用户 |
| 用户 | `{channel}:{group}:{uuid}` | 单个用户 |

示例：
```json
{
    "targets": ["news:*", "news:admin:u1"],
    "message": {"type": "alert", "content": "hello"}
}
```

## 数据结构

### 推送队列（异步）

```go
type PushTask struct {
    Targets []string
    Message json.RawMessage
}

// 推送队列：Push 接口写入，异步消费并 fan-out
pushQueue chan PushTask
```

**消费 goroutine 数量：** `CPU核心数 * 2`

### 连接注册表（内存）

```go
type Connection struct {
    UserID string
    Token  string
    Conn   *http.ResponseWriter
    Mu     sync.Mutex
}

// 分片注册表：按 channel 哈希分片，减少锁竞争
type Registry struct {
    shards     []*RegistryShard
    shardNum   uint64
    mu         sync.Mutex
}

type RegistryShard struct {
    byUser    map[string]*Connection    // userKey -> Connection
    byGroup   map[string][]*Connection  // groupKey -> Connections
    byChannel map[string][]*Connection  // channel -> Connections
    mu        sync.RWMutex
}
```

**分片策略：**
- 按 `channel` 哈希到不同分片
- 推送时只锁目标 channel 所在的分片，减少锁竞争
- 分片数量建议：`CPU核心数 * 4`

### 推送请求

```go
type PushRequest struct {
    Targets []string        `json:"targets"`  // ["*", "c1:*", "c1:g1:*", "c1:g1:u1"]
    Message json.RawMessage `json:"message"`  // 任意 JSON 格式
}
```

### 推送响应

```json
// 成功
{"code": 0, "msg": "ok"}

// 失败
{"code": 1001, "msg": "invalid token"}
```

### 错误码

| 错误码 | 含义 |
|--------|------|
| 0 | 成功 |
| 1001 | Token 无效 |
| 1002 | Token 解析失败 |
| 1003 | Targets 为空 |
| 1004 | Token 已过期 |
| 2001 | 服务器内部错误 |
| 2002 | 推送队列满 |
| 2003 | 请求频率超限 |
| 3001 | 推送接口未授权 |

## API 接口

### 1. SSE 连接建立

```
GET /sse/connect?token={token}

成功：SSE 数据流
失败：401 Unauthorized
```

**CORS 配置：**
```
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, POST, OPTIONS
Access-Control-Allow-Headers: Content-Type, Authorization
```

**SSE 数据格式：**
```
event: message
data: {"channel":"news","group":"admin","uuid":"u1","message":{...}}
```

### 2. 推送消息

```
POST /push
Authorization: Bearer {push_token}

请求体：
{
    "targets": ["news:*", "news:admin:u1"],
    "message": {"type": "alert", "content": "hello"}
}

响应：
{"code": 0, "msg": "ok"}

认证失败：
{"code": 3001, "msg": "unauthorized"}

队列满：
{"code": 2002, "msg": "push queue full"}
```

### 3. 健康检查

```
GET /health

响应：{"status": "ok"}
```

## Token 格式与校验

### Token 格式

```
token = base64(channel={channel}&group={group}&uuid={uuid}&ts={unix秒级时间戳}&sign={md5({channel}{group}{uuid}{ts}{salt})})
```

**说明**：签名计算为各字段值直接拼接，无分隔符。例如 channel="news"、group="admin"、uuid="u1"、ts="1747830600"、salt="mysecret" 时：
```
sign = md5("newsadminu11747830600mysecret")
```

### 校验流程

1. Base64 解码 token 字符串
2. 解析出 `channel`、`group`、`uuid`、`ts`、`sign`
3. 验证 `ts` 是否在过期时间范围内（服务端配置 `token.expire_seconds`，默认 3600 秒）
4. 重新计算 `md5(channel+group+uuid+ts+salt)` 并与 `sign` 比对
5. 校验通过后提取用户身份信息

## 推送流程

1. 客户端 POST 推送请求到 `/push`
2. 验证推送 token，失败返回 `3001`
3. 验证请求参数，失败返回错误码
4. 将 PushTask 写入 `pushQueue` channel
5. 若队列满，返回 `2002`
6. HTTP 请求立即返回 `{"code": 0, "msg": "ok"}`
7. 后台 goroutine 消费 PushTask，根据 targets 匹配连接
8. 异步 fan-out 消息到所有匹配的 SSE 连接，跳过已断开连接

## 连接生命周期

### 连接建立流程

1. 客户端请求 `GET /sse/connect?token={token}`
2. 服务端校验 token，映射到 `{channel}:{group}:{uuid}`
3. 若同一用户已有连接，先断开旧连接并移除索引
4. 注册连接到 `byUser / byGroup / byChannel` 索引
5. 返回 SSE 流，保持长连接

### 心跳机制

- 服务端每 30 秒发送一次 comment (`:heartbeat`)，检测存活
- 客户端超过 60 秒无响应则判定断线

### 断线处理

- 检测到断线立即从各索引中移除连接
- 无需客户端显式通知

### 客户端重连策略

- 浏览器端使用 `EventSource` API，自带自动重连机制
- 重连时携带同一个 token
- 服务端检测同一用户已有连接时，断开旧连接并注册新连接
- 建议客户端重连间隔：指数退避（1s → 2s → 4s → 8s，最大 30s）

## 日志规范

采用 `zap.SugaredLogger` 输出日志，支持文件写入和自动轮转。

### 日志配置

- 输出文件：`logs/pusher.log`
- 轮转策略：按天轮转，保留 7 天日志
- 输出格式：JSON（便于日志收集和分析）

日志示例：
```json
{"level":"INFO","ts":"2026-05-22 10:30:00","msg":"SSE连接建立","channel":"news","group":"admin","uuid":"u1","client_ip":"192.168.1.1"}
{"level":"INFO","ts":"2026-05-22 10:30:05","msg":"推送消息","targets":["news:*"],"target_count":100}
```

### 日志条目

**连接建立：**
```json
{"level":"INFO","ts":"2026-05-22 10:30:00","msg":"SSE连接建立","channel":"news","group":"admin","uuid":"u1","client_ip":"192.168.1.1"}
```

**连接断开：**
```json
{"level":"INFO","ts":"2026-05-22 10:30:05","msg":"SSE连接断开","channel":"news","group":"admin","uuid":"u1","duration":"5s"}
```

**推送消息：**
```json
{"level":"INFO","ts":"2026-05-22 10:30:10","msg":"推送消息","targets":["news:*"],"target_count":100}
```

**连接失败：**
```json
{"level":"ERROR","ts":"2026-05-22 10:30:15","msg":"SSE连接失败","error":"invalid token","channel":"news","group":"admin","uuid":"u1"}
```

**Token 校验失败：**
```json
{"level":"ERROR","ts":"2026-05-22 10:30:20","msg":"Token校验失败","error":"invalid signature","token":"eyJjaGFu"}
```

**推送失败：**
```json
{"level":"ERROR","ts":"2026-05-22 10:30:25","msg":"推送失败","targets":["news:*"],"error":"connection closed"}
```

**服务统计（定期）：**
```json
{"level":"INFO","ts":"2026-05-22 10:30:30","msg":"服务状态","connections":1000,"groups":5,"channels":3}
```

### 日志库选型

- 日志库：`go.uber.org/zap`
- 文件轮转：`github.com/natefinch/lumberjack`（配合 zap 的 `SyncWriter`）
- 时间格式：`"2006-01-02 15:04:05"`（通过 zap 自定义 EncoderConfig 配置）

## 配置文件

### config.yaml

```yaml
app:
  host: "0.0.0.0"
  port: 8080

log:
  level: "info"
  path: "logs/pusher.log"
  max_days: 7

sse:
  heartbeat_interval: 30  # 心跳间隔（秒）
  read_timeout: 60       # 读超时（秒）
  cors_origins: "*"      # CORS 允许的来源
  worker_num: 8          # 消费推送队列的 goroutine 数量
  push_queue_capacity: 10000  # 推送队列容量
  shard_num: 32          # 连接注册表分片数量（默认 CPU核心数*4）

token:
  salt: "your-secret-salt-here"  # Token 签名盐值
  expire_seconds: 3600         # Token 过期时间（秒）

push:
  token: "your-push-token-here"  # 推送接口认证 token
  rate_limit: 100               # 推送接口限流（每秒最大请求数）
```

## Graceful Shutdown

- 收到 SIGINT / SIGTERM 信号时，停止接收新请求
- 等待现有 SSE 连接全部断开
- 关闭推送队列，丢弃 pending 任务
- 退出前输出最终统计日志

## 技术选型

- 语言：Go
- HTTP 框架：gin（轻量 wrapper）
- SSE：原生 http.ResponseWriter
- 连接存储：内存 map + 定时清理过期连接
- 日志：zap + lumberjack

## 验收标准

### 功能验收

| 编号 | 验收项 | 验收标准 |
|------|--------|----------|
| F-001 | SSE 连接 | 前端通过 token 成功建立 SSE 连接，收到心跳 |
| F-002 | Token 校验 | 无效 token 返回 401，有效 token 连接成功 |
| F-003 | 单用户推送 | target 为具体用户时，该用户收到消息 |
| F-004 | Group 推送 | target 为 `{channel}:{group}:*` 时，该 group 所有用户收到消息 |
| F-005 | Channel 推送 | target 为 `{channel}:*` 时，该 channel 所有用户收到消息 |
| F-006 | 全量推送 | target 为 `*` 时，所有在线用户收到消息 |
| F-007 | 多 targets | 支持单次推送多个 targets，消息正确送达 |
| F-008 | 断线重连 | 用户重连后，新连接替换旧连接，旧连接被关闭 |
| F-009 | 推送接口认证 | 无 push_token 时返回 3001 |
| F-010 | 队列满处理 | 队列满时返回 2002，不丢失已有任务 |
| F-011 | 健康检查 | `/health` 接口返回服务状态 |

### 非功能验收

| 编号 | 验收项 | 验收标准 |
|------|--------|----------|
| NF-001 | 日志格式 | 日志为 JSON 格式，包含 level/ts/msg 等字段 |
| NF-002 | 日志轮转 | 日志文件按天轮转，自动删除 7 天前日志 |
| NF-003 | Graceful Shutdown | 收到 SIGTERM 后，服务正常关闭，无 panic |
| NF-004 | 内存占用 | 空载时内存占用 < 100MB |

## 性能测试

### 测试环境

| 项目 | 规格 |
|------|------|
| 操作系统 | Linux (Ubuntu 22.04) |
| CPU | 8 核 |
| 内存 | 16GB |
| Go 版本 | 1.22+ |
| 测试工具 | vegeta / wrk / 自研 Go 并发客户端 |

### 测试场景

#### 场景 1：连接建立性能

**目标**：验证 SSE 连接建立速度

```
并发数：10000
持续时间：60s
操作：建立 SSE 连接
```

**验收标准**：
- 连接成功率：≥ 99.9%
- 平均响应时间：< 100ms
- P99 响应时间：< 500ms

#### 场景 2：消息推送性能

**目标**：验证不同 targets 维度下的推送延迟

```
在线连接数：100,000
推送目标：单用户 / Group / Channel / 全量
每秒推送：1000 条
```

**验收标准**：
| 目标维度 | 平均延迟 | P99 延迟 |
|----------|----------|----------|
| 单用户 | < 10ms | < 50ms |
| Group (100人) | < 50ms | < 200ms |
| Channel (10,000人) | < 200ms | < 1s |
| 全量 (100,000人) | < 1s | < 5s |

#### 场景 3：高并发稳定性

**目标**：验证服务在高负载下的稳定性

```
在线连接数：500,000
推送频率：持续 30 分钟，每秒 500 条推送
```

**验收标准**：
- 服务无 panic / OOM
- 内存占用增长 < 20%
- 推送成功率 ≥ 99.9%

#### 场景 4：长时间运行

**目标**：验证服务长期运行稳定性

```
在线连接数：100,000
运行时长：24 小时
推送频率：每秒 100 条
```

**验收标准**：
- 服务无重启
- 内存无泄漏（内存增长 < 10%）
- 日志文件正确轮转

### 测试脚本示例

```bash
# 连接测试（vegeta）
echo "GET http://localhost:8080/sse/connect?token=xxx" | \
  vegeta attack -duration=60s -rate=10000/s | vegeta report

# 推送测试
for i in {1..1000}; do
  curl -X POST http://localhost:8080/push \
    -H "Authorization: Bearer push-token" \
    -H "Content-Type: application/json" \
    -d '{"targets":["news:*"],"message":{"type":"test","seq":'$i'}}' &
done
wait
```

## 性能指标

- 支持 100 万同时在线连接
- 标准部署（非 C10M 极限优化）
- Fire-and-forget 推送模式（不统计送达数）
- 心跳间隔 30 秒检测存活连接
- 异步队列解耦推送请求与 fan-out
- 分片锁减少并发锁竞争
- Token 过期机制防止重放攻击
- 推送接口限流防滥用