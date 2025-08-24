package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/usual2970/pushlet"
)

func main() {
	// 创建 Pushlet 实例
	p := pushlet.New()
	p.EnableDistributedMode("localhost:6379", "password", 0) // 启用分布式模式，连接到本地 Redis 实例

	// 启动消息代理
	p.Start()
	defer p.Stop()

	// 处理 SSE 连接请求
	http.HandleFunc("/events", p.HandleSSE)

	http.HandleFunc("/ws", p.HandleWebsocket)

	// 处理发送消息的请求
	http.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		topic := r.URL.Query().Get("topic")
		if topic == "" {
			topic = "default"
		}

		message := r.URL.Query().Get("message")
		if message == "" {
			http.Error(w, "Message cannot be empty", http.StatusBadRequest)
			return
		}

		// 发布消息
		p.Publish(topic, "message", message)

		fmt.Fprintf(w, "Message sent to topic %s", topic)
	})

	// 每5秒向默认主题发送时间更新
	go func() {
		for {
			time.Sleep(5 * time.Second)
			timeStr := time.Now().Format("2006-01-02 15:04:05")
			p.Publish("default", "time", timeStr)
		}
	}()

	// 启动服务器
	log.Println("Server started at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
