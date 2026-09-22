package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/usual2970/pushlet"
)

func main() {
	p := pushlet.New()

	if dsn := os.Getenv("PUSHLET_MYSQL_DSN"); dsn != "" {
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			log.Fatalf("mysql open: %v", err)
		}
		db.SetMaxOpenConns(16)
		if err := p.EnableDistributedMode(db, pushlet.DefaultDistributedOptions()); err != nil {
			log.Fatalf("distributed mode: %v", err)
		}
		log.Println("Distributed mode enabled (novaque + MySQL)")
	}

	p.Start()
	defer p.Stop()

	http.HandleFunc("/events", p.HandleSSE)
	http.HandleFunc("/ws", p.HandleWebsocket)

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

		p.Publish(topic, "message", message)
		fmt.Fprintf(w, "Message sent to topic %s", topic)
	})

	go func() {
		for {
			time.Sleep(5 * time.Second)
			timeStr := time.Now().Format("2006-01-02 15:04:05")
			p.Publish("default", "time", timeStr)
		}
	}()

	log.Println("Server started at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
