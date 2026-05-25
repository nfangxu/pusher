# 部署指南

## 环境要求

- Go 版本以 `go.mod` 中的 `go` 指令为准

## 编译运行

```bash
# 克隆项目
git clone <repo-url>
cd pusher

# 安装依赖
go mod tidy

# 修改配置（生产环境建议修改 token.salt 和 push.token）
vim config.yaml

# 编译
go build -o server ./cmd/server/

# 运行
./server
```

服务默认监听 `0.0.0.0:8080`。

### 验证服务

```bash
# 健康检查
curl http://localhost:8080/health
# 返回: {"status":"ok","connections":0,"groups":0,"channels":0}
```

---

## 配置说明

配置文件为项目根目录下的 `config.yaml`：

```yaml
app:
  host: "0.0.0.0"        # 监听地址
  port: 8080              # 监听端口

log:
  level: "info"           # 日志级别: debug, info, warn, error
  path: "logs/pusher.log" # 日志文件路径
  max_days: 7             # 日志保留天数

sse:
  heartbeat_interval: 30  # SSE 心跳间隔（秒），WebSocket ping 也使用 30 秒固定间隔
  read_timeout: 60        # SSE 心跳超时时间（秒），超过后主动断开
  cors_origins: "*"       # CORS 允许的来源，多个用逗号分隔
  worker_num: 8           # 推送队列消费 goroutine 数量
  push_queue_capacity: 10000  # 推送队列容量
  shard_num: 32           # 连接注册表分片数
  fan_out_workers: 200    # 并发 fan-out goroutine 数

token:
  salt: "dev-secret-salt-change-before-production"  # Token HMAC 签名密钥，代码仅校验非空
  expire_seconds: 3600                              # Token 有效期（秒）

push:
  token: "dev-push-token-change-before-production"  # 推送接口认证 Token，代码仅校验非空
  rate_limit: 100                                   # 推送接口限流（每秒请求数）
```

### 关键配置项说明

| 配置项 | 说明 | 建议值 |
|--------|------|--------|
| `sse.heartbeat_interval` | 心跳间隔，影响连接存活检测精度 | 30 |
| `sse.read_timeout` | SSE 心跳超时时间 | 60-300 |
| `sse.worker_num` | 消费 goroutine 数，影响推送吞吐量 | CPU 核心数 * 2 |
| `sse.push_queue_capacity` | 队列容量，满时新推送返回 2002 | 10000-100000 |
| `sse.shard_num` | 分片数，高并发时减少锁竞争 | CPU 核心数 * 4 |
| `sse.fan_out_workers` | 并发 fan-out goroutine 数，大量连接时降低推送延迟 | 200 |
| `token.expire_seconds` | Token 有效期，过期后客户端需重新获取 | 3600 |
| `push.rate_limit` | 推送 QPS 上限，防止单点打满 | 按业务需求设置 |

---

## 部署方式

### 方式一：直接编译部署

```bash
# 交叉编译（例如在 Mac 上编译 Linux 二进制）
GOOS=linux GOARCH=amd64 go build -o server ./cmd/server/

# 上传到服务器
scp server user@host:/opt/pusher/
scp config.yaml user@host:/opt/pusher/

# 在服务器上运行
cd /opt/pusher
./server
```

### 方式二：Docker 部署

创建 `Dockerfile`：

```dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o server ./cmd/server/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=builder /app/server .
COPY config.yaml .
EXPOSE 8080
CMD ["./server"]
```

构建和运行：

```bash
docker build -t pusher .
docker run -d \
  --name pusher \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/logs:/app/logs \
  pusher
```

### 方式三：systemd 服务

创建 `/etc/systemd/system/pusher.service`：

```ini
[Unit]
Description=Pusher Push Service
After=network.target

[Service]
Type=simple
User=pusher
Group=pusher
WorkingDirectory=/opt/pusher
ExecStart=/opt/pusher/server
Restart=always
RestartSec=5
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
```

```bash
# 创建用户
sudo useradd -r -s /sbin/nologin pusher

# 设置权限
sudo chown -R pusher:pusher /opt/pusher

# 启动服务
sudo systemctl daemon-reload
sudo systemctl enable pusher
sudo systemctl start pusher

# 查看状态
sudo systemctl status pusher
sudo journalctl -u pusher -f
```

---

## 生产环境注意事项

1. **修改密钥**：务必修改 `token.salt` 和 `push.token`，使用高强度随机字符串
2. **文件描述符**：高并发场景下需要调高系统文件描述符限制：
   ```bash
   # /etc/security/limits.conf
   * soft nofile 1048576
   * hard nofile 1048576
   ```
3. **内核参数调优**：
   ```bash
   # /etc/sysctl.conf
   net.core.somaxconn = 65535
   net.ipv4.tcp_max_syn_backlog = 65535
   net.ipv4.ip_local_port_range = 1024 65535
   net.ipv4.tcp_tw_reuse = 1
   ```
4. **CORS**：生产环境将 `cors_origins` 设置为具体域名，不要使用 `*`
5. **反向代理**：如果使用 Nginx，需要关闭 SSE 响应的缓冲，并配置 WebSocket 支持：
   ```nginx
   location /sse/ {
       proxy_pass http://pusher_backend;
       proxy_http_version 1.1;
       proxy_set_header Connection "";
       proxy_buffering off;
       proxy_cache off;
       chunked_transfer_encoding off;
   }

   location /ws/ {
       proxy_pass http://pusher_backend;
       proxy_http_version 1.1;
       proxy_set_header Upgrade $http_upgrade;
       proxy_set_header Connection "Upgrade";
       proxy_read_timeout 86400;
   }
   ```

---

## 性能调优

### 连接数相关

| 参数 | 说明 | 调优建议 |
|------|------|----------|
| 系统 `nofile` | 文件描述符上限 | 100 万连接需要至少 1048576 |
| `sse.shard_num` | 注册表分片数 | CPU 核心数 * 4，分片越多锁竞争越少 |
| `sse.heartbeat_interval` | 心跳间隔 | 不要太频繁，30 秒足够 |

### 推送性能相关

| 参数 | 说明 | 调优建议 |
|------|------|----------|
| `sse.worker_num` | 消费 goroutine 数 | CPU 核心数 * 2 |
| `sse.push_queue_capacity` | 队列容量 | 根据业务峰值设置，10000-100000 |
| `sse.fan_out_workers` | 并发 fan-out goroutine 数 | 连接数多时增大，建议 100-500 |
| `push.rate_limit` | 推送 QPS 上限 | 根据业务需求和服务器能力设置 |

> **Fan-out 说明：** 全量广播时，连接数 ≤100 走顺序发送，>100 走并发 fan-out。`fan_out_workers` 控制并发度。详见 [Fan-out 分析报告](fan-out-report.md)。

### 内存估算

每个连接大约占用 1-2 KB 内存（包含注册表索引，具体取决于 SSE/WS 协议栈开销），100 万连接约需 1-2 GB 内存。

---

## 日志说明

日志输出到 `logs/pusher.log`（可通过配置修改），格式为 JSON，便于 ELK 等日志系统采集。

### 日志级别

| 级别 | 使用场景 |
|------|----------|
| `debug` | 调试信息，生产环境建议关闭 |
| `info` | 正常运行信息（连接建立/断开、推送记录） |
| `warn` | 警告信息 |
| `error` | 错误信息（Token 校验失败、推送失败） |

### 日志条目示例

**连接建立：**
```json
{"level":"INFO","ts":"2026-05-23 10:30:00","msg":"SSE连接建立","channel":"news","group":"admin","uuid":"u1","client_ip":"192.168.1.1"}
```

**连接断开：**
```json
{"level":"INFO","ts":"2026-05-23 10:35:00","msg":"SSE连接断开","channel":"news","group":"admin","uuid":"u1","duration":"300s"}
```

**WebSocket 连接建立：**
```json
{"level":"INFO","ts":"2026-05-23 10:30:00","msg":"WebSocket连接建立","channel":"news","group":"admin","uuid":"u1","client_ip":"192.168.1.1"}
```

**推送记录：**
```json
{"level":"INFO","ts":"2026-05-23 10:31:00","msg":"推送消息","targets":["news:*"],"target_count":150}
```

**Token 校验失败：**
```json
{"level":"ERROR","ts":"2026-05-23 10:30:05","msg":"Token校验失败","error":"token expired","token":"eyJjaGFu"}
```

**推送失败：**
```json
{"level":"ERROR","ts":"2026-05-23 10:32:00","msg":"推送失败","targets":["news:*"],"error":"push queue full"}
```

### 日志轮转

- 按天自动轮转
- 默认保留 7 天日志（通过 `log.max_days` 配置）
- 使用 lumberjack 实现，无需外部 logrotate

---

## 性能监控 (pprof)

服务内置 Go 标准库 pprof 性能分析工具，默认监听 `localhost:6060`，仅本地可访问。

### 常用分析端点

| 端点 | 用途 | 命令 |
|------|------|--------|
| `/debug/pprof/` | 首页概览 | `curl http://localhost:6060/debug/pprof/` |
| `/debug/pprof/heap` | 堆内存快照 | `go tool pprof http://localhost:6060/debug/pprof/heap` |
| `/debug/pprof/goroutine` | Goroutine 栈 | `curl http://localhost:6060/debug/pprof/goroutine?debug=1` |
| `/debug/pprof/allocs` | 内存分配采样 | `go tool pprof http://localhost:6060/debug/pprof/allocs` |
| `/debug/pprof/profile` | CPU 30秒采样 | `go tool pprof http://localhost:6060/debug/pprof/profile` |
| `/debug/pprof/block` | 阻塞分析 | 需开启 `runtime.SetBlockProfileRate` |

### 常用分析命令

```bash
# 查看堆内存使用（线上出报告
go tool pprof http://localhost:6060/debug/pprof/heap
# (pprof) top 10   # 前 10 大内存占用
# (pprof) list push   # 查看 push 包详情
# (pprof) web      # 生成 SVG 调用图（需 graphviz）

# 查看 Goroutine 数量（监控
curl -s 'http://localhost:6060/debug/pprof/goroutine?debug=1 | head -1

# 30 秒 CPU 采样
go tool pprof http://localhost:6060/debug/pprof/profile

# 对比两次堆内存对比（采集
curl -s 'http://localhost:6060/debug/pprof/heap > heap1.pprof
# 5 分钟后
curl -s 'http://localhost:6060/debug/pprof/heap > heap2.pprof
go tool pprof -base heap1.pprof heap2.pprof
```

---

## 线上关注指标

### 1. 连接相关

| 指标 | 健康阈值 | 告警阈值 | 排查方向 |
|------|-----------|----------|---------|
| **Goroutine 数量 | idle: 15 + 连接数 × 3 | > 连接数 × 5 持续增长 | curl pprof goroutine，查找泄露 |
| **堆内存 (Heap InUse) | < 连接数 × 2KB | > 连接数 × 5KB 持续增长 | pprof heap，allocs |
| **活跃连接数** | 预期业务正常值 | 突增 / 突降 | GET /health |
| **连接建立成功率 | > 99.5% | < 95% | 查看 Token 校验日志 |

### 2. 推送相关

| 指标 | 健康阈值 | 告警阈值 | 排查方向 |
|------|-----------|----------|---------|
| **推送成功率** | > 99.9% | < 99% | 队列是否满，连接是否有效 |
| **推送延迟 P99 | < 500ms | > 1s | fan_out_workers 是否足够，连接数是否过大 |
| **推送队列长度 | < 容量的 50% | > 容量的 80% | 增大 worker_num 或 队列阻塞 |

### 3. 系统相关

| 指标 | 健康阈值 | 告警阈值 | 排查方向 |
|------|-----------|----------|---------|
| **CPU 使用率** | < 70% | > 85% | pprof profile，检查 fan-out 并发 |
| **GC 频率 | < 10 次/秒 | > 5 次/秒 | allocs 分析分配热点 |
| **文件描述符** | < 总量的 50% | > 总量的 80% | lsof -p PID |

### 4. 故障排查清单

**内存持续增长？
1. `goroutine 数量是否稳定？curl pprof/goroutine 确认无泄露
2. `pprof heap 查看 top，定位分配热点
3. `allocs 对比两次 heap 快照，定位增长来源

**推送延迟升高？
1. `fan_out_workers 是否足够
2. `推送目标是否过大（全量广播延迟自然更高）
3. `连接数是否超过单 CPU 核心数 × 10k

**服务无法退出？
1. `确认 CloseAll 被调用（主流程已内置）
2. `查看是否有外部连接未正常关闭
