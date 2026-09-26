# Wire protocol

## SSE

The server emits a standard Event Stream. Application messages look like:

```text
event: message
data: <payload>

```

On connect you receive `event: connected`. Heartbeats are SSE comment lines (`: heartbeat ...`).

## WebSocket

- Server pushes **binary** frames: `topic` + space + JSON `Message`.
- Dynamic subscribe/unsubscribe uses **binary text commands** (not JSON):
  - `SUB <topic>\n`
  - `UNSUB <topic>\n`
  - `PING\n`
- Success replies are binary `OK`. Malformed commands (for example `SUB` with no topic) are logged and dropped — they never panic the connection.

## Message (JSON fields)

| Field | Description |
|-------|-------------|
| `topic` | Topic name |
| `event` | Event name (SSE `event` line) |
| `data` | Payload body |
| `timestamp` | Timestamp |

## Delivery semantics

- Delivery is **best-effort per connection**: only clients subscribed at publish time receive a message; there is no replay. Applications that need gap-free history should persist messages themselves and backfill by id on reconnect (the [sample app](https://github.com/usual2970/pushlet-sample-im) shows the pattern).
- A slow consumer whose send buffer fills is disconnected and unregistered; clients should reconnect and re-subscribe.
