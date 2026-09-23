package pushlet

import (
	"time"
)

const defaultAsyncQueueSize = 512

const defaultAsyncStopDrain = 2 * time.Second

// AsyncPublishOptions configures the optional background publisher.
type AsyncPublishOptions struct {
	// QueueSize bounds the outbound queue. When full, events are dropped.
	QueueSize int
	// StopDrain is how long Stop waits for the worker to exit after quit.
	StopDrain time.Duration
}

// DefaultAsyncPublishOptions returns library defaults aligned with production embedders.
func DefaultAsyncPublishOptions() AsyncPublishOptions {
	return AsyncPublishOptions{
		QueueSize: defaultAsyncQueueSize,
		StopDrain: defaultAsyncStopDrain,
	}
}

type asyncOutbound struct {
	topic string
	event string
	data  string
	all   bool
}

// EnableAsyncPublish starts a single worker that is the only caller of the
// broker's Publish methods. Call before Start. Publish and PublishToAll become
// non-blocking enqueues when enabled; a full queue drops events (see DroppedEvents).
func (p *Pushlet) EnableAsyncPublish(opts AsyncPublishOptions) {
	if p == nil {
		return
	}
	if opts.QueueSize <= 0 {
		opts.QueueSize = defaultAsyncQueueSize
	}
	if opts.StopDrain <= 0 {
		opts.StopDrain = defaultAsyncStopDrain
	}
	p.asyncOpts = opts
	size := opts.QueueSize
	p.asyncOut = make(chan asyncOutbound, size)
	p.asyncQuit = make(chan struct{})
	p.asyncDone = make(chan struct{})
	go p.runAsyncPublisher()
}

func (p *Pushlet) runAsyncPublisher() {
	defer close(p.asyncDone)
	for {
		select {
		case ev := <-p.asyncOut:
			var err error
			if ev.all {
				err = p.broker.PublishToAll(NewMessage("global", ev.event, ev.data))
			} else {
				err = p.broker.Publish(ev.topic, NewMessage(ev.topic, ev.event, ev.data))
			}
			if err != nil {
				p.asyncDropped.Add(1)
			}
		case <-p.asyncQuit:
			return
		}
	}
}

func (p *Pushlet) asyncEnabled() bool {
	return p != nil && p.asyncOut != nil
}

func (p *Pushlet) enqueuePublish(topic, event, data string, all bool) error {
	if !p.asyncEnabled() {
		if all {
			return p.broker.PublishToAll(NewMessage("global", event, data))
		}
		return p.broker.Publish(topic, NewMessage(topic, event, data))
	}
	select {
	case p.asyncOut <- asyncOutbound{topic: topic, event: event, data: data, all: all}:
		return nil
	default:
		p.asyncDropped.Add(1)
		return nil
	}
}

// DroppedEvents reports async queue overflow and failed broker publishes.
func (p *Pushlet) DroppedEvents() uint64 {
	if p == nil {
		return 0
	}
	return p.asyncDropped.Load()
}

func (p *Pushlet) stopAsyncPublisher() {
	if p == nil {
		return
	}
	p.asyncStopOnce.Do(func() {
		if p.asyncQuit == nil {
			return
		}
		close(p.asyncQuit)
		drain := p.asyncOpts.StopDrain
		if drain <= 0 {
			drain = defaultAsyncStopDrain
		}
		select {
		case <-p.asyncDone:
		case <-time.After(drain):
		}
	})
}
