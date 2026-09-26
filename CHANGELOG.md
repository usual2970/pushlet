# Changelog

## v0.0.21

- WebSocket text commands are arity-checked: a malformed frame such as `SUB\n` (no topic argument) is logged and dropped instead of panicking the per-connection read goroutine — which killed the whole embedding process.

## v0.0.20

- `EnableDistributedMode` now accepts a [DistributedConnector](distributed_connector.go); use `EnableDistributedNovaque` for the previous novaque-only signature.
- Nil-client errors use `errDistributedNoConnector` instead of the removed `errDistributedNoClient` (`errors.Is` checks must be updated).
- Redis distributed mode returns via [RedisConnector](redis_connector.go) (unified relay envelope, not legacy per-topic Redis channels).

## v0.0.12 – v0.0.11 and earlier

- Distributed mode no longer uses the v0.0.11 `EnableDistributedMode(redisAddr, password, db int)` API.
- `Publish` / `PublishToAll` return `error` when the active connector fails.
