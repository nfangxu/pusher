# 部署指南

## 环境要求

- Go 1.26.3+

## 编译运行

```bash
# 克隆项目
git clone <repo-url>
cd pusher

# 安装依赖
go mod tidy

# 修改配置（至少修改 token.salt 和 push.token）
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
  heartbeat_interval: 30  # SSE 心跳间隔（秒）
  read_timeout: 60        # 连接最大存活时间（秒），超过后主动断开
  cors_origins: "*"       # CORS 允许的来源，多个用逗号分隔
  worker_num: 8           # 推送队列消费 goroutine 数量
  push_queue_capacity: 10000  # 推送队列容量
  shard_num: 32           # 连接注册表分片数

token:
  salt: "your-secret-salt-here"  # Token 签名盐值，生产环境必须修改
  expire_seconds: 3600           # Token 有效期（秒）

push:
  token: "your-push-token-here"  # 推送接口认证 Token，生产环境必须修改
  rate_limit: 100                # 推送接口限流（每秒请求数）
```

### 关键配置项说明

| 配置项 | 说明 | 建议值 |
|--------|------|--------|
| `sse.heartbeat_interval` | 心跳间隔，影响连接存活检测精度 | 30 |
| `sse.read_timeout` | 单个连接最大存活时间，0 表示不限制 | 60-300 |
| `sse.worker_num` | 消费 goroutine 数，影响推送吞吐量 | CPU 核心数 * 2 |
| `sse.push_queue_capacity` | 队列容量，满时新推送返回 2002 | 10000-100000 |
| `sse.shard_num` | 分片数，高并发时减少锁竞争 | CPU 核心数 * 4 |
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
Description=Pusher SSE Service
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
5. **反向代理**：如果使用 Nginx，需要关闭 SSE 响应的缓冲：
   ```nginx
   location /sse/ {
       proxy_pass http://pusher_backend;
       proxy_http_version 1.1;
       proxy_set_header Connection "";
       proxy_buffering off;
       proxy_cache off;
       chunked_transfer_encoding off;
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
| `push.rate_limit` | 推送 QPS 上限 | 根据业务需求和服务器能力设置 |

### 内存估算

每个 SSE 连接大约占用 1-2 KB 内存（包含注册表索引），100 万连接约需 1-2 GB 内存。

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
