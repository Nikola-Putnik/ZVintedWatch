package httpx

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/activity"
)

// fakeTransport answers every request with a fixed status and records calls.
type fakeTransport struct {
	mu     sync.Mutex
	status int
	header http.Header
	calls  []time.Time
}

func (f *fakeTransport) Do(ctx context.Context, req *Request) (*Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, time.Now())
	h := f.header
	if h == nil {
		h = http.Header{}
	}
	return &Response{Status: f.status, Header: h, Body: []byte("ok"), Bytes: 2}, nil
}
func (f *fakeTransport) Cookies(string) []Cookie { return nil }
func (f *fakeTransport) Close()                  {}

func newTestFetcher(t *testing.T, status int) (*Fetcher, map[string]*fakeTransport) {
	t.Helper()
	transports := map[string]*fakeTransport{}
	var mu sync.Mutex
	f := New(activity.NewBus(100, nil), func(proxy string, _ time.Duration) (Transport, error) {
		mu.Lock()
		defer mu.Unlock()
		ft := &fakeTransport{status: status}
		transports[laneLabel(proxy)] = ft
		return ft, nil
	})
	return f, transports
}

func TestLanePacing(t *testing.T) {
	f, tr := newTestFetcher(t, 200)
	if err := f.Configure("site", Settings{MinGapMs: 80, JitterMs: 0}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		lease, err := f.Acquire(ctx, "site")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := lease.Do(ctx, &Request{Method: "GET", URL: "https://example.test/"}); err != nil {
			t.Fatal(err)
		}
		lease.Release()
	}
	calls := tr["direct"].calls
	if len(calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(calls))
	}
	for i := 1; i < len(calls); i++ {
		if gap := calls[i].Sub(calls[i-1]); gap < 75*time.Millisecond {
			t.Fatalf("requests %d and %d only %v apart", i-1, i, gap)
		}
	}
	if h := f.Health("site"); h.RequestsLastHour != 3 || h.State != "ok" {
		t.Fatalf("unexpected health %+v", h)
	}
}

func TestRateLimitTriggersCooldown(t *testing.T) {
	f, tr := newTestFetcher(t, 429)
	if err := f.Configure("site", Settings{MinGapMs: 1}); err != nil {
		t.Fatal(err)
	}
	tr["direct"].header = http.Header{"Retry-After": {"120"}}

	ctx := context.Background()
	lease, err := f.Acquire(ctx, "site")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := lease.Do(ctx, &Request{Method: "GET", URL: "https://example.test/"})
	if err != nil || resp.Status != 429 {
		t.Fatalf("unexpected result %v %v", resp, err)
	}
	lease.Release()

	_, err = f.Acquire(ctx, "site")
	var cool *CooldownError
	if !errors.As(err, &cool) {
		t.Fatalf("expected CooldownError, got %v", err)
	}
	if d := time.Until(cool.Until); d < 110*time.Second || d > 125*time.Second {
		t.Fatalf("Retry-After not honoured, cooldown ends in %v", d)
	}
	if h := f.Health("site"); h.State != "cooldown" {
		t.Fatalf("expected cooldown state, got %+v", h)
	}

	// Reconfiguring with the same lanes keeps the cooldown...
	if err := f.Configure("site", Settings{MinGapMs: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Acquire(ctx, "site"); !errors.As(err, &cool) {
		t.Fatalf("cooldown lost after reconfigure: %v", err)
	}
	// ...until it is reset manually.
	f.ResetCooldown("site")
	lease, err = f.Acquire(ctx, "site")
	if err != nil {
		t.Fatalf("expected lane after reset, got %v", err)
	}
	lease.Release()
}

func TestBlockBackoffEscalates(t *testing.T) {
	f, _ := newTestFetcher(t, 200)
	ctx := context.Background()
	var got []time.Duration
	for i := 0; i < 7; i++ {
		f.ResetCooldown("site")
		lease, err := f.Acquire(ctx, "site")
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, lease.Block("blocked (HTTP 403)", 0))
		lease.Release()
	}
	want := []time.Duration{2 * time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute,
		60 * time.Minute, 60 * time.Minute, 60 * time.Minute}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("step %d: got %v want %v", i, got[i], want[i])
		}
	}
}

func TestProxiesSpreadConcurrentLeases(t *testing.T) {
	f, _ := newTestFetcher(t, 200)
	err := f.Configure("site", Settings{Proxies: []string{"http://user:secret@p1.example:8080", "socks5://p2.example:1080"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	a, err := f.Acquire(ctx, "site")
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.Acquire(ctx, "site")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	defer b.Release()
	if a.Lane() == b.Lane() {
		t.Fatalf("both leases use lane %s", a.Lane())
	}
	for _, l := range []string{a.Lane(), b.Lane()} {
		if l != "http://p1.example:8080" && l != "socks5://p2.example:1080" {
			t.Fatalf("unexpected lane label %q (credentials must be hidden)", l)
		}
	}
}

func TestAcquireRespectsContext(t *testing.T) {
	f, _ := newTestFetcher(t, 200)
	held, err := f.Acquire(context.Background(), "site")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := f.Acquire(ctx, "site"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestValidateProxy(t *testing.T) {
	for _, ok := range []string{"http://h:1", "https://u:p@h:2", "socks5://h:3"} {
		if err := ValidateProxy(ok); err != nil {
			t.Errorf("%s: unexpected error %v", ok, err)
		}
	}
	for _, bad := range []string{"ftp://h:1", "h:1", "http://"} {
		if err := ValidateProxy(bad); err == nil {
			t.Errorf("%s: expected an error", bad)
		}
	}
}

func TestRetryAfter(t *testing.T) {
	if d := RetryAfter(http.Header{"Retry-After": {"30"}}); d != 30*time.Second {
		t.Fatalf("got %v", d)
	}
	if d := RetryAfter(http.Header{}); d != 0 {
		t.Fatalf("got %v", d)
	}
	future := time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat)
	if d := RetryAfter(http.Header{"Retry-After": {future}}); d < 80*time.Second || d > 91*time.Second {
		t.Fatalf("got %v", d)
	}
}

func TestMinuteCounter(t *testing.T) {
	var c minuteCounter
	now := time.Now()
	c.add(now.Add(-90 * time.Minute)) // outside the window
	c.add(now.Add(-30 * time.Minute))
	c.add(now)
	c.add(now)
	if got := c.total(now); got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
}
