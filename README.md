# Pushlet

Lightweight Go library for real-time push over **SSE** and **WebSocket**. In standalone mode, messages are routed in-process; in distributed mode, instances stay in sync via a pluggable [DistributedConnector](distributed_connector.go)—either **Redis pub/sub** or embedded [novaque](https://github.com/usual2970/novaque) on a shared SQL database (MySQL, PostgreSQL, or SQLite).

![Go Version](https://img.shields.io/badge/Go-1.26.5+-blue.svg)
![License](https://img.shields.io/badge/License-MIT-green.svg)

**Live demo:** <https://pushlet-sample.ikit.fun/> — a small instant messenger built on pushlet ([source](https://github.com/usual2970/pushlet-sample-im)). Register in two browsers and watch room chat, the online list, and direct messages flow over SSE, with pushlet running its novaque/SQLite distributed relay.

## Requirements

| Mode | Dependencies |
|------|----------------|
| Standalone | Go 1.26.5+ |
| Distributed (Redis) | Above + **Redis** reachable from every replica |
| Distributed (novaque) | Above + shared database via novaque: **MySQL ≥ 8.0.1** (InnoDB), **PostgreSQL ≥ 14**, or **SQLite ≥ 3.39** |

## Install

```bash
go get github.com/usual2970/pushlet@v0.0.21
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

## Going further

- **[Distributed mode](docs/distributed.md)** — Redis or novaque (MySQL / PostgreSQL / SQLite) relay, per-replica channels, async publish, guarded SSE, operational notes.
- **[Wire protocol](docs/protocol.md)** — SSE framing, WebSocket commands, the `Message` JSON shape.
- **Runnable example** — `example/` runs two distributed instances in one process (`cd example && go run .`; without `PUSHLET_MYSQL_DSN` it starts MySQL via testcontainers, Docker required). Subscribe on `:9091/events?topic=demo`, publish on `:9090/send?...`, watch the message cross instances.
- **[Changelog](CHANGELOG.md)** — breaking changes and fixes by version.

## API summary

| Method | Description |
|--------|-------------|
| `New(...Option)` | Create instance; `WithLogger` injects logging |
| `SetHeartbeatInterval` | SSE ping / WebSocket ping interval |
| `EnableDistributedMode(connector)` | Enable distributed mode with Redis or custom connector (before `Start`, once) |
| `EnableDistributedNovaque(client, opts)` | Enable distributed mode via novaque (before `Start`, once) |
| `NewRedisConnector(opts)` | Build a Redis [DistributedConnector](distributed_connector.go) |
| `Start()` | Start broker (**required before** accepting connections) |
| `Stop()` | Stop broker and distributed connector |
| `HandleSSE` / `HandleWebsocket` | HTTP handlers |
| `EnableAsyncPublish(opts)` | Non-blocking publish queue (optional, before `Start`) |
| `DroppedEvents()` | Async overflow / failed publish counter |
| `Publish(topic, event, data)` | Publish to a topic; returns `error` |
| `PublishJSON(topic, event, v)` | JSON-encoded publish |
| `PublishToAll(event, data)` | Broadcast to all subscribed topics; returns `error` |
| `ResolveRelayChannel(configured)` | K8s-friendly novaque channel name |
| `OpenMySQLNovaque(ctx, dsn, opts, pool)` | DSN → `*sql.DB` + `*novaque.Client` |
| `HandleSSEGuarded(w, r, guard)` | SSE with validity recheck and write deadlines |

Registration fails with HTTP 503 if the broker is not `Start`ed. When a client send buffer is full, the connection is dropped and unregistered to avoid sends on a closed channel.

## Tests

```bash
go test ./...

# Requires Docker
go test -tags=integration ./...
```

## License

[MIT License](./LICENSE)
