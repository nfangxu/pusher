# 压力测试

项目内置了压测工具，位于 `cmd/stress/`，覆盖设计文档中定义的三个测试场景。

## 编译

```bash
go build -o stress ./cmd/stress/
```

---

## 场景 1：连接建立压测

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

---

## 场景 2：消息推送压测

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

---

## 场景 3：高并发稳定性压测

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
