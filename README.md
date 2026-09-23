# Pushlet

Lightweight Go library for real-time push over **SSE** and **WebSocket**. In standalone mode, messages are routed in-process; in distributed mode, instances stay in sync via embedded [novaque](https://github.com/usual2970/novaque) and a shared SQL database (MySQL, PostgreSQL, or SQLite).

![Go Version](https://img.shields.io/badge/Go-1.26.5+-blue.svg)
![License](https://img.shields.io/badge/License-MIT-green.svg)

## Requirements

| Mode | Dependencies |
|------|----------------|
| Standalone | Go 1.26.5+ |
| Distributed | Above + shared database via novaque: **MySQL ≥ 8.0.1** (InnoDB), **PostgreSQL ≥ 14**, or **SQLite ≥ 3.39** |

## Install

```bash
go get github.com/usual2970/pushlet@v0.0.17
```

## Quick start (standalone)

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

- SSE: `GET /events?topic=<name>`
- WebSocket: `GET /ws?topic=<name>` (optional initial topic)
- Publish: `POST /send?topic=<name>&message=<text>`

## Distributed mode

1. Each instance opens the **same** database with a [novaque driver](https://github.com/usual2970/novaque) (`mysql`, `postgres`, or `sqlite`).
2. Call `novaque.Open(driver.New(db), opts.Novaque)` and pass the client to `EnableDistributedMode` **before** `Start()` (once per process).
3. Then `Start()` / `Stop()`.

Cross-instance delivery is **at-least-once**: clients should dedupe or handle idempotently. Relay publishes fan out only to **channels that exist at publish time**; new instances should finish `Start()` before taking traffic.

MySQL example:

```go
import (
	"database/sql"

	_ "github.com/go-sql-driver/mysql"

	"github.com/usual2970/novaque"
	"github.com/usual2970/novaque/driver/mysql"
	"github.com/usual2970/pushlet"
)

db, err := sql.Open("mysql", dsn)
if err != nil {
	log.Fatal(err)
}

opts := pushlet.DefaultDistributedOptions()
// opts.Channel = "pushlet-prod-1" // optional: set per replica; if empty, auto-generated per process

client, err := novaque.Open(mysql.New(db), opts.Novaque)
if err != nil {
	log.Fatal(err)
}

p := pushlet.New()
if err := p.EnableDistributedMode(client, opts); err != nil {
	log.Fatal(err)
}
p.Start()
defer p.Stop()
```

PostgreSQL: use `github.com/usual2970/novaque/driver/postgres` with a `pgx`/`database/sql` pool. SQLite: use `github.com/usual2970/novaque/driver/sqlite` (see novaque docs for WAL / busy-timeout DSN flags).

`DistributedOptions` fields:

| Field | Description |
|-------|-------------|
| `Channel` | Novaque channel; **must be unique per replica** in multi-instance setups. If empty, auto-generated as `pushlet-node-<host>-<random>` |
| `RelayTopic` | Novaque topic for relay traffic (default `pushlet-relay`) |
| `RelayPublishTTL` | TTL for each relay message |
| `Novaque` | Options passed to `novaque.Open` |

For production multi-replica deployments, set `Channel` explicitly for stable identity and easier debugging.

### Changes since v0.0.11 and earlier

- Redis backend removed; distributed mode uses novaque on MySQL, PostgreSQL, or SQLite.
- `EnableDistributedMode(redisAddr, password, db int)` replaced by `EnableDistributedMode(client *novaque.Client, opts DistributedOptions) error`.
- `Publish` / `PublishToAll` return `error` when the novaque store fails.

## Runnable example (two instances + Docker)

`example/` runs **two distributed instances in one process** (defaults **9090** / **9091**). Without `PUSHLET_MYSQL_DSN`, it starts MySQL 8 via testcontainers (Docker required).

```bash
cd example
go run .
```

Cross-instance push:

```bash
# Terminal 1: subscribe on instance B
curl -N 'http://localhost:9091/events?topic=demo'

# Terminal 2: publish on instance A
curl -X POST 'http://localhost:9090/send?topic=demo&message=hello'
```

Environment variables:

| Variable | Meaning |
|----------|---------|
| `PUSHLET_MYSQL_DSN` | Use existing MySQL; skip testcontainer |
| `PUSHLET_ADDR` | Instance A listen address (default `:9090`) |
| `PUSHLET_ADDR_B` | Instance B listen address (default `:9091`) |
| `PUSHLET_NOVAQUE_CHANNEL_A` / `_B` | Novaque channel for A/B (default `pushlet-a` / `pushlet-b`) |

## Protocol

### SSE

The server emits a standard Event Stream. Application messages look like:

```text
event: message
data: <payload>

```

On connect you receive `event: connected`. Heartbeats are SSE comment lines (`: heartbeat ...`).

### WebSocket

- Server pushes **binary** frames: `topic` + space + JSON `Message`.
- Dynamic subscribe/unsubscribe uses **binary text commands** (not JSON):
  - `SUB <topic>\n`
  - `UNSUB <topic>\n`
  - `PING\n`
- Success replies are binary `OK`.

### Message (JSON fields)

| Field | Description |
|-------|-------------|
| `topic` | Topic name |
| `event` | Event name (SSE `event` line) |
| `data` | Payload body |
| `timestamp` | Timestamp |

## API summary

| Method | Description |
|--------|-------------|
| `New(...Option)` | Create instance; `WithLogger` injects logging |
| `SetHeartbeatInterval` | SSE ping / WebSocket ping interval |
| `EnableDistributedMode(client, opts)` | Enable distributed mode (before `Start`, once) |
| `Start()` | Start broker (**required before** accepting connections) |
| `Stop()` | Stop broker and distributed connector |
| `HandleSSE` / `HandleWebsocket` | HTTP handlers |
| `Publish(topic, event, data)` | Publish to a topic; returns `error` |
| `PublishToAll(event, data)` | Broadcast to all subscribed topics; returns `error` |

Registration fails with HTTP 503 if the broker is not `Start`ed. When a client send buffer is full, the connection is dropped and unregistered to avoid sends on a closed channel.

## Layout

```text
pushlet/
├── pushlet.go              # HTTP entrypoints and Publish API
├── broker.go               # Topic routing and distributed relay
├── client.go               # Per-connection outbound channel and backpressure
├── novaque_connector.go    # Novaque relay implementation
├── distributed_connector.go
├── message.go
├── logger.go
├── example/main.go         # Two-instance testcontainers demo
└── internal/testmysql/     # MySQL container for integration tests (tag: integration)
```

## Tests

```bash
go test ./...

# Requires Docker
go test -tags=integration ./...
```

## Operations

- Before scaling out, ensure every instance has completed `EnableDistributedMode` + `Start()` before load balancing.
- Slow consumers are disconnected under backpressure; clients should reconnect.
- Restrict CORS and `CheckOrigin` in production (defaults are permissive for demos).
- Monitor database and novaque backlog; invalid relay envelopes are logged and dropped.

## License

[MIT License](./LICENSE)
