# Pushlet - 轻量级实时消息推送库

![Go Version](https://img.shields.io/badge/Go-1.26.5+-blue.svg)
![License](https://img.shields.io/badge/License-MIT-green.svg)
![Version](https://img.shields.io/badge/Version-v0.0.7-orange.svg)

Pushlet 是一个基于 Go 语言的轻量级实时消息推送库，同时支持 **Server-Sent Events (SSE)** 和 **WebSocket** 两种协议。它支持单机和分布式部署模式，是构建实时通知、事件流和数据更新等功能的理想选择。

## ✨ 核心特性

- 🚀 **双协议支持** - 同时支持 SSE 和 WebSocket 协议
- 📡 **多主题订阅** - 支持基于主题的消息订阅和发布
- 🌐 **分布式架构** - 通过 [novaque](https://github.com/usual2970/novaque)（MySQL）实现多实例间的消息同步
- 💪 **动态订阅** - WebSocket 客户端可通过消息动态订阅/取消订阅主题
- 🔒 **二进制传输** - WebSocket 使用二进制格式传输，提高效率
- ❤️ **心跳保活** - 自动发送心跳消息保持连接活跃
- 🔧 **简单易用** - 简洁的 API 设计，易于集成到现有项目
- ⚡ **低延迟** - 消息实时推送，适合需要即时反馈的场景
- 🛡️ **高可靠** - 断线自动重连，消息不丢失
- 📊 **可观测** - 内置日志系统，支持自定义日志记录器

## 📦 安装

```bash
go get github.com/usual2970/pushlet
```

## 🚀 快速开始

### 基本用法

```go
package main

import (
    "log"
    "net/http"
    "time"

    "github.com/usual2970/pushlet"
)

func main() {
    // 创建 Pushlet 实例
    p := pushlet.New()
    
    // 设置心跳间隔
    p.SetHeartbeatInterval(30 * time.Second)
    
    // 启动消息代理
    p.Start()
    defer p.Stop()

    // SSE 端点
    http.HandleFunc("/events", p.HandleSSE)
    
    // WebSocket 端点
    http.HandleFunc("/ws", p.HandleWebSocket)
    
    // 消息发送接口
    http.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
        topic := r.URL.Query().Get("topic")
        if topic == "" {
            topic = "default"
        }
        
        message := r.URL.Query().Get("message")
        // 同时发送到 SSE 和 WebSocket 客户端
        p.Publish(topic, "message", message)
        
        w.Write([]byte("Message sent"))
    })

    log.Println("Server started at http://localhost:8080")
    log.Println("SSE endpoint: http://localhost:8080/events")
    log.Println("WebSocket endpoint: ws://localhost:8080/ws")
    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

### 分布式部署

需要 **Go 1.26.5+**、**MySQL 8.0.1+**，以及可共享的 MySQL 数据库。分布式投递为 **至少一次**（at-least-once），客户端应容忍重复事件。

```go
package main

import (
    "database/sql"
    "log"
    "net/http"

    _ "github.com/go-sql-driver/mysql"
    "github.com/usual2970/pushlet"
)

func main() {
    p := pushlet.New()

    db, err := sql.Open("mysql", "user:pass@tcp(127.0.0.1:3306)/app?parseTime=true&loc=UTC")
    if err != nil {
        log.Fatal(err)
    }
    if err := p.EnableDistributedMode(db, pushlet.DefaultDistributedOptions()); err != nil {
        log.Fatalf("Failed to enable distributed mode: %v", err)
    }

    p.Start()
    defer p.Stop()

    http.HandleFunc("/events", p.HandleSSE)
    http.HandleFunc("/ws", p.HandleWebSocket)

    http.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
        topic := r.URL.Query().Get("topic")
        if topic == "" {
            topic = "default"
        }

        message := r.URL.Query().Get("message")
        p.Publish(topic, "message", message)

        w.Write([]byte("Message sent to all instances"))
    })

    log.Println("Distributed server started at http://localhost:8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

## 📋 协议对比

| 特性 | SSE | WebSocket |
|------|-----|-----------|
| 传输方向 | 单向（服务器到客户端） | 双向 |
| 数据格式 | 文本（事件流） | 二进制 JSON |
| 订阅方式 | URL 参数指定主题 | 消息动态订阅 |
| 浏览器支持 | 现代浏览器原生支持 | 现代浏览器原生支持 |
| 连接开销 | 低 | 低 |
| 协议复杂度 | 简单 | 中等 |
| 断线重连 | 浏览器自动处理 | 需要手动处理 |
| 多主题支持 | 一个连接一个主题 | 一个连接多个主题 |

## 💻 客户端使用

### SSE 客户端

```html
<!DOCTYPE html>
<html>
<head>
    <title>SSE 客户端</title>
</head>
<body>
    <div id="messages"></div>

    <script>
        // 连接到特定主题
        const evtSource = new EventSource("/events?topic=my-topic");
        
        evtSource.addEventListener("message", function(e) {
            const div = document.createElement("div");
            div.textContent = `SSE 消息: ${e.data}`;
            document.getElementById("messages").appendChild(div);
        });
        
        evtSource.onerror = function() {
            console.log("SSE 连接错误，正在重新连接...");
        };
    </script>
</body>
</html>
```

### WebSocket 客户端（动态订阅）

```html
<!DOCTYPE html>
<html>
<head>
    <title>WebSocket 动态订阅客户端</title>
</head>
<body>
    <div id="messages"></div>
    <button onclick="subscribe('topic1')">订阅 topic1</button>
    <button onclick="unsubscribe('topic1')">取消订阅 topic1</button>

    <script>
        const ws = new WebSocket("ws://localhost:8080/ws");
        
        ws.onmessage = function(event) {
            if (event.data instanceof Blob) {
                // 处理二进制数据
                event.data.arrayBuffer().then(buffer => {
                    const decoder = new TextDecoder();
                    const jsonText = decoder.decode(buffer);
                    const msg = JSON.parse(jsonText);
                    
                    const div = document.createElement("div");
                    div.textContent = `[${msg.event}] ${msg.data}`;
                    document.getElementById("messages").appendChild(div);
                });
            }
        };
        
        function subscribe(topic) {
            ws.send(JSON.stringify({
                action: 'subscribe',
                topic: topic
            }));
        }
        
        function unsubscribe(topic) {
            ws.send(JSON.stringify({
                action: 'unsubscribe', 
                topic: topic
            }));
        }
    </script>
</body>
</html>
```

### Go WebSocket 客户端

```go
package main

import (
    "encoding/json"
    "log"
    "net/url"

    "github.com/gorilla/websocket"
)

type Message struct {
    Event     string    `json:"event"`
    Data      string    `json:"data"`
    Timestamp time.Time `json:"timestamp"`
}

type SubscriptionMessage struct {
    Action string `json:"action"` // "subscribe" 或 "unsubscribe"
    Topic  string `json:"topic"`
}

func main() {
    u := url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/ws"}

    c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
    if err != nil {
        log.Fatal("连接失败:", err)
    }
    defer c.Close()

    // 订阅主题
    subMsg := SubscriptionMessage{
        Action: "subscribe",
        Topic:  "my-topic",
    }
    c.WriteJSON(subMsg)

    for {
        msgType, message, err := c.ReadMessage()
        if err != nil {
            log.Println("读取消息出错:", err)
            break
        }

        if msgType == websocket.BinaryMessage {
            // 处理二进制消息
            var msg Message
            if err := json.Unmarshal(message, &msg); err == nil {
                log.Printf("[%s] %s", msg.Event, msg.Data)
            }
        }
    }
}
```

## 🔧 API 参考

### 核心方法

#### `New(options ...Option) *Pushlet`
创建一个新的 Pushlet 实例，支持选项配置。

#### `WithLogger(newLogger NewLogger) Option`
配置自定义日志记录器。

#### `(p *Pushlet) SetHeartbeatInterval(interval time.Duration)`
设置心跳间隔时间。

#### `(p *Pushlet) Start()`
启动消息代理，开始处理消息。

#### `(p *Pushlet) Stop()`
停止消息代理，关闭所有连接。

#### `(p *Pushlet) EnableDistributedMode(db *sql.DB, opts DistributedOptions) error`
启用分布式模式，通过嵌入的 novaque 客户端将消息写入 MySQL 并在实例间同步（**破坏性变更**：不再支持 Redis 地址参数）。

#### `(p *Pushlet) HandleSSE(w http.ResponseWriter, r *http.Request)`
处理 SSE 连接请求，支持通过 `topic` 查询参数指定主题。

#### `(p *Pushlet) HandleWebSocket(w http.ResponseWriter, r *http.Request)`
处理 WebSocket 连接请求，支持动态主题订阅。

#### `(p *Pushlet) Publish(topic, event, data string)`
向指定主题发布消息。

#### `(p *Pushlet) PublishToAll(event, data string)`
向所有主题发布消息。

## 📨 消息格式

### SSE 消息格式
```
event: message
data: Hello, World!

```

### WebSocket 订阅消息格式
```json
{
  "action": "subscribe",
  "topic": "my-topic"
}
```

### WebSocket 消息格式（二进制 JSON）
```json
{
  "event": "message",
  "data": "Hello, World!",
  "timestamp": "2024-01-01T12:00:00Z"
}
```

## 🔄 动态订阅示例

WebSocket 客户端可以在连接后动态管理订阅：

```javascript
// 订阅主题
ws.send(JSON.stringify({
    action: 'subscribe',
    topic: 'user-notifications'
}));

// 取消订阅
ws.send(JSON.stringify({
    action: 'unsubscribe',
    topic: 'user-notifications'
}));
```

## 🏗️ 项目结构

```
pushlet/
├── broker.go          # 消息代理核心逻辑
├── client.go          # 客户端连接管理
├── logger.go          # 日志系统
├── message.go         # 消息结构定义
├── pushlet.go         # 主要 API 入口
├── novaque_connector.go # novaque 分布式支持
├── go.mod            # Go 模块定义
└── example/          # 使用示例
    └── dual_protocol.go
```

## 🚀 性能特点

- **WebSocket 二进制传输**：相比文本传输减少约 20-30% 的数据量
- **心跳保活**：防止代理服务器超时，提高连接稳定性
- **分布式架构**：水平扩展支持更多并发连接
- **动态订阅**：一个 WebSocket 连接可管理多个主题，减少连接数

## 🎯 使用场景

- 📱 **实时通知系统** - 用户消息、系统通知
- 📊 **实时数据监控** - 服务器状态、性能指标
- 💬 **聊天应用** - 消息推送、在线状态
- 🎮 **实时游戏** - 游戏状态同步
- 📈 **股票行情** - 实时价格推送
- 🔔 **事件提醒** - 任务提醒、日程通知

## 🛠️ 最佳实践

### 协议选择

- **只需单向推送时使用 SSE**：简单的通知、状态更新
- **需要双向通信或多主题管理时使用 WebSocket**：复杂的实时应用

### 分布式部署

- 生产环境使用高可用 MySQL，并确保各实例在发布前已启动分布式模式（novaque 按 channel 快照 fan-out）
- 设置合适的心跳间隔（建议 30-60 秒）
- 考虑负载均衡和故障转移

### 错误处理

- 客户端应实现重连逻辑
- 服务端应处理连接异常
- 使用自定义日志记录器监控系统状态

## 📄 许可证

MIT License - 详见 [LICENSE](LICENSE) 文件

## 🤝 贡献指南

欢迎提交 Issue 和 Pull Request 来改进这个项目！

1. Fork 本仓库
2. 创建你的特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交你的更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 打开一个 Pull Request

## 📞 支持

如果你在使用过程中遇到问题或有建议，请：

- 提交 [GitHub Issue](https://github.com/usual2970/pushlet/issues)
- 查看 [文档和示例](./example/)

---

**Pushlet** - 让实时消息推送变得简单高效 🚀
		
		message := r.URL.Query().Get("message")
		p.Publish(topic, "message", message)
		
		w.Write([]byte("Message sent"))
	})

	log.Println("Server started at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

### 分布式部署

```go
package main

import (
	"log"
	"net/http"

	"github.com/usual2970/pushlet"
)

func main() {
	// 创建 Pushlet 实例
	p := pushlet.New()
	
	db, _ := sql.Open("mysql", os.Getenv("PUSHLET_MYSQL_DSN"))
	if err := p.EnableDistributedMode(db, pushlet.DefaultDistributedOptions()); err != nil {
		log.Fatalf("Failed to enable distributed mode: %v", err)
	}

	p.Start()
	defer p.Stop()

	http.HandleFunc("/events", p.HandleSSE)
	
	// 消息发送接口
	http.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
		topic := r.URL.Query().Get("topic")
		if topic == "" {
			topic = "default"
		}
		
		message := r.URL.Query().Get("message")
		p.Publish(topic, "message", message)
		
		w.Write([]byte("Message sent to all instances"))
	})

	log.Println("Server started at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

## 客户端用法

```html
<!DOCTYPE html>
<html>
<head>
    <title>Pushlet 客户端示例</title>
</head>
<body>
    <h1>Pushlet 消息接收器</h1>
    <div id="messages"></div>

    <script>
        const evtSource = new EventSource("/events");
        const messagesDiv = document.getElementById("messages");
        
        // 监听默认消息
        evtSource.addEventListener("message", function(e) {
            const newElement = document.createElement("div");
            newElement.textContent = `收到消息: ${e.data}`;
            messagesDiv.appendChild(newElement);
        });
        
        // 监听自定义事件
        evtSource.addEventListener("custom-event", function(e) {
            const newElement = document.createElement("div");
            newElement.textContent = `收到自定义事件: ${e.data}`;
            newElement.style.color = "blue";
            messagesDiv.appendChild(newElement);
        });
        
        // 处理连接错误
        evtSource.onerror = function() {
            const newElement = document.createElement("div");
            newElement.textContent = "连接错误，正在重新连接...";
            newElement.style.color = "red";
            messagesDiv.appendChild(newElement);
        };
    </script>
</body>
</html>
```

## API 参考

### 核心方法

#### `New() *Pushlet`
创建一个新的 Pushlet 实例。

#### `(p *Pushlet) Start()`
启动消息代理，开始处理消息。

#### `(p *Pushlet) Stop()`
停止消息代理，关闭所有连接。

#### `(p *Pushlet) EnableDistributedMode(db *sql.DB, opts DistributedOptions) error`
启用分布式模式，通过嵌入的 novaque 客户端将消息写入 MySQL 并在实例间同步（**破坏性变更**：不再支持 Redis 地址参数）。

#### `(p *Pushlet) HandleSSE(w http.ResponseWriter, r *http.Request)`
处理 SSE 连接请求，建立实时消息通道。

#### `(p *Pushlet) Publish(topic, event, data string)`
向指定主题发布消息。

#### `(p *Pushlet) PublishToAll(event, data string)`
向所有主题发布消息。

## 高级用法

### 自定义事件类型

```go
// 发送自定义事件类型
p.Publish("user-notifications", "user-login", "User John has logged in")
```

```javascript
// 客户端监听自定义事件
evtSource.addEventListener("user-login", function(e) {
    console.log("User login event:", e.data);
});
```

### 使用主题路径

```go
// 主题可以用路径格式组织
p.Publish("users/123/notifications", "new-message", "你有一条新消息")
```

### 处理大量连接

对于高并发场景，建议使用分布式模式并结合负载均衡：

```go
p.EnableDistributedMode(db, pushlet.DefaultDistributedOptions())

// 增加缓冲区大小以处理更多连接
http.ListenAndServe(":8080", nil)
```

## 性能考虑

- SSE 连接会占用服务器资源，建议在高负载场景使用分布式部署
- 考虑为长期空闲的连接设置超时机制
- 对于超大规模部署，考虑使用消息队列作为中间层

## 贡献指南

欢迎提交 Issue 和 Pull Request 来改进这个项目。贡献前请确保：

1. 代码风格符合 Go 的规范
2. 添加测试用例
3. 更新文档

## 许可证

MIT

## 致谢

感谢所有贡献者以及 Go 社区的支持。