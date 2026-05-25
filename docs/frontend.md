# 前端接入指南

## 客户端示例

`examples/html/index.html` — SSE 调试页面，支持前端 Token 生成、连接管理、实时日志。

`examples/html/ws.html` — WebSocket 调试页面，功能与 SSE 示例一致，连接方式替换为 WebSocket。

使用方式：

```bash
# SSE 示例
open examples/html/index.html

# WebSocket 示例
open examples/html/ws.html
```

---

## 基本连接

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

---

## 消息格式

### SSE 消息格式

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

### WebSocket 消息格式

WebSocket 直接接收 JSON，无 SSE 帧包装：

```json
{"channel":"news","group":"admin","uuid":"u1","message":{"type":"alert","content":"hello"}}
```

字段含义与 SSE 相同。

---

## WebSocket 连接

WebSocket 是 SSE 的替代方案，连接方式类似：

```javascript
// 1. 从你的后端获取 Token
const token = await fetch('/api/sse-token').then(r => r.text());

// 2. 建立 WebSocket 连接
const ws = new WebSocket(`ws://your-push-server:8080/ws/connect?token=${token}`);

ws.onopen = () => {
    console.log('WebSocket 连接已建立');
};

ws.onmessage = (event) => {
    const data = JSON.parse(event.data);
    console.log('收到推送:', data);
    // data 结构: { channel, group, uuid, message }
};

ws.onerror = (err) => {
    console.error('WebSocket 连接错误', err);
};

ws.onclose = () => {
    console.log('WebSocket 连接断开');
    // 可在此处实现重连逻辑
};
```

### 与 SSE 的对比

| 特性 | SSE | WebSocket |
|------|-----|-----------|
| 协议 | HTTP/1.1 | TCP |
| 单向/双向 | 单向（服务端推） | 双向 |
| 自动重连 | 内置 | 需自行实现 |
| 浏览器支持 | 几乎全部 | 几乎全部 |
| 数据格式 | SSE 帧包裹 | 原始 JSON |

### 重连示例

```javascript
function connect() {
    const ws = new WebSocket(`ws://localhost:8080/ws/connect?token=${token}`);

    ws.onclose = () => {
        setTimeout(connect, 1000); // 重连
    };
}
```

---

## 断线重连

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

---

## 多标签页处理

浏览器每个标签页会建立独立的 SSE 连接。同一用户在多个标签页连接时，服务端会保留最新连接，断开旧连接。如果你的业务需要协调多标签页，建议在前端使用 `BroadcastChannel` 或 `SharedWorker`。

---

## 注意事项

1. **Token 过期**：Token 有有效期（默认 3600 秒），过期后连接会被拒绝（错误码 1004），需要重新获取 Token
2. **连接超时**：服务端默认 60 秒读超时，超时后连接会断开，`EventSource` 会自动重连
3. **心跳**：服务端每 30 秒发送一次心跳注释行（`:heartbeat`），不影响消息接收
4. **CORS**：跨域连接时需要服务端配置 `cors_origins` 允许你的前端域名
