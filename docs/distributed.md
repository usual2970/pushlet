# Distributed mode

Pick **one** backend per process. Both use the same JSON **relay envelope** on a single relay channel/topic (default `pushlet-relay`).

1. Build a [DistributedConnector](../distributed_connector.go) (`RedisConnector` or `NovaqueConnector`).
2. Call `EnableDistributedMode(connector)` **before** `Start()` (once per process).
3. Then `Start()` / `Stop()`.

## Redis (pub/sub)

Ephemeral **fire-and-forget** relay—fast, no SQL, but messages are not durably queued for offline replicas.

```go
opts := pushlet.DefaultRedisOptions()
opts.Addr = "127.0.0.1:6379"

conn, err := pushlet.NewRedisConnector(opts)
if err != nil {
	log.Fatal(err)
}

p := pushlet.New()
if err := p.EnableDistributedMode(conn); err != nil {
	log.Fatal(err)
}
p.Start()
defer p.Stop()
```

## Novaque (SQL)

Cross-instance delivery is **at-least-once**: clients should dedupe or handle idempotently. Relay publishes fan out only to **channels that exist at publish time**; new instances should finish `Start()` before taking traffic.

1. Each instance opens the **same** database with a [novaque driver](https://github.com/usual2970/novaque) (`mysql`, `postgres`, or `sqlite`).
2. Call `novaque.Open(driver.New(db), opts.Novaque)` and `EnableDistributedNovaque(client, opts)`.

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
if err := p.EnableDistributedNovaque(client, opts); err != nil {
	log.Fatal(err)
}
p.Start()
defer p.Stop()
```

PostgreSQL: use `github.com/usual2970/novaque/driver/postgres` with a `pgx`/`database/sql` pool. SQLite: use `github.com/usual2970/novaque/driver/sqlite` (see novaque docs for WAL / busy-timeout DSN flags — `foreign_keys(1)`, `journal_mode(WAL)`, and `busy_timeout(10000)` are the expected pragmas).

### `DistributedOptions` fields

| Field | Description |
|-------|-------------|
| `Channel` | Novaque channel; **must be unique per replica** in multi-instance setups. If empty, auto-generated as `pushlet-node-<host>-<random>` |
| `RelayTopic` | Novaque topic for relay traffic (default `pushlet-relay`) |
| `RelayPublishTTL` | TTL for each relay message |
| `Novaque` | Options passed to `novaque.Open` |

For production multi-replica deployments, set `Channel` explicitly for stable identity and easier debugging. In Kubernetes you can derive a stable name with:

```go
opts.Channel = pushlet.ResolveRelayChannel("") // pushlet-<POD_NAME> or pushlet-<hostname>
```

## Async publish (embedders)

Distributed `Publish` performs synchronous novaque I/O on the calling goroutine. For HTTP handlers and business logic that must not wait on relay latency, enable a background publisher **before** `Start()`:

```go
p.EnableAsyncPublish(pushlet.DefaultAsyncPublishOptions())
```

Events enqueue best-effort; a full queue drops overflow (see `DroppedEvents()`). `Stop()` stops the worker first, then the broker.

## Guarded SSE (access revocation)

`HandleSSEGuarded` wraps `HandleSSE` with periodic `IsValid` checks, optional unix expiry, and write deadlines so slow readers cannot block revocation:

```go
pushlet.HandleSSEGuarded(w, r, pushlet.SSEGuard{
    IsValid: func(ctx context.Context) bool { /* access still active */ },
    ExpiresAt: jwtExpUnix,
})
```

## MySQL open helper

`OpenMySQLNovaque` opens a pooled `*sql.DB`, pings, and returns an opened `*novaque.Client` for `EnableDistributedNovaque`. The embedder closes the DB after `Stop()`.

## Example environment variables

The runnable `example/` (two instances in one process, defaults `:9090`/`:9091`):

| Variable | Meaning |
|----------|---------|
| `PUSHLET_MYSQL_DSN` | Use existing MySQL; skip testcontainer |
| `PUSHLET_ADDR` | Instance A listen address (default `:9090`) |
| `PUSHLET_ADDR_B` | Instance B listen address (default `:9091`) |
| `PUSHLET_NOVAQUE_CHANNEL_A` / `_B` | Novaque channel for A/B (default `pushlet-a` / `pushlet-b`) |

## Operations

- Before scaling out, ensure every instance has completed `EnableDistributedMode` + `Start()` before load balancing.
- Slow consumers are disconnected under backpressure; clients should reconnect.
- Restrict CORS and `CheckOrigin` in production (defaults are permissive for demos).
- Monitor database and novaque backlog; invalid relay envelopes are logged and dropped.
