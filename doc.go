// Package pushlet is a lightweight Go library for real-time pub/sub over
// Server-Sent Events (SSE) and WebSocket.
//
// Construct a server with [New], call [Pushlet.Start], wire HTTP handlers via
// [Pushlet.HandleSSE] and [Pushlet.HandleWebsocket], and publish events with
// [Pushlet.Publish] or [Pushlet.PublishToAll].
//
// # Single-instance mode
//
// By default, messages are routed in-process through an internal [Broker] that
// maps topics to connected [Client] values.
//
// # Distributed mode
//
// Call [Pushlet.EnableDistributedMode] before [Pushlet.Start] to fan out
// publishes across processes using embedded [novaque] and a shared MySQL
// database. Delivery is at-least-once; subscribers should deduplicate if
// needed. Configure channels and relay options with [DistributedOptions].
//
// # Logging
//
// Inject a custom [Logger] with [WithLogger] when constructing [Pushlet].
//
// For usage examples, see the repository README and the example/ directory at
// https://github.com/usual2970/pushlet.
package pushlet
