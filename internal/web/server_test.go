package web

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/activity"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/engine"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/httpx"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/notify/telegram"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/store"
)

type fakeProvider struct{}

func (fakeProvider) Info() provider.Info {
	return provider.Info{ID: "fake", Name: "Fake", MinIntervalSec: 10, Domains: []string{"fake.test"}}
}
func (fakeProvider) ParseSearch(in provider.SearchInput) (provider.Query, error) {
	kw := strings.Join(strings.Fields(strings.ToLower(in.Text)), " ")
	if kw == "" {
		return provider.Query{}, provider.ErrInvalidSearch
	}
	return provider.Query{Provider: "fake", Domain: "fake.test", Params: url.Values{"q": {kw}}, Label: kw}, nil
}
func (fakeProvider) Search(context.Context, provider.Query) ([]model.Listing, error) {
	return []model.Listing{{Provider: "fake", ID: "1", Title: "Lego Technic", Price: 10}}, nil
}
func (fakeProvider) FetchDescription(context.Context, model.Listing) (string, error) { return "", nil }

type testServer struct {
	*httptest.Server
	t      *testing.T
	client *http.Client
	auth   *Auth
	bus    *activity.Bus
}

func newTestServer(t *testing.T, loopback bool) (*testServer, string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	bus := activity.NewBus(100, nil)
	reg := provider.NewRegistry(fakeProvider{})
	fetcher := httpx.New(bus, nil)
	eng := engine.New(st, reg, bus, nil, engine.Options{})
	tg := telegram.NewService(st, bus, "http://127.0.0.1:1")
	t.Cleanup(tg.Close)
	auth, generated, err := NewAuth(ctx, st, loopback)
	if err != nil {
		t.Fatal(err)
	}
	srv := New(Deps{Store: st, Bus: bus, Fetcher: fetcher, Registry: reg, Engine: eng, Telegram: tg, Auth: auth, Loopback: loopback})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	jar := newJar()
	return &testServer{Server: ts, t: t, client: &http.Client{Jar: jar}, auth: auth, bus: bus}, generated
}

func (s *testServer) do(method, path string, body any, csrf bool) (int, []byte) {
	s.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, s.URL+path, r)
	if csrf {
		req.Header.Set(csrfHeader, "1")
	}
	resp, err := s.client.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func TestPasswordGeneratedWhenListeningOnNetwork(t *testing.T) {
	ts, password := newTestServer(t, false)
	if len(password) != 16 {
		t.Fatalf("expected a generated password, got %q", password)
	}
	if code, _ := ts.do("GET", "/api/status", nil, false); code != http.StatusUnauthorized {
		t.Fatalf("expected 401 before login, got %d", code)
	}
	if code, _ := ts.do("POST", "/api/auth/login", map[string]string{"password": "wrong"}, true); code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a wrong password, got %d", code)
	}
	if code, _ := ts.do("POST", "/api/auth/login", map[string]string{"password": password}, true); code != http.StatusOK {
		t.Fatalf("login failed: %d", code)
	}
	if code, body := ts.do("GET", "/api/status", nil, false); code != http.StatusOK {
		t.Fatalf("expected 200 after login, got %d %s", code, body)
	}
	// The password cannot be removed while reachable from the network.
	code, _ := ts.do("PUT", "/api/auth/password", map[string]string{"current": password, "new": ""}, true)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 when removing the password, got %d", code)
	}
}

func TestCSRFHeaderRequired(t *testing.T) {
	ts, _ := newTestServer(t, true)
	code, _ := ts.do("POST", "/api/alerts", map[string]any{}, false)
	if code != http.StatusForbidden {
		t.Fatalf("expected 403 without the CSRF header, got %d", code)
	}
}

func TestAlertLifecycle(t *testing.T) {
	ts, _ := newTestServer(t, true)
	alert := map[string]any{
		"provider": "fake", "name": "Lego", "enabled": true, "searchInput": "Lego",
		"title":          map[string]any{"include": []string{"technic", " "}, "exclude": []string{}},
		"intervalMinSec": 30, "intervalMaxSec": 90,
	}
	code, body := ts.do("POST", "/api/alerts", alert, true)
	if code != http.StatusOK {
		t.Fatalf("create: %d %s", code, body)
	}
	var created model.Alert
	_ = json.Unmarshal(body, &created)
	if created.ID == 0 || len(created.Title.Include) != 1 || created.Title.IncludeMode != model.MatchAll {
		t.Fatalf("unexpected alert %+v", created)
	}

	alert["name"] = "Lego 2"
	alert["searchInput"] = "LEGO"
	if code, body := ts.do("POST", "/api/alerts", alert, true); code != http.StatusOK {
		t.Fatalf("create second: %d %s", code, body)
	}

	code, body = ts.do("GET", "/api/alerts?provider=fake", nil, false)
	var views []alertView
	_ = json.Unmarshal(body, &views)
	if code != http.StatusOK || len(views) != 2 || views[0].SharedWith != 1 || views[0].SearchLabel != "lego" {
		t.Fatalf("unexpected alert list %d %s", code, body)
	}

	alert["intervalMinSec"] = 1
	if code, body := ts.do("POST", "/api/alerts", alert, true); code != http.StatusBadRequest ||
		!strings.Contains(string(body), "at least 10 seconds") {
		t.Fatalf("expected a validation error, got %d %s", code, body)
	}

	if code, _ := ts.do("POST", "/api/alerts/"+itoa(created.ID)+"/enabled", map[string]bool{"enabled": false}, true); code != 200 {
		t.Fatalf("disable failed: %d", code)
	}
	code, body = ts.do("GET", "/api/searches", nil, false)
	var searches []engine.SearchStatus
	_ = json.Unmarshal(body, &searches)
	if len(searches) != 1 || len(searches[0].Alerts) != 1 {
		t.Fatalf("expected one search serving one alert, got %s", body)
	}

	code, body = ts.do("POST", "/api/alerts/preview", alert, true)
	if code != http.StatusOK || !strings.Contains(string(body), "Lego Technic") {
		t.Fatalf("preview failed: %d %s", code, body)
	}

	if code, _ := ts.do("DELETE", "/api/alerts/"+itoa(created.ID), nil, true); code != http.StatusOK {
		t.Fatalf("delete failed: %d", code)
	}
	if code, _ := ts.do("GET", "/api/alerts/"+itoa(created.ID), nil, false); code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", code)
	}
}

func TestMatcherEndpoint(t *testing.T) {
	ts, _ := newTestServer(t, true)
	code, body := ts.do("POST", "/api/matcher/test", map[string]any{
		"filter": map[string]any{"include": []string{"god of war|gow"}}, "text": "GoW Ragnarök"}, true)
	if code != 200 || !strings.Contains(string(body), `"match":true`) {
		t.Fatalf("unexpected response %d %s", code, body)
	}
	code, _ = ts.do("POST", "/api/matcher/test", map[string]any{
		"filter": map[string]any{"include": []string{"/[/"}}, "text": "x"}, true)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an invalid regex, got %d", code)
	}
}

func TestDestinationValidation(t *testing.T) {
	ts, _ := newTestServer(t, true)
	for _, chat := range []string{"123456", "-1001234567890", "@my_channel"} {
		if code, body := ts.do("POST", "/api/destinations", map[string]string{"name": "x", "chatId": chat}, true); code != 200 {
			t.Errorf("%s: %d %s", chat, code, body)
		}
	}
	for _, chat := range []string{"", "abc", "@ab", "12 34"} {
		if code, _ := ts.do("POST", "/api/destinations", map[string]string{"name": "x", "chatId": chat}, true); code != 400 {
			t.Errorf("%q: expected 400, got %d", chat, code)
		}
	}
}

func TestActivityStream(t *testing.T) {
	ts, _ := newTestServer(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/activity/stream", nil)
	resp, err := ts.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("unexpected content type %q", ct)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		ts.bus.Publish(activity.Event{Kind: activity.KindSystem, Message: "hello stream"})
	}()
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if line := sc.Text(); strings.HasPrefix(line, "data: ") && strings.Contains(line, "hello stream") {
			return
		}
	}
	t.Fatal("event not received")
}

func TestStaticFallback(t *testing.T) {
	ts, _ := newTestServer(t, true)
	code, body := ts.do("GET", "/some/client/route", nil, false)
	if code != 200 || !strings.Contains(string(body), "<html") {
		t.Fatalf("expected the HTML page, got %d", code)
	}
	if code, _ := ts.do("GET", "/api/nope", nil, false); code != 404 {
		t.Fatalf("expected 404 for unknown API routes, got %d", code)
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// jar is a minimal cookie jar for the tests.
type jar struct{ cookies map[string]*http.Cookie }

func newJar() *jar { return &jar{cookies: map[string]*http.Cookie{}} }

func (j *jar) SetCookies(_ *url.URL, cookies []*http.Cookie) {
	for _, c := range cookies {
		j.cookies[c.Name] = c
	}
}

func (j *jar) Cookies(*url.URL) []*http.Cookie {
	var out []*http.Cookie
	for _, c := range j.cookies {
		if c.MaxAge >= 0 {
			out = append(out, c)
		}
	}
	return out
}
