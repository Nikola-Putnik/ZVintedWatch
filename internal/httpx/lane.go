package httpx

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"sync"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/activity"
)

// cooldownSteps is the exponential backoff applied to consecutive blocks.
var cooldownSteps = []time.Duration{
	2 * time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute, 60 * time.Minute,
}

// strikeResetAfter is how long a lane must stay unblocked before the backoff
// starts again from its first step.
const strikeResetAfter = 2 * time.Hour

// ErrLeaseReleased is returned when a released lease is used.
var ErrLeaseReleased = errors.New("httpx: lease already released")

// lane is one network identity (direct connection or proxy).
type lane struct {
	label     string
	transport Transport
	sem       chan struct{} // capacity 1: one request at a time

	mu             sync.Mutex
	nextAt         time.Time // earliest time the next request may start
	cooldownUntil  time.Time
	cooldownReason string
	strikes        int
	lastBlockAt    time.Time
}

func newLane(label string, t Transport) *lane {
	return &lane{label: label, transport: t, sem: make(chan struct{}, 1)}
}

func (l *lane) cooldownErr(now time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cooldownUntil.After(now) {
		return &CooldownError{Until: l.cooldownUntil, Reason: l.cooldownReason}
	}
	return nil
}

// Lease is the exclusive use of a lane, obtained with Fetcher.Acquire.
// It must be released with Release once the operation is complete.
type Lease struct {
	f        *Fetcher
	provider string
	lane     *lane
	settings Settings

	mu       sync.Mutex
	released bool
}

// Lane returns the label of the leased lane ("direct" or a redacted proxy URL).
func (l *Lease) Lane() string { return l.lane.label }

// Cookies returns the cookies the lane's session would send to rawURL.
func (l *Lease) Cookies(rawURL string) []Cookie { return l.lane.transport.Cookies(rawURL) }

// Release gives the lane back. It is safe to call more than once.
func (l *Lease) Release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return
	}
	l.released = true
	<-l.lane.sem
}

// Do performs a request on the leased lane, after waiting for the lane's
// pacing delay. HTTP 429 responses automatically put the lane in cooldown.
// The response is returned for every HTTP status; err is only set for
// transport failures, cancellation and cooldowns.
func (l *Lease) Do(ctx context.Context, req *Request) (*Response, error) {
	l.mu.Lock()
	released := l.released
	l.mu.Unlock()
	if released {
		return nil, ErrLeaseReleased
	}

	ln := l.lane
	if err := ln.cooldownErr(time.Now()); err != nil {
		return nil, err
	}
	ln.mu.Lock()
	wait := time.Until(ln.nextAt)
	ln.mu.Unlock()
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		}
	}

	start := time.Now()
	resp, err := ln.transport.Do(ctx, req)
	elapsed := time.Since(start)

	ln.mu.Lock()
	ln.nextAt = time.Now().Add(l.gap())
	ln.mu.Unlock()
	l.f.counter(l.provider).add(start)

	l.publishRequest(ctx, req, resp, err, elapsed)

	if err == nil && resp.Status == 429 {
		l.Block("rate limited (HTTP 429)", RetryAfter(resp.Header))
	}
	return resp, err
}

// gap returns the pacing delay: the minimum gap plus random jitter.
func (l *Lease) gap() time.Duration {
	d := time.Duration(l.settings.MinGapMs) * time.Millisecond
	if l.settings.JitterMs > 0 {
		d += time.Duration(rand.Int64N(int64(l.settings.JitterMs)+1)) * time.Millisecond
	}
	return d
}

// Block puts the lane in cooldown, typically after the site refused access.
// A positive retryAfter is used as is; otherwise the delay grows with every
// consecutive block (2, 5, 15, 30 then 60 minutes). It returns the delay.
func (l *Lease) Block(reason string, retryAfter time.Duration) time.Duration {
	ln := l.lane
	now := time.Now()

	ln.mu.Lock()
	if now.Sub(ln.lastBlockAt) > strikeResetAfter {
		ln.strikes = 0
	}
	ln.strikes++
	d := retryAfter
	if d <= 0 {
		step := ln.strikes - 1
		if step >= len(cooldownSteps) {
			step = len(cooldownSteps) - 1
		}
		d = cooldownSteps[step]
	}
	ln.cooldownUntil = now.Add(d)
	ln.cooldownReason = reason
	ln.lastBlockAt = now
	ln.mu.Unlock()

	if l.f.bus != nil {
		l.f.bus.Publish(activity.Event{
			Level:    activity.LevelWarn,
			Kind:     activity.KindHealth,
			Provider: l.provider,
			Message: fmt.Sprintf("%s on %s — pausing requests for %s (until %s)",
				reason, ln.label, d.Round(time.Second), now.Add(d).Format("15:04:05")),
		})
	}
	return d
}

func (l *Lease) publishRequest(ctx context.Context, req *Request, resp *Response, err error, elapsed time.Duration) {
	if l.f.bus == nil {
		return
	}
	info := &activity.HTTPInfo{
		Method:     req.Method,
		URL:        displayURL(req.URL),
		DurationMs: elapsed.Milliseconds(),
		Lane:       l.lane.label,
	}
	level := activity.LevelInfo
	msg := ""
	switch {
	case err != nil:
		level = activity.LevelError
		info.Error = err.Error()
		msg = fmt.Sprintf("%s %s failed: %v", req.Method, purpose(req), err)
	default:
		info.Status = resp.Status
		info.Bytes = resp.Bytes
		if resp.Status >= 400 {
			level = activity.LevelWarn
		}
		msg = fmt.Sprintf("%s %s → %d", req.Method, purpose(req), resp.Status)
	}
	e := activity.Event{Level: level, Kind: activity.KindRequest, Provider: l.provider, Message: msg, HTTP: info}
	activity.ScopeFrom(ctx).Apply(&e)
	l.f.bus.Publish(e)
}

func purpose(req *Request) string {
	if req.Purpose != "" {
		return req.Purpose
	}
	return "request"
}

// displayURL removes credentials from a URL before it is shown or logged.
func displayURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.User = nil
	return u.String()
}
