# Pusher 推送服务实现计划

## 项目概述

基于 Go 语言实现高并发 SSE 推送服务，支持 100 万同时在线用户。

## 项目结构

```
pusher/
├── cmd/
│   └── server/
│       └── main.go           # 程序入口
├── internal/
│   ├── config/
│   │   └── config.go         # 配置加载
│   ├── handler/
│   │   ├── sse.go            # SSE 连接处理器
│   │   ├── push.go           # 推送处理器
│   │   └── health.go         # 健康检查处理器
│   ├── log/
│   │   └── log.go            # 日志初始化
│   ├── registry/
│   │   └── registry.go       # 连接注册表
│   ├── token/
│   │   └── token.go          # Token 校验
│   └── push/
│       └── push.go           # 推送队列与 fan-out
├── config.yaml               # 配置文件
├── go.mod
└── go.sum
```

## 错误处理模式

统一使用 `gin.H` 返回错误响应：

```go
func abortWithError(c *gin.Context, code int, msg string) {
    c.JSON(http.StatusOK, gin.H{"code": code, "msg": msg})
    c.Abort()
}
```

## 实现阶段

### 阶段 1：项目初始化

**目标**：搭建项目骨架

1. 更新 `go.mod`，添加依赖：
   - `github.com/gin-gonic/gin`
   - `go.uber.org/zap`
   - `github.com/natefinch/lumberjack`
   - `gopkg.in/yaml.v3`

2. 创建目录结构

3. 创建 `config.yaml` 配置文件

4. 创建 `cmd/server/main.go` 入口文件

**输出**：可编译的项目骨架

---

### 阶段 2：配置加载

**目标**：实现配置文件解析

1. 定义配置结构体 `Config`
2. 实现 `Load(path string) (*Config, error)` 函数
3. 支持 YAML 格式解析
4. 验证必填配置项

**输出**：`internal/config/config.go`、`internal/config/config_test.go`

---

### 阶段 3：日志模块

**目标**：实现 JSON 日志输出与文件轮转

1. 初始化 zap.Logger
2. 配置 lumberjack 文件轮转（按天，保留 7 天）
3. 自定义时间格式 `2006-01-02 15:04:05`
4. 导出全局 SugaredLogger

**输出**：`internal/log/log.go`、`internal/log/log_test.go`

---

### 阶段 4：Token 校验

**目标**：实现 Token 生成与校验

1. 实现 Token 生成函数：
   ```
   base64(channel=...&group=...&uuid=...&ts=...&sign=...)
   ```
2. 实现 Token 校验函数：
   - Base64 解码
   - 解析字段
   - 验证 ts 过期
   - 验证 sign 签名
3. 提取用户身份信息

**输出**：`internal/token/token.go`、`internal/token/token_test.go`

---

### 阶段 5：连接注册表

**目标**：实现分片内存注册表

1. 定义 `Connection` 结构体
2. 定义 `RegistryShard` 结构体
3. 实现 `Registry` 分片管理
4. 实现方法：
   - `Register(conn *Connection)` - 注册连接
   - `Unregister(userKey string)` - 移除连接
   - `GetByUser(userKey string)` - 获取单用户连接
   - `GetByGroup(channel, group string)` - 获取 Group 连接列表
   - `GetByChannel(channel string)` - 获取 Channel 连接列表
   - `GetAll()` - 获取所有连接
5. 按 channel 哈希分片

**输出**：`internal/registry/registry.go`、`internal/registry/registry_test.go`

---

### 阶段 6：SSE 连接

**目标**：实现 SSE 连接管理

1. 实现 SSE 连接建立：
   - 校验 Token
   - 替换已有连接
   - 注册到 Registry
2. 实现心跳机制：
   - 每 30 秒发送 `:heartbeat`
   - 记录最后响应时间
3. 实现断线检测：
   - 启动超时检测 goroutine
   - 超过 60 秒未收到客户端响应则判定断线
   - 自动从 Registry 移除连接

**输出**：`internal/handler/sse.go`、`internal/handler/sse_test.go`

---

### 阶段 7：推送队列

**目标**：实现异步推送与 fan-out

**依赖**：阶段 5（Registry）

1. 定义 `PushTask` 结构体
2. 创建 `pushQueue chan PushTask`
3. 实现消费 goroutine 池（worker_num 个）
4. 实现 fan-out 逻辑：
   - 解析 targets（支持 `*`、`{channel}:*`、`{channel}:{group}:*`、`{channel}:{group}:{uuid}`）
   - 通过 Registry 匹配连接
   - 发送消息
   - 跳过已断开连接

**输出**：`internal/push/push.go`、`internal/push/push_test.go`

---

### 阶段 8：HTTP API

**目标**：实现 REST API 接口

**依赖**：阶段 6（SSE）、阶段 7（Push Queue）

1. 初始化 gin Engine
2. 配置 CORS 中间件
3. 实现接口：
   - `GET /sse/connect` - SSE 连接（调用阶段 6 逻辑）
   - `POST /push` - 推送消息（调用阶段 7 逻辑）
   - `GET /health` - 健康检查
4. 实现限流中间件：
   - 使用 `golang.org/x/time/rate` 令牌桶算法
   - 配置 `push.rate_limit`（每秒最大请求数）
5. 实现统一错误响应

**输出**：`internal/handler/push.go`、`internal/handler/health.go`、`internal/handler/push_test.go`

---

### 阶段 9：Graceful Shutdown

**目标**：实现优雅退出

**依赖**：阶段 8

1. 监听 SIGINT/SIGTERM 信号
2. 停止接受新请求
3. 等待现有连接断开（超时 30 秒）
4. 关闭推送队列
5. 输出最终统计日志
6. 退出程序

**输出**：集成到 `cmd/server/main.go`

---

### 阶段 10：集成测试

**目标**：验证服务功能

1. 启动服务
2. 测试 SSE 连接
3. 测试 Token 校验
4. 测试推送消息（各维度）
5. 测试断线重连
6. 测试健康检查

**输出**：`cmd/server/main_test.go`（功能验收通过）

---

## 依赖列表

| 依赖 | 版本 | 用途 |
|------|------|------|
| github.com/gin-gonic/gin | v1.10+ | HTTP 框架 |
| go.uber.org/zap | v1.27+ | 日志库 |
| github.com/natefinch/lumberjack | v2.2+ | 日志轮转 |
| gopkg.in/yaml.v3 | v3.0+ | YAML 解析 |
| golang.org/x/time | v0.5+ | 令牌桶限流 |

## 实现顺序

```
阶段 1 → 阶段 2 → 阶段 3 → 阶段 4 → 阶段 5 → 阶段 6 → 阶段 7 → 阶段 8 → 阶段 9 → 阶段 10
```

各阶段依赖关系：
- 阶段 2-3 可并行
- 阶段 4-5 可并行
- 阶段 6 依赖 4、5
- 阶段 7 依赖 5
- 阶段 8 依赖 6、7
- 阶段 9 依赖 8

## 预计工作量

| 阶段 | 预计时间 |
|------|----------|
| 阶段 1 | 10 分钟 |
| 阶段 2 | 15 分钟 |
| 阶段 3 | 15 分钟 |
| 阶段 4 | 30 分钟 |
| 阶段 5 | 45 分钟 |
| 阶段 6 | 30 分钟 |
| 阶段 7 | 30 分钟 |
| 阶段 8 | 45 分钟 |
| 阶段 9 | 15 分钟 |
| 阶段 10 | 30 分钟 |
| **总计** | **约 3.5 小时** |