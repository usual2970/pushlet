# Pushlet

轻量级 Go 实时推送库，支持 **SSE** 与 **WebSocket**。单机模式下进程内转发；分布式模式下通过嵌入 [novaque](https://github.com/usual2970/novaque) 与共享 **MySQL** 在多个实例间同步消息。

![Go Version](https://img.shields.io/badge/Go-1.26.5+-blue.svg)
![License](https://img.shields.io/badge/License-MIT-green.svg)

## 要求

| 模式 | 依赖 |
|------|------|
| 单机 | Go 1.26.5+ |
| 分布式 | 上述 + MySQL 8.0.1+（InnoDB）、可共享的数据库 |

## 安装

```bash
go get github.com/usual2970/pushlet@v0.0.14
```

## 快速开始（单机）

```go
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/usual2970/pushlet"
)

func main() {
	p := pushlet.New()
	p.SetHeartbeatInterval(30 * time.Second)
	p.Start()
	defer p.Stop()

	http.HandleFunc("/events", p.HandleSSE)
	http.HandleFunc("/ws", p.HandleWebsocket)
	http.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		topic := r.URL.Query().Get("topic")
		if topic == "" {
			topic = "default"
		}
		msg := r.URL.Query().Get("message")
		if err := p.Publish(topic, "message", msg); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Write([]byte("ok"))
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

- SSE：`GET /events?topic=<name>`
- WebSocket：`GET /ws?topic=<name>`（可选初始主题）
- 发布：`POST /send?topic=<name>&message=<text>`

## 分布式模式

1. 每个实例持有指向**同一 MySQL** 的 `*sql.DB`。
2. 在 **`Start()` 之前** 调用 `EnableDistributedMode`（每个进程只能启用一次）。
3. 再 `Start()` / `Stop()`。

跨实例投递为 **至少一次**（at-least-once）：客户端应做幂等或去重。发布时 novaque 只对**当时已存在的 channel** 做 fan-out，新实例上线前应完成 `Start`，再接入流量。

```go
db, err := sql.Open("mysql", dsn)
if err != nil {
	log.Fatal(err)
}

p := pushlet.New()
opts := pushlet.DefaultDistributedOptions()
// opts.Channel = "pushlet-prod-1" // 可选：多副本时建议显式指定（pod 名 / 实例 ID）；留空则每进程自动生成唯一 channel
if err := p.EnableDistributedMode(db, opts); err != nil {
	log.Fatal(err)
}
p.Start()
defer p.Stop()
```

`DistributedOptions` 字段：

| 字段 | 说明 |
|------|------|
| `Channel` | novaque channel；**多副本时每实例须唯一**。留空则自动生成 `pushlet-node-<host>-<random>` |
| `RelayTopic` | relay 用的 novaque topic（默认 `pushlet-relay`） |
| `RelayPublishTTL` | 单条 relay 消息 TTL |
| `Novaque` | 传给 `novaque.Open` 的选项 |

生产环境多实例部署建议显式设置 `Channel`，便于排查与稳定标识。

### 与 v0.0.11 及更早版本的差异

- 已移除 Redis 后端；分布式仅支持 novaque + MySQL。
- `EnableDistributedMode(redisAddr, password, db int)` 已改为 `EnableDistributedMode(db *sql.DB, opts DistributedOptions) error`。
- `Publish` / `PublishToAll` 返回 `error`（MySQL/novaque 失败时会向上传递）。

## 可运行示例（双实例 + Docker）

`example/` 在**同一进程**内启动两个分布式实例（默认 **9090** / **9091**）。未设置 `PUSHLET_MYSQL_DSN` 时会用 testcontainers 拉起 MySQL 8（需要 Docker）。

```bash
cd example
go run .
```

验证跨实例推送：

```bash
# 终端 1：订阅实例 B
curl -N 'http://localhost:9091/events?topic=demo'

# 终端 2：在实例 A 发布
curl -X POST 'http://localhost:9090/send?topic=demo&message=hello'
```

环境变量：

| 变量 | 含义 |
|------|------|
| `PUSHLET_MYSQL_DSN` | 使用已有 MySQL，跳过 testcontainer |
| `PUSHLET_ADDR` | 实例 A 监听地址（默认 `:9090`） |
| `PUSHLET_ADDR_B` | 实例 B 监听地址（默认 `:9091`） |
| `PUSHLET_NOVAQUE_CHANNEL_A` / `_B` | 实例 A/B 的 novaque channel（默认 `pushlet-a` / `pushlet-b`） |

## 协议说明

### SSE

服务端输出标准 Event Stream。业务消息形如：

```text
event: message
data: <payload>

```

连接建立时会收到 `event: connected`。心跳为 SSE 注释行（`: heartbeat ...`）。

### WebSocket

- 服务端推送为 **binary**，载荷为 `topic` + 空格 + JSON `Message`。
- 客户端动态订阅使用 **binary 文本命令**（非 JSON）：
  - `SUB <topic>\n`
  - `UNSUB <topic>\n`
  - `PING\n`
- 成功时服务端回复 binary `OK`。

### Message（JSON 字段）

| 字段 | 说明 |
|------|------|
| `topic` | 主题 |
| `event` | 事件名（SSE 的 event 行） |
| `data` | 正文 |
| `timestamp` | 时间戳 |

## API 摘要

| 方法 | 说明 |
|------|------|
| `New(...Option)` | 创建实例；`WithLogger` 可注入日志 |
| `SetHeartbeatInterval` | SSE ping / WS ping 间隔 |
| `EnableDistributedMode(db, opts)` | 启用分布式（须在 `Start` 前，且仅一次） |
| `Start()` | 启动 broker（**必须先于**接受连接） |
| `Stop()` | 停止 broker 与分布式 connector |
| `HandleSSE` / `HandleWebsocket` | HTTP 处理器 |
| `Publish(topic, event, data)` | 按主题发布，返回 `error` |
| `PublishToAll(event, data)` | 广播到所有已订阅主题，返回 `error` |

Broker 未 `Start` 时注册连接会失败（HTTP 503）。客户端发送缓冲区满时会丢弃该连接并自动从 broker 注销，避免向已关闭 channel 发送。

## 项目结构

```text
pushlet/
├── pushlet.go              # HTTP 入口与 Publish API
├── broker.go               # 主题路由与分布式 relay
├── client.go               # 单连接 outbound 通道与背压
├── novaque_connector.go    # novaque relay 实现
├── distributed_connector.go
├── message.go
├── logger.go
├── example/main.go         # 双实例 + testcontainers 演示
└── internal/testmysql/     # 集成测试用 MySQL 容器（build tag: integration）
```

## 测试

```bash
go test ./...

# 需要 Docker
go test -tags=integration ./...
```

## 运维建议

- 多副本前确保每个实例已成功 `EnableDistributedMode` + `Start`，再挂负载均衡。
- 慢消费者会触发背压断开；客户端应实现重连。
- 生产环境请自行限制 CORS、`CheckOrigin`（当前默认为宽松配置，便于 demo）。
- 监控 MySQL 与 novaque 积压；relay 解码失败会打日志并丢弃该条消息。

## 许可证

MIT
