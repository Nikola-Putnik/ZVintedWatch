// Package activity is the in-process event bus behind the live console of the
// web UI. Components publish events (HTTP requests, poll results, matches,
// notifications, system messages); the bus keeps the most recent ones in a
// ring buffer and fans them out to subscribers such as Server-Sent Events
// connections. Every event is also written to the application log.
package activity

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Kind classifies events so the UI can filter and colour them.
type Kind string

// Event kinds.
const (
	KindRequest Kind = "request" // an outgoing HTTP request to a marketplace
	KindPoll    Kind = "poll"    // the outcome of a search refresh
	KindMatch   Kind = "match"   // a listing matched an alert
	KindNotify  Kind = "notify"  // a Telegram notification was sent (or failed)
	KindSystem  Kind = "system"  // lifecycle, configuration and health messages
)

// Level is the severity of an event.
type Level string

// Event levels.
const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// HTTPInfo describes an outgoing HTTP request and its outcome.
type HTTPInfo struct {
	Method     string `json:"method"`
	URL        string `json:"url"`
	Status     int    `json:"status"`
	DurationMs int64  `json:"durationMs"`
	Bytes      int64  `json:"bytes"`
	// Lane identifies the connection used (direct or a redacted proxy).
	Lane  string `json:"lane,omitempty"`
	Error string `json:"error,omitempty"`
}

// Event is one line of the live console.
type Event struct {
	ID       int64     `json:"id"`
	Time     time.Time `json:"time"`
	Level    Level     `json:"level"`
	Kind     Kind      `json:"kind"`
	Provider string    `json:"provider,omitempty"`
	SearchID int64     `json:"searchId,omitempty"`
	Search   string    `json:"search,omitempty"`
	Message  string    `json:"message"`
	HTTP     *HTTPInfo `json:"http,omitempty"`
	// Details holds optional extra lines shown when the event is expanded,
	// for example the titles of the listings returned by a search.
	Details []string `json:"details,omitempty"`
}

// Bus stores recent events and broadcasts new ones. It is safe for
// concurrent use. The zero value is not usable; call NewBus.
type Bus struct {
	mu     sync.Mutex
	ring   []Event
	next   int // index where the next event is written
	full   bool
	lastID int64
	subs   map[chan Event]struct{}
	logger *slog.Logger
}

// NewBus creates a bus that keeps the last size events. A nil logger
// disables log output.
func NewBus(size int, logger *slog.Logger) *Bus {
	if size <= 0 {
		size = 1000
	}
	return &Bus{
		ring:   make([]Event, size),
		subs:   make(map[chan Event]struct{}),
		logger: logger,
	}
}

// Publish records an event, assigning its ID and time (when unset), and
// delivers it to subscribers. Slow subscribers miss events instead of
// blocking the publisher.
func (b *Bus) Publish(e Event) Event {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	if e.Level == "" {
		e.Level = LevelInfo
	}

	b.mu.Lock()
	b.lastID++
	e.ID = b.lastID
	b.ring[b.next] = e
	b.next = (b.next + 1) % len(b.ring)
	if b.next == 0 {
		b.full = true
	}
	for ch := range b.subs {
		select {
		case ch <- e:
		default: // subscriber is too slow; drop the event for it
		}
	}
	b.mu.Unlock()

	b.log(e)
	return e
}

// Recent returns up to limit events, oldest first. limit <= 0 returns all
// buffered events.
func (b *Bus) Recent(limit int) []Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	var out []Event
	if b.full {
		out = append(out, b.ring[b.next:]...)
	}
	out = append(out, b.ring[:b.next]...)
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// Subscribe returns a channel receiving every new event and a function that
// cancels the subscription. The channel is closed by cancel.
func (b *Bus) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer <= 0 {
		buffer = 256
	}
	ch := make(chan Event, buffer)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, ch)
			b.mu.Unlock()
			close(ch)
		})
	}
}

func (b *Bus) log(e Event) {
	if b.logger == nil {
		return
	}
	attrs := []any{"kind", string(e.Kind)}
	if e.Provider != "" {
		attrs = append(attrs, "provider", e.Provider)
	}
	if e.Search != "" {
		attrs = append(attrs, "search", e.Search)
	}
	if e.HTTP != nil {
		attrs = append(attrs, "status", e.HTTP.Status, "ms", e.HTTP.DurationMs)
	}
	level := slog.LevelInfo
	switch e.Level {
	case LevelDebug:
		level = slog.LevelDebug
	case LevelWarn:
		level = slog.LevelWarn
	case LevelError:
		level = slog.LevelError
	}
	b.logger.Log(context.Background(), level, e.Message, attrs...)
}

// Scope identifies the provider and search an operation belongs to. It is
// carried through contexts so low-level components (such as the HTTP layer)
// can tag their events without knowing about searches.
type Scope struct {
	Provider string
	SearchID int64
	Search   string
}

type scopeKey struct{}

// WithScope returns a context carrying the given scope.
func WithScope(ctx context.Context, s Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, s)
}

// ScopeFrom extracts the scope stored in ctx (zero value when absent).
func ScopeFrom(ctx context.Context) Scope {
	s, _ := ctx.Value(scopeKey{}).(Scope)
	return s
}

// Apply copies the scope fields into an event when they are not already set.
func (s Scope) Apply(e *Event) {
	if e.Provider == "" {
		e.Provider = s.Provider
	}
	if e.SearchID == 0 {
		e.SearchID = s.SearchID
	}
	if e.Search == "" {
		e.Search = s.Search
	}
}
