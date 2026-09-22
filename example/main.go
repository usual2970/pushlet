package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/usual2970/pushlet"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, cleanupDB, err := openMySQL(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanupDB()

	opts := pushlet.DefaultDistributedOptions()
	opts.Novaque.PollInterval = 100 * time.Millisecond

	addrA := envOr("PUSHLET_ADDR", ":9090")
	addrB := envOr("PUSHLET_ADDR_B", ":9091")

	runInstance(ctx, db, opts, "A", addrA)
	runInstance(ctx, db, opts, "B", addrB)

	log.Println("Distributed Pushlet (novaque + MySQL) — two instances, one process")
	log.Printf("  Instance A  http://localhost%s  SSE /events?topic=demo", addrA)
	log.Printf("  Instance B  http://localhost%s  SSE /events?topic=demo", addrB)
	log.Printf("  Try: curl -X POST 'http://localhost%s/send?topic=demo&message=hi'", addrA)
	log.Println("  Open B's SSE URL in a browser or curl -N, then POST to A.")

	<-ctx.Done()
	log.Println("shutting down")
}

func runInstance(ctx context.Context, db *sql.DB, opts pushlet.DistributedOptions, name, addr string) {
	p := pushlet.New()
	p.SetHeartbeatInterval(30 * time.Second)
	if err := p.EnableDistributedMode(db, opts); err != nil {
		log.Fatalf("instance %s: distributed mode: %v", name, err)
	}
	p.Start()

	mux := http.NewServeMux()
	mux.HandleFunc("/events", p.HandleSSE)
	mux.HandleFunc("/ws", p.HandleWebsocket)
	mux.HandleFunc("/send", handleSend(p, name))

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		p.Stop()
	}()
	go func() {
		log.Printf("instance %s listening on %s", name, addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("instance %s: %v", name, err)
		}
	}()

	go tickPublish(p, name)
}

func handleSend(p *pushlet.Pushlet, instance string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		topic := r.URL.Query().Get("topic")
		if topic == "" {
			topic = "default"
		}
		message := r.URL.Query().Get("message")
		if message == "" {
			http.Error(w, "message required", http.StatusBadRequest)
			return
		}
		p.Publish(topic, "message", message)
		fmt.Fprintf(w, "ok instance=%s topic=%s\n", instance, topic)
	}
}

func tickPublish(p *pushlet.Pushlet, instance string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		p.Publish("default", "time", fmt.Sprintf("%s %s", instance, time.Now().Format(time.RFC3339)))
	}
}

func openMySQL(ctx context.Context) (*sql.DB, func(), error) {
	if dsn := os.Getenv("PUSHLET_MYSQL_DSN"); dsn != "" {
		log.Println("Using PUSHLET_MYSQL_DSN (no testcontainer)")
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			return nil, func() {}, err
		}
		db.SetMaxOpenConns(16)
		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			return nil, func() {}, fmt.Errorf("mysql ping: %w", err)
		}
		return db, func() { _ = db.Close() }, nil
	}

	log.Println("Starting MySQL 8 testcontainer (requires Docker)...")
	containerCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	req := testcontainers.ContainerRequest{
		Image:        "mysql:8.0.36",
		Env:          map[string]string{"MYSQL_ROOT_PASSWORD": "root", "MYSQL_DATABASE": "pushlet"},
		ExposedPorts: []string{"3306/tcp"},
		WaitingFor: wait.ForLog("port: 3306  MySQL Community Server").
			WithStartupTimeout(2 * time.Minute),
	}
	c, err := testcontainers.GenericContainer(containerCtx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, func() {}, fmt.Errorf("mysql container: %w", err)
	}

	host, err := c.Host(containerCtx)
	if err != nil {
		_ = c.Terminate(context.Background())
		return nil, func() {}, err
	}
	port, err := c.MappedPort(containerCtx, "3306")
	if err != nil {
		_ = c.Terminate(context.Background())
		return nil, func() {}, err
	}

	dsn := fmt.Sprintf("root:root@tcp(%s:%s)/pushlet?parseTime=true&loc=UTC&multiStatements=true", host, port.Port())
	var db *sql.DB
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		db, err = sql.Open("mysql", dsn)
		if err == nil {
			if pingErr := db.PingContext(containerCtx); pingErr == nil {
				break
			}
			_ = db.Close()
			db = nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	if db == nil {
		_ = c.Terminate(context.Background())
		return nil, func() {}, fmt.Errorf("mysql not ready")
	}
	db.SetMaxOpenConns(16)
	log.Printf("MySQL ready at %s:%s", host, port.Port())

	cleanup := func() {
		_ = db.Close()
		_ = c.Terminate(context.Background())
	}
	return db, cleanup, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
