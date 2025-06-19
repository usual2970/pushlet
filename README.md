# Pushlet - 轻量级实时消息推送库

Pushlet 是一个基于 Go 语言的轻量级实时消息推送库，使用 Server-Sent Events (SSE) 技术实现服务器到客户端的单向实时数据流。它支持单机和分布式部署模式，是构建实时通知、事件流和数据更新等功能的理想选择。

## 特性

- **基于 SSE 技术** - 使用标准的 HTTP 连接，无需 WebSocket
- **多主题订阅** - 支持基于主题的消息订阅和发布
- **分布式支持** - 通过 Redis 实现多实例间的消息同步
- **简单易用** - 简洁的 API 设计，易于集成到现有项目
- **低延迟** - 消息实时推送，适合需要即时反馈的场景
- **可靠性** - 断线自动重连，消息不丢失

## 安装

```bash
go get github.com/usual2970/pushlet
```

## 快速开始

### 基本用法

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
	
	// 启动消息代理
	p.Start()
	defer p.Stop()

	// 处理 SSE 连接请求
	http.HandleFunc("/events", p.HandleSSE)
	
	// 简单的发送消息接口
	http.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
		topic := r.URL.Query().Get("topic")
		if topic == "" {
			topic = "default"
		}
		
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
	
	// 启用分布式模式
	err := p.EnableDistributedMode("localhost:6379", "", 0)
	if err != nil {
		log.Fatalf("Failed to enable distributed mode: %v", err)
	}
	
	// 启动消息代理
	p.Start()
	defer p.Stop()

	// 处理 SSE 连接请求
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

#### `(p *Pushlet) EnableDistributedMode(redisAddr, redisPassword string, redisDB int) error`
启用分布式模式，通过 Redis 同步消息。

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
// 启用分布式模式
p.EnableDistributedMode("redis-server:6379", "password", 0)

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