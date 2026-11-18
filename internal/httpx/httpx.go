// Package httpx is the outgoing HTTP layer used by marketplace providers.
//
// Its job is to keep ZVintedWatch polite and hard to block:
//
//   - Requests go through "lanes". A lane is one network identity: either the
//     direct connection or one configured proxy. Each lane performs at most one
//     request at a time and waits a minimum gap (plus random jitter) between
//     two requests, so polling never bursts.
//   - When a site answers with a rate limit (HTTP 429) or a provider reports a
//     block (typically HTTP 403), the lane enters an exponential cooldown and
//     stops sending requests until it expires.
//   - The production transport impersonates the TLS and HTTP/2 fingerprint of a
//     desktop Chrome browser (github.com/bogdanfinn/tls-client), and each lane
//     keeps its own cookie jar, i.e. its own browser-like session.
//   - Every request is published on the activity bus for the live console.
//
// Providers obtain a Lease for a lane with Fetcher.Acquire, perform one or more
// requests with Lease.Do and give the lane back with Lease.Release.
package httpx

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/activity"
)

// Header is one request header. Headers are an ordered list because browsers
// send them in a stable order, which some anti-bot systems check.
type Header struct {
	Name  string
	Value string
}

// Request describes an outgoing HTTP request.
type Request struct {
	Method  string
	URL     string
	Headers []Header
	// Purpose is a short label shown in the live console ("search", "item"...).
	Purpose string
	// MaxBodyBytes limits how much of the body is read (default 8 MiB).
	MaxBodyBytes int64
	// Consume, when set, receives the (decompressed) body as a stream instead
	// of it being buffered into Response.Body. It may stop reading early.
	Consume func(r io.Reader) error
}

// Cookie is a name/value pair taken from a lane's cookie jar.
type Cookie struct {
	Name  string
	Value string
}

// Response is the outcome of a request.
type Response struct {
	Status   int
	Header   http.Header
	Body     []byte
	Bytes    int64
	Duration time.Duration
}

// Transport performs single HTTP requests for one lane.
type Transport interface {
	Do(ctx context.Context, req *Request) (*Response, error)
	// Cookies returns the cookies the lane's jar would send to rawURL.
	Cookies(rawURL string) []Cookie
	Close()
}

// TransportFactory builds a Transport using the given proxy URL (empty for a
// direct connection).
type TransportFactory func(proxy string, timeout time.Duration) (Transport, error)

// Settings tune the request pacing of one provider. They are editable in the
// web UI (Settings → Network).
type Settings struct {
	// MinGapMs is the minimum delay between two requests on the same lane.
	MinGapMs int `json:"minGapMs"`
	// JitterMs is a random extra delay added to MinGapMs.
	JitterMs int `json:"jitterMs"`
	// TimeoutSec is the per-request timeout.
	TimeoutSec int `json:"timeoutSec"`
	// Proxies lists proxy URLs (http://, https://, socks5://). When empty,
	// the direct connection is used. With several proxies, requests are spread
	// across them and each one is paced independently.
	Proxies []string `json:"proxies"`
}

// DefaultSettings returns conservative defaults suitable for most sites.
func DefaultSettings() Settings {
	return Settings{MinGapMs: 3000, JitterMs: 1500, TimeoutSec: 30, Proxies: []string{}}
}

// Normalized fills unset values with defaults and drops blank proxies.
func (s Settings) Normalized() Settings {
	d := DefaultSettings()
	if s.MinGapMs <= 0 {
		s.MinGapMs = d.MinGapMs
	}
	if s.JitterMs < 0 {
		s.JitterMs = 0
	}
	if s.TimeoutSec <= 0 {
		s.TimeoutSec = d.TimeoutSec
	}
	proxies := []string{}
	for _, p := range s.Proxies {
		if p = strings.TrimSpace(p); p != "" {
			proxies = append(proxies, p)
		}
	}
	s.Proxies = proxies
	return s
}

// ValidateProxy checks that a proxy URL is well formed and uses a supported scheme.
func ValidateProxy(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid proxy URL: %w", err)
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return fmt.Errorf("unsupported proxy scheme %q (use http, https or socks5)", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("proxy URL %q has no host", raw)
	}
	return nil
}

// CooldownError is returned when every lane of a provider is cooling down.
type CooldownError struct {
	Until  time.Time
	Reason string
}

func (e *CooldownError) Error() string {
	return fmt.Sprintf("cooling down until %s (%s)", e.Until.Format("15:04:05"), e.Reason)
}

// Fetcher manages the lanes of every provider. It is safe for concurrent use.
type Fetcher struct {
	bus     *activity.Bus
	factory TransportFactory

	mu    sync.Mutex
	pools map[string]*pool
	stats map[string]*minuteCounter
}

type pool struct {
	settings Settings
	lanes    []*lane
}

// New creates a Fetcher. A nil factory uses the Chrome-impersonating transport.
func New(bus *activity.Bus, factory TransportFactory) *Fetcher {
	if factory == nil {
		factory = NewBrowserTransport
	}
	return &Fetcher{
		bus:     bus,
		factory: factory,
		pools:   make(map[string]*pool),
		stats:   make(map[string]*minuteCounter),
	}
}

// Configure (re)creates the lanes of a provider. Cooldown state is kept for
// lanes whose proxy did not change.
func (f *Fetcher) Configure(provider string, s Settings) error {
	s = s.Normalized()
	for _, p := range s.Proxies {
		if err := ValidateProxy(p); err != nil {
			return err
		}
	}
	proxies := s.Proxies
	if len(proxies) == 0 {
		proxies = []string{""}
	}

	f.mu.Lock()
	old := f.pools[provider]
	f.mu.Unlock()

	lanes := make([]*lane, 0, len(proxies))
	for _, p := range proxies {
		label := laneLabel(p)
		if old != nil {
			if l := old.find(label); l != nil {
				lanes = append(lanes, l)
				continue
			}
		}
		t, err := f.factory(p, time.Duration(s.TimeoutSec)*time.Second)
		if err != nil {
			return fmt.Errorf("lane %s: %w", label, err)
		}
		lanes = append(lanes, newLane(label, t))
	}

	f.mu.Lock()
	f.pools[provider] = &pool{settings: s, lanes: lanes}
	f.mu.Unlock()

	// Close transports that are no longer used, leaving in-flight requests time to finish.
	if old != nil {
		for _, l := range old.lanes {
			if !containsLane(lanes, l) {
				t := l.transport
				time.AfterFunc(2*time.Minute, t.Close)
			}
		}
	}
	return nil
}

// Settings returns the effective settings of a provider.
func (f *Fetcher) Settings(provider string) Settings {
	p, err := f.pool(provider)
	if err != nil {
		return DefaultSettings()
	}
	return p.settings
}

func (f *Fetcher) pool(provider string) (*pool, error) {
	f.mu.Lock()
	p := f.pools[provider]
	f.mu.Unlock()
	if p != nil {
		return p, nil
	}
	if err := f.Configure(provider, DefaultSettings()); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pools[provider], nil
}

func (p *pool) find(label string) *lane {
	for _, l := range p.lanes {
		if l.label == label {
			return l
		}
	}
	return nil
}

func containsLane(lanes []*lane, l *lane) bool {
	for _, x := range lanes {
		if x == l {
			return true
		}
	}
	return false
}

// Acquire reserves a lane of the provider for exclusive use. It prefers idle
// lanes that became available the earliest and blocks until the chosen lane
// is free. When every lane is cooling down, it returns a *CooldownError.
func (f *Fetcher) Acquire(ctx context.Context, provider string) (*Lease, error) {
	p, err := f.pool(provider)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var best *lane
	var bestScore time.Time
	cool := &CooldownError{}
	for _, l := range p.lanes {
		l.mu.Lock()
		until, reason, next := l.cooldownUntil, l.cooldownReason, l.nextAt
		l.mu.Unlock()
		if until.After(now) {
			if cool.Until.IsZero() || until.Before(cool.Until) {
				cool.Until, cool.Reason = until, reason
			}
			continue
		}
		score := next
		if len(l.sem) > 0 { // busy: only use it if nothing else is available
			score = score.Add(time.Hour)
		}
		if best == nil || score.Before(bestScore) {
			best, bestScore = l, score
		}
	}
	if best == nil {
		return nil, cool
	}

	select {
	case best.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := best.cooldownErr(time.Now()); err != nil {
		<-best.sem
		return nil, err
	}
	return &Lease{f: f, provider: provider, lane: best, settings: p.settings}, nil
}

// Health summarizes the state of a provider's lanes for the dashboard.
type Health struct {
	// State is "ok", "degraded" (some lanes cooling down) or "cooldown".
	State            string       `json:"state"`
	CooldownUntil    time.Time    `json:"cooldownUntil"`
	Reason           string       `json:"reason"`
	RequestsLastHour int          `json:"requestsLastHour"`
	Lanes            []LaneHealth `json:"lanes"`
}

// LaneHealth is the state of one lane.
type LaneHealth struct {
	Label         string    `json:"label"`
	CoolingDown   bool      `json:"coolingDown"`
	CooldownUntil time.Time `json:"cooldownUntil"`
	Reason        string    `json:"reason"`
}

// Health returns the current health of a provider.
func (f *Fetcher) Health(provider string) Health {
	h := Health{State: "ok", Lanes: []LaneHealth{}}
	p, err := f.pool(provider)
	if err != nil {
		h.State, h.Reason = "error", err.Error()
		return h
	}
	now := time.Now()
	cooling := 0
	for _, l := range p.lanes {
		l.mu.Lock()
		lh := LaneHealth{Label: l.label, CooldownUntil: l.cooldownUntil, Reason: l.cooldownReason}
		l.mu.Unlock()
		lh.CoolingDown = lh.CooldownUntil.After(now)
		if lh.CoolingDown {
			cooling++
			if h.CooldownUntil.IsZero() || lh.CooldownUntil.Before(h.CooldownUntil) {
				h.CooldownUntil, h.Reason = lh.CooldownUntil, lh.Reason
			}
		} else {
			lh.CooldownUntil, lh.Reason = time.Time{}, ""
		}
		h.Lanes = append(h.Lanes, lh)
	}
	switch {
	case cooling == len(p.lanes) && cooling > 0:
		h.State = "cooldown"
	case cooling > 0:
		h.State = "degraded"
	default:
		h.CooldownUntil, h.Reason = time.Time{}, ""
	}
	h.RequestsLastHour = f.counter(provider).total(now)
	return h
}

// ResetCooldown clears the cooldown of every lane of a provider (used by the
// "retry now" button of the UI).
func (f *Fetcher) ResetCooldown(provider string) {
	p, err := f.pool(provider)
	if err != nil {
		return
	}
	for _, l := range p.lanes {
		l.mu.Lock()
		l.cooldownUntil, l.cooldownReason = time.Time{}, ""
		l.mu.Unlock()
	}
}

func (f *Fetcher) counter(provider string) *minuteCounter {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.stats[provider]
	if c == nil {
		c = &minuteCounter{}
		f.stats[provider] = c
	}
	return c
}

// Close releases every transport.
func (f *Fetcher) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.pools {
		for _, l := range p.lanes {
			l.transport.Close()
		}
	}
	f.pools = map[string]*pool{}
}

func laneLabel(proxy string) string {
	if proxy == "" {
		return "direct"
	}
	u, err := url.Parse(proxy)
	if err != nil || u.Host == "" {
		return "proxy"
	}
	return u.Scheme + "://" + u.Host // credentials are never displayed
}

// RetryAfter parses a Retry-After header expressed in seconds or as an HTTP date.
func RetryAfter(h http.Header) time.Duration {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// minuteCounter counts events over the last 60 minutes using one bucket per minute.
type minuteCounter struct {
	mu      sync.Mutex
	buckets [60]int
	stamps  [60]int64
}

func (c *minuteCounter) add(t time.Time) {
	m := t.Unix() / 60
	i := m % 60
	c.mu.Lock()
	if c.stamps[i] != m {
		c.stamps[i], c.buckets[i] = m, 0
	}
	c.buckets[i]++
	c.mu.Unlock()
}

func (c *minuteCounter) total(t time.Time) int {
	m := t.Unix() / 60
	c.mu.Lock()
	defer c.mu.Unlock()
	sum := 0
	for i := range c.buckets {
		if m-c.stamps[i] < 60 {
			sum += c.buckets[i]
		}
	}
	return sum
}
