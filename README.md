# Pusher - 高并发 SSE 推送服务

基于 Go 语言实现的 Server-Sent Events 推送服务，支持 100 万同时在线用户。

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

## 文档

| 文档 | 说明 | 适用角色 |
|------|------|----------|
| [部署指南](docs/deployment.md) | 编译运行、配置说明、Docker/systemd 部署、性能调优、日志 | 运维 / 后端 |
| [前端接入指南](docs/frontend.md) | SSE 连接、消息格式、断线重连、多标签页 | 前端 |
| [后台推送接入指南](docs/backend.md) | 推送 API 调用、Token 生成、Go/PHP 示例 | 后端 |
| [API 接口文档](docs/api.md) | 接口定义、错误码、常见问题 | 全部 |
| [压力测试](docs/benchmark.md) | 连接压测、推送压测、稳定性压测 | 运维 / 测试 |

## 快速开始

```bash
# 安装依赖
go mod tidy

# 修改配置（至少修改 token.salt 和 push.token）
vim config.yaml

# 编译运行
go build -o server ./cmd/server/
./server

# 验证
curl http://localhost:8080/health
```

## 项目结构

```
pusher/
├── cmd/
│   ├── server/         # 服务入口
│   └── stress/         # 压测工具
├── config.yaml         # 配置文件
├── docs/               # 文档
├── internal/
│   ├── config/         # 配置加载
│   ├── handler/        # HTTP 处理器（SSE / Push / Health）
│   ├── log/            # 日志模块
│   ├── push/           # 推送队列与 fan-out
│   ├── registry/       # 分片连接注册表
│   └── token/          # Token 生成与校验
└── logs/               # 日志输出目录
```
