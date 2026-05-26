# API 接口文档

## 1. SSE 连接

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
data: {...}

:heartbeat

event: message
data: {...}
```

**失败响应：**

```json
{"code": 1001, "msg": "invalid token"}
{"code": 1004, "msg": "token expired"}
```

---

## 2. WebSocket 连接

建立 WebSocket 连接，功能与 SSE 等价，消息格式为纯 JSON（无 SSE 帧包装）。

```
GET /ws/connect?token={token}
```

**参数：**

| 参数 | 位置 | 必填 | 说明 |
|------|------|------|------|
| `token` | query | 是 | 身份验证 Token（与 SSE 相同） |

**成功响应：**

协议升级为 WebSocket，服务端直接推送后端传入的 message 原始 JSON：

```json
{...}
```

服务端每 30 秒发送 WebSocket Ping 保持连接。

**失败响应：**

与 SSE 相同（1001 / 1004）。

---

## 3. 推送消息

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

---

## 4. 健康检查

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
| `1001` | Token 无效 | SSE/WS 连接时 Token 格式错误或签名校验失败 |
| `1002` | 请求解析失败 | 推送请求体格式错误或 message 为空 |
| `1003` | Targets 为空 | 推送请求中 targets 数组为空 |
| `1004` | Token 已过期 | SSE/WS 连接时 Token 超过有效期 |
| `2002` | 推送队列满 | 推送队列容量不足，稍后重试 |
| `2003` | 请求频率超限 | 推送接口触发限流 |
| `3001` | 未授权 | 推送接口 Authorization 头缺失或错误 |

---

## 常见问题

### 连接后收不到消息？

1. 检查 Token 是否正确生成（签名算法、salt 是否匹配）
2. 检查 Token 是否过期（错误码 1004）
3. 检查推送时的 target 格式是否与连接时的 channel/group/uuid 匹配
4. 查看服务端日志确认连接是否建立成功

### 推送返回 2002？

推送队列已满，可能是推送速率过高或 worker 处理不过来。解决方案：
- 增大 `push.queue_capacity`
- 增大 `push.worker_num`
- 降低推送频率

### 推送返回 2003？

触发了推送接口限流。解决方案：
- 降低推送频率
- 增大 `push.rate_limit`

### 同一用户多个标签页只收到一个的消息？

服务端对同一 `{channel}:{group}:{uuid}` 只保留最新连接，旧连接会被断开。如果需要多标签页都收到消息，可以考虑：
- 前端使用不同 uuid 连接
- 前端使用 `BroadcastChannel` 在标签页间同步

### 如何监控服务状态？

- 健康检查接口：`GET /health` 返回当前连接数、分组数、频道数
- 日志监控：解析 JSON 日志中的 `msg` 字段，关注 `SSE连接建立`、`WebSocket连接建立`、连接断开、`推送失败` 等关键事件
- 建议接入 Prometheus + Grafana 做更完善的监控

### 支持多实例部署吗？

当前版本使用内存存储连接信息，不支持多实例共享。如果需要多实例部署，需要引入 Redis 等外部存储来同步连接注册表，或者通过负载均衡将同一 channel 的连接路由到同一实例。
