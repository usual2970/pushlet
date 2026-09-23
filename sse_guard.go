package pushlet

import (
	"context"
	"net/http"
	"sync"
	"time"
)

const defaultSSERecheckInterval = 2 * time.Second

const defaultSSEWriteTimeout = 2 * time.Second

// SSEGuard configures periodic validity checks and write deadlines for SSE.
type SSEGuard struct {
	// RecheckInterval is how often IsValid runs after the stream opens.
	RecheckInterval time.Duration
	// WriteTimeout bounds each Write/Flush on the response.
	WriteTimeout time.Duration
	// ExpiresAt is a unix timestamp; when now >= ExpiresAt the stream ends. Zero disables.
	ExpiresAt int64
	// IsValid returns false to revoke the stream (access denied, etc.).
	IsValid func(context.Context) bool
}

func (g SSEGuard) recheckInterval() time.Duration {
	if g.RecheckInterval > 0 {
		return g.RecheckInterval
	}
	return defaultSSERecheckInterval
}

func (g SSEGuard) writeTimeout() time.Duration {
	if g.WriteTimeout > 0 {
		return g.WriteTimeout
	}
	return defaultSSEWriteTimeout
}

// HandleSSEGuarded serves SSE like HandleSSE but revokes the stream when the
// guard fails, JWT expiry elapses, or the client is slow to read after revoke.
func (p *Pushlet) HandleSSEGuarded(w http.ResponseWriter, r *http.Request, guard SSEGuard) {
	if p == nil || guard.IsValid == nil {
		http.Error(w, "events unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	topic := r.URL.Query().Get("topic")
	if topic == "" || topic == "/" {
		topic = "default"
	}

	writeTimeout := guard.writeTimeout()
	rw := &writeDeadlinedResponseWriter{
		ResponseWriter: w,
		controller:     http.NewResponseController(w),
		writeTimeout:   writeTimeout,
	}
	if err := rw.controller.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		http.Error(w, "events unavailable", http.StatusServiceUnavailable)
		return
	}

	valid := func(ctx context.Context) bool {
		qctx, stop := context.WithTimeout(ctx, writeTimeout)
		defer stop()
		return guard.IsValid(qctx)
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	if !valid(ctx) || (guard.ExpiresAt > 0 && time.Now().Unix() >= guard.ExpiresAt) {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			rw.revoke()
			cancel()
		}()
		ticker := time.NewTicker(guard.recheckInterval())
		defer ticker.Stop()
		var expiry <-chan time.Time
		if guard.ExpiresAt > 0 {
			timer := time.NewTimer(time.Until(time.Unix(guard.ExpiresAt, 0)))
			defer timer.Stop()
			expiry = timer.C
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-expiry:
				return
			case <-ticker.C:
				if !valid(ctx) {
					return
				}
			}
		}
	}()

	p.serveSSE(rw, r.Clone(ctx), topic)
	cancel()
	<-done
}

type writeDeadlinedResponseWriter struct {
	http.ResponseWriter
	controller   *http.ResponseController
	writeTimeout time.Duration
	mu           sync.Mutex
	revoked      bool
}

func (w *writeDeadlinedResponseWriter) prepareWrite() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.revoked {
		return context.Canceled
	}
	return w.controller.SetWriteDeadline(time.Now().Add(w.writeTimeout))
}

func (w *writeDeadlinedResponseWriter) Write(b []byte) (int, error) {
	if err := w.prepareWrite(); err != nil {
		return 0, err
	}
	return w.ResponseWriter.Write(b)
}

func (w *writeDeadlinedResponseWriter) Flush() {
	if w.prepareWrite() == nil {
		_ = w.controller.Flush()
	}
}

func (w *writeDeadlinedResponseWriter) revoke() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.revoked = true
	_ = w.controller.SetWriteDeadline(time.Now())
}

func (w *writeDeadlinedResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
