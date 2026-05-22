# Pusher - 高并发 SSE 推送服务

基于 Go 语言实现的 Server-Sent Events 推送服务，支持 100 万同时在线用户。

## 目录

- [架构概览](#架构概览)
- [快速开始](#快速开始)
- [配置说明](#配置说明)
- [部署指南](#部署指南)
- [前端接入指南](#前端接入指南)
- [后台推送接入指南](#后台推送接入指南)
- [Token 生成规则](#token-生成规则)
- [API 接口文档](#api-接口文档)
- [错误码一览](#错误码一览)
- [日志说明](#日志说明)
- [性能调优](#性能调优)
- [常见问题](#常见问题)

---

## 架构概览

```
                 ┌─────────────┐
                 │   Client    │
                 │  (Browser)  │
                 └──────┬──────┘
                        │ SSE + token
                        ▼
                 ┌──────────┐      ┌─────────────────┐
                 │   HTTP    │      │   SSE Manager   │
                 │   Server  │──────│ (conn registry) │
                 │   (gin)   │      └────────┬────────┘
                 └──────┬────┘               │
                        │                    │ async fan-out
                        ▼                    │
                 ┌──────────────┐     ┌──────┴──────┐
                 │  In-Memory   │◄────│ Push Queue  │
                 │  Connection  │     │  (channel)  │
                 │   Registry   │     └─────────────┘
                 └──────────────┘
```

**核心特性：**
- SSE 长连接推送，支持 100 万并发
- 分片连接注册表，减少锁竞争
- 异步推送队列，HTTP 请求立即返回
- 支持向指定用户 / 分组 / 频道 / 全量推送
- Token 签名验证，防止伪造连接
- 推送接口限流，防止滥用

**技术栈：** Go + gin + zap + lumberjack

---

## 快速开始

### 环境要求

- Go 1.26.3+

### 编译运行

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

## 部署指南

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

### 生产环境注意事项

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

## 前端接入指南

### 基本连接

前端通过 `EventSource` API 连接推送服务。连接前需要从后端获取 Token。

```javascript
// 1. 从你的后端获取 Token（后端调用 token 生成逻辑）
const token = await fetch('/api/sse-token').then(r => r.text());

// 2. 建立 SSE 连接
const es = new EventSource(`http://your-push-server:8080/sse/connect?token=${token}`);

// 3. 监听消息
es.addEventListener('message', (event) => {
    const data = JSON.parse(event.data);
    console.log('收到推送:', data);
    // data 结构: { channel, group, uuid, message }
    // message 是后端推送时传入的原始 JSON
});

// 4. 监听连接状态
es.onopen = () => {
    console.log('SSE 连接已建立');
};

es.onerror = (err) => {
    console.error('SSE 连接错误', err);
    // EventSource 会自动重连
};
```

### 消息格式

服务端推送的消息格式为 SSE 标准格式：

```
event: message
data: {"channel":"news","group":"admin","uuid":"u1","message":{"type":"alert","content":"hello"}}
```

解析后各字段含义：

| 字段 | 说明 | 示例 |
|------|------|------|
| `channel` | 业务频道 | `"news"` |
| `group` | 用户分组 | `"admin"` |
| `uuid` | 用户唯一标识 | `"u1"` |
| `message` | 后端推送的原始消息体 | `{"type":"alert","content":"hello"}` |

### 断线重连

`EventSource` 内置自动重连机制。建议配合指数退避策略：

```javascript
let retryCount = 0;
const maxRetryDelay = 30000;

function connect() {
    const es = new EventSource(`http://your-push-server:8080/sse/connect?token=${token}`);

    es.onopen = () => {
        retryCount = 0; // 连接成功，重置计数
    };

    es.onerror = () => {
        es.close();
        retryCount++;
        const delay = Math.min(1000 * Math.pow(2, retryCount), maxRetryDelay);
        console.log(`${delay / 1000}秒后重连...`);
        setTimeout(connect, delay);
    };

    es.addEventListener('message', (event) => {
        const data = JSON.parse(event.data);
        handleMessage(data);
    });
}

connect();
```

### 多标签页处理

浏览器每个标签页会建立独立的 SSE 连接。同一用户在多个标签页连接时，服务端会保留最新连接，断开旧连接。如果你的业务需要协调多标签页，建议在前端使用 `BroadcastChannel` 或 `SharedWorker`。

### 注意事项

1. **Token 过期**：Token 有有效期（默认 3600 秒），过期后连接会被拒绝（错误码 1004），需要重新获取 Token
2. **连接超时**：服务端默认 60 秒读超时，超时后连接会断开，`EventSource` 会自动重连
3. **心跳**：服务端每 30 秒发送一次心跳注释行（`:heartbeat`），不影响消息接收
4. **CORS**：跨域连接时需要服务端配置 `cors_origins` 允许你的前端域名

---

## 后台推送接入指南

后台服务通过 HTTP POST 调用 `/push` 接口向在线用户推送消息。

### 调用方式

```bash
curl -X POST http://your-push-server:8080/push \
  -H "Authorization: Bearer your-push-token-here" \
  -H "Content-Type: application/json" \
  -d '{
    "targets": ["news:admin:u1"],
    "message": {"type": "alert", "content": "你有一条新消息"}
  }'
```

### 各语言示例

#### Go

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type PushRequest struct {
	Targets []string    `json:"targets"`
	Message interface{} `json:"message"`
}

type PushResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// Push 向推送服务发送消息
func Push(ctx context.Context, serverURL, pushToken string, targets []string, message interface{}) (*PushResponse, error) {
	reqBody, err := json.Marshal(PushRequest{
		Targets: targets,
		Message: message,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", serverURL+"/push", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+pushToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	var result PushResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := Push(ctx, "http://localhost:8080", "your-push-token-here",
		[]string{"news:admin:u1"},
		map[string]interface{}{"type": "alert", "content": "hello"},
	)
	if err != nil {
		fmt.Printf("推送失败: %v\n", err)
		return
	}
	if resp.Code != 0 {
		fmt.Printf("推送错误: code=%d msg=%s\n", resp.Code, resp.Msg)
		return
	}
	fmt.Println("推送成功")
}
```

#### PHP（cURL）

```php
<?php

/**
 * 向推送服务发送消息（cURL 版本，无需额外依赖）
 *
 * @param string $serverUrl   推送服务地址，如 http://localhost:8080
 * @param string $pushToken   推送接口认证 Token
 * @param array  $targets     推送目标，如 ["news:*", "news:admin:u1"]
 * @param array  $message     消息体，任意可 JSON 编码的数据
 * @return array 响应数组，包含 code 和 msg 字段
 * @throws RuntimeException 请求失败时抛出异常
 */
function push(string $serverUrl, string $pushToken, array $targets, array $message): array
{
    $body = json_encode([
        'targets' => $targets,
        'message' => $message,
    ], JSON_UNESCAPED_UNICODE);

    $ch = curl_init($serverUrl . '/push');
    curl_setopt_array($ch, [
        CURLOPT_POST           => true,
        CURLOPT_POSTFIELDS     => $body,
        CURLOPT_RETURNTRANSFER => true,
        CURLOPT_TIMEOUT        => 5,
        CURLOPT_HTTPHEADER     => [
            'Content-Type: application/json',
            'Authorization: Bearer ' . $pushToken,
        ],
    ]);

    $response = curl_exec($ch);
    if (curl_errno($ch)) {
        $error = curl_error($ch);
        curl_close($ch);
        throw new RuntimeException("推送请求失败: {$error}");
    }
    curl_close($ch);

    return json_decode($response, true);
}

// --- 使用示例 ---

try {
    $result = push(
        'http://localhost:8080',
        'your-push-token-here',
        ['news:admin:u1'],
        ['type' => 'alert', 'content' => 'hello']
    );

    if ($result['code'] === 0) {
        echo "推送成功\n";
    } else {
        echo "推送错误: code={$result['code']} msg={$result['msg']}\n";
    }
} catch (RuntimeException $e) {
    echo $e->getMessage() . "\n";
}
```

#### PHP（Guzzle）

需要先安装 Guzzle：`composer require guzzlehttp/guzzle`

```php
<?php

require 'vendor/autoload.php';

use GuzzleHttp\Client;
use GuzzleHttp\Exception\GuzzleException;

/**
 * 向推送服务发送消息（Guzzle 版本，推荐用于已有 Guzzle 依赖的项目）
 *
 * @param string $serverUrl   推送服务地址，如 http://localhost:8080
 * @param string $pushToken   推送接口认证 Token
 * @param array  $targets     推送目标，如 ["news:*", "news:admin:u1"]
 * @param array  $message     消息体，任意可 JSON 编码的数据
 * @return array 响应数组，包含 code 和 msg 字段
 * @throws GuzzleException 请求失败时抛出异常
 */
function push(string $serverUrl, string $pushToken, array $targets, array $message): array
{
    $client = new Client([
        'base_uri' => $serverUrl,
        'timeout'  => 5,
    ]);

    $response = $client->post('/push', [
        'headers' => [
            'Authorization' => 'Bearer ' . $pushToken,
        ],
        'json' => [
            'targets' => $targets,
            'message' => $message,
        ],
    ]);

    return json_decode($response->getBody()->getContents(), true);
}

// --- 使用示例 ---

try {
    $result = push(
        'http://localhost:8080',
        'your-push-token-here',
        ['news:admin:u1'],
        ['type' => 'alert', 'content' => 'hello']
    );

    if ($result['code'] === 0) {
        echo "推送成功\n";
    } else {
        echo "推送错误: code={$result['code']} msg={$result['msg']}\n";
    }
} catch (GuzzleException $e) {
    echo "推送请求失败: " . $e->getMessage() . "\n";
}
```

### 推送目标（targets）格式

`targets` 是一个字符串数组，支持以下格式：

| 格式 | 含义 | 示例 |
|------|------|------|
| `*` | 广播给所有在线用户 | `["*"]` |
| `{channel}:*` | 该频道下所有用户 | `["news:*"]` |
| `{channel}:{group}:*` | 该分组下所有用户 | `["news:admin:*"]` |
| `{channel}:{group}:{uuid}` | 指定用户 | `["news:admin:u1"]` |

可以在一次请求中混合使用多种格式，服务端会自动去重，同一连接不会收到重复消息：

```json
{
    "targets": ["news:*", "sports:admin:*", "system:ops:u1"],
    "message": {"type": "system", "content": "服务将于今晚 22:00 维护"}
}
```

### 消息体（message）格式

`message` 可以是任意合法 JSON，服务端不做解析，原样推送给前端：

```json
{
    "targets": ["news:admin:*"],
    "message": {
        "type": "notification",
        "title": "新文章发布",
        "content": "《Go 并发编程指南》已发布",
        "url": "https://example.com/articles/123",
        "timestamp": 1716400000
    }
}
```

### 推送响应

成功时返回：

```json
{"code": 0, "msg": "ok"}
```

失败时返回对应错误码，参见 [错误码一览](#错误码一览)。

### 最佳实践

1. **批量推送**：一次请求推送多个 target，而不是多次单目标请求
2. **错误重试**：收到 2002（队列满）时稍后重试
3. **限流控制**：推送频率不要超过服务端配置的 `push.rate_limit`
4. **消息体精简**：message 尽量精简，减少网络传输开销
5. **异步调用**：推送是非关键路径，建议后台异步调用，不要阻塞主流程

---

## Token 生成规则

Token 用于前端建立 SSE 连接时的身份验证。Token 由后端生成，返回给前端。

### 格式

```
base64(channel={channel}&group={group}&uuid={uuid}&ts={unix_timestamp}&sign={md5_sign})
```

### 签名算法

```
sign = md5({channel}{group}{uuid}{ts}{salt})
```

注意：各字段值**直接拼接，无分隔符**。

### 生成示例

以 `channel=news`、`group=admin`、`uuid=u1`、`ts=1747830600`、`salt=mysecret` 为例：

```
拼接原文: "newsadminu11747830600mysecret"
MD5:      "a1b2c3d4e5f6..."
Token:    base64("channel=news&group=admin&uuid=u1&ts=1747830600&sign=a1b2c3d4e5f6...")
```

### 各语言 Token 生成代码

#### Go

```go
package main

import (
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"time"
)

// GenerateToken 生成 SSE 连接用的 Token
// salt: 签名盐值，需与服务端 config.yaml 中 token.salt 一致
// channel, group, uuid: 用户标识三元组
func GenerateToken(salt, channel, group, uuid string) string {
	ts := time.Now().Unix()
	signInput := fmt.Sprintf("%s%s%s%d%s", channel, group, uuid, ts, salt)
	sign := fmt.Sprintf("%x", md5.Sum([]byte(signInput)))
	token := fmt.Sprintf("channel=%s&group=%s&uuid=%s&ts=%d&sign=%s", channel, group, uuid, ts, sign)
	return base64.StdEncoding.EncodeToString([]byte(token))
}

func main() {
	token := GenerateToken("your-secret-salt-here", "news", "admin", "u1")
	fmt.Println(token)
}
```

#### PHP

```php
<?php

/**
 * 生成 SSE 连接用的 Token
 *
 * @param string $salt    签名盐值，需与服务端 config.yaml 中 token.salt 一致
 * @param string $channel 业务频道
 * @param string $group   用户分组
 * @param string $uuid    用户唯一标识
 * @return string Base64 编码的 Token
 */
function generateToken(string $salt, string $channel, string $group, string $uuid): string
{
    $ts = time();
    $signInput = $channel . $group . $uuid . $ts . $salt;
    $sign = md5($signInput);
    $token = "channel={$channel}&group={$group}&uuid={$uuid}&ts={$ts}&sign={$sign}";
    return base64_encode($token);
}

// --- 使用示例 ---

$token = generateToken('your-secret-salt-here', 'news', 'admin', 'u1');
echo $token . "\n";
```

### 注意事项

1. `salt` 必须与服务端配置的 `token.salt` 一致
2. `ts` 为 Unix 秒级时间戳，服务端会校验是否在 `token.expire_seconds` 范围内
3. Token 过期后前端需重新获取，建议在后端提供一个接口返回新 Token
4. **不要在前端生成 Token**，salt 不应暴露给客户端

---

## API 接口文档

### 1. SSE 连接

建立 SSE 长连接。

```
GET /sse/connect?token={token}
```

**参数：**

| 参数 | 位置 | 必填 | 说明 |
|------|------|------|------|
| `token` | query | 是 | 身份验证 Token |

**成功响应：**

```
HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive

event: message
data: {"channel":"news","group":"admin","uuid":"u1","message":{...}}

:heartbeat

event: message
data: {"channel":"news","group":"admin","uuid":"u1","message":{...}}
```

**失败响应：**

```json
{"code": 1001, "msg": "invalid token"}
{"code": 1004, "msg": "token expired"}
```

### 2. 推送消息

向指定目标推送消息。

```
POST /push
Authorization: Bearer {push_token}
Content-Type: application/json
```

**请求体：**

```json
{
    "targets": ["news:*", "sports:admin:u1"],
    "message": {"type": "alert", "content": "hello"}
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `targets` | `string[]` | 是 | 推送目标列表，支持通配符 |
| `message` | `any` | 是 | 任意 JSON，原样推送给前端 |

**成功响应：**

```json
{"code": 0, "msg": "ok"}
```

**失败响应：**

```json
{"code": 3001, "msg": "unauthorized"}
{"code": 1002, "msg": "invalid request body"}
{"code": 1003, "msg": "targets is required"}
{"code": 2002, "msg": "push queue full"}
{"code": 2003, "msg": "rate limit exceeded"}
```

### 3. 健康检查

检查服务运行状态。

```
GET /health
```

**响应：**

```json
{
    "status": "ok",
    "connections": 1234,
    "groups": 56,
    "channels": 8
}
```

---

## 错误码一览

| 错误码 | 含义 | 触发场景 |
|--------|------|----------|
| `0` | 成功 | 推送成功 |
| `1001` | Token 无效 | SSE 连接时 Token 格式错误或签名校验失败 |
| `1002` | 请求解析失败 | 推送请求体格式错误或 message 为空 |
| `1003` | Targets 为空 | 推送请求中 targets 数组为空 |
| `1004` | Token 已过期 | SSE 连接时 Token 超过有效期 |
| `2002` | 推送队列满 | 推送队列容量不足，稍后重试 |
| `2003` | 请求频率超限 | 推送接口触发限流 |
| `3001` | 未授权 | 推送接口 Authorization 头缺失或错误 |

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

## 压力测试

项目内置了压测工具，位于 `cmd/stress/`，覆盖设计文档中定义的三个测试场景。

### 编译

```bash
go build -o stress ./cmd/stress/
```

### 场景 1：连接建立压测

测试并发建立 SSE 连接的性能。

```bash
./stress connect \
  --url http://localhost:8080 \
  --salt your-secret-salt-here \
  --num 10000 \
  --concurrency 1000
```

参数：

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--url` | 服务地址 | `http://localhost:8080` |
| `--salt` | Token 签名盐值（必填） | - |
| `--num` | 总连接数 | 10000 |
| `--concurrency` | 并发 goroutine 数 | 1000 |

输出示例：

```
=== 连接压测结果 ===
总请求数:   10000
成功数:     9998
失败数:     2
成功率:     99.98%
总耗时:     2.35s
QPS:        4255
平均耗时:   12ms
P50:        8ms
P95:        45ms
P99:        120ms
```

### 场景 2：消息推送压测

测试不同推送目标维度下的延迟和吞吐量。

```bash
# 单用户推送
./stress push --salt your-secret-salt-here --token your-push-token-here \
  --num 1000 --concurrency 100 --target "news:admin:u1"

# Channel 推送
./stress push --salt your-secret-salt-here --token your-push-token-here \
  --num 1000 --concurrency 100 --target "news:*"

# 全量推送
./stress push --salt your-secret-salt-here --token your-push-token-here \
  --num 1000 --concurrency 100 --target "*"
```

参数：

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--url` | 服务地址 | `http://localhost:8080` |
| `--salt` | Token 签名盐值（必填） | - |
| `--token` | 推送接口 Token（必填） | - |
| `--num` | 推送请求数 | 1000 |
| `--concurrency` | 并发 goroutine 数 | 100 |
| `--target` | 推送目标格式 | `*` |

### 场景 3：高并发稳定性压测

测试长时间高负载下的服务稳定性。

```bash
./stress stability \
  --salt your-secret-salt-here \
  --token your-push-token-here \
  --num 100000 \
  --duration 30m \
  --qps 500
```

参数：

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--url` | 服务地址 | `http://localhost:8080` |
| `--salt` | Token 签名盐值（必填） | - |
| `--token` | 推送接口 Token（必填） | - |
| `--num` | 在线连接数 | 100000 |
| `--duration` | 压测时长 | `30m` |
| `--qps` | 目标推送 QPS | 500 |

每 10 秒输出一次快照：

```
[快照] 总推送=5000 成功=4998 失败=2
[快照] 总推送=10000 成功=9995 失败=5
```

---

## 常见问题

### Q: 连接后收不到消息？

1. 检查 Token 是否正确生成（签名算法、salt 是否匹配）
2. 检查 Token 是否过期（错误码 1004）
3. 检查推送时的 target 格式是否与连接时的 channel/group/uuid 匹配
4. 查看服务端日志确认连接是否建立成功

### Q: 推送返回 2002？

推送队列已满，可能是推送速率过高或 worker 处理不过来。解决方案：
- 增大 `sse.push_queue_capacity`
- 增大 `sse.worker_num`
- 降低推送频率

### Q: 推送返回 2003？

触发了推送接口限流。解决方案：
- 降低推送频率
- 增大 `push.rate_limit`

### Q: 同一用户多个标签页只收到一个的消息？

服务端对同一 `{channel}:{group}:{uuid}` 只保留最新连接，旧连接会被断开。如果需要多标签页都收到消息，可以考虑：
- 前端使用不同 uuid 连接
- 前端使用 `BroadcastChannel` 在标签页间同步

### Q: 如何监控服务状态？

- 健康检查接口：`GET /health` 返回当前连接数、分组数、频道数
- 日志监控：解析 JSON 日志中的 `msg` 字段，关注 `SSE连接建立`、`SSE连接断开`、`推送失败` 等关键事件
- 建议接入 Prometheus + Grafana 做更完善的监控

### Q: 支持多实例部署吗？

当前版本使用内存存储连接信息，不支持多实例共享。如果需要多实例部署，需要引入 Redis 等外部存储来同步连接注册表，或者通过负载均衡将同一 channel 的连接路由到同一实例。
