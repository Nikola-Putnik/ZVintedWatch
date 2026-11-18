package vinted

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/activity"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/httpx"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
)

func TestParseSearchURL(t *testing.T) {
	p := New(nil)
	cases := []struct {
		name, in   string
		wantDomain string
		want       map[string]string
	}{
		{
			name:       "old style array filters are translated",
			in:         "https://www.vinted.fr/catalog?search_text=Jeux%20PS5&catalog[]=3026&brand_ids[]=12&brand_ids[]=5&size_ids[]=207&status_ids[]=6&price_to=30&currency=EUR&order=price_low_to_high&time=1700000000",
			wantDomain: "vinted.fr",
			want: map[string]string{
				"search_text":            "jeux ps5",
				"attribute_ids[catalog]": "3026",
				"attribute_ids[brand]":   "5,12",
				"attribute_ids[size]":    "207",
				"attribute_ids[status]":  "6",
				"price_to":               "30",
				"currency":               "EUR",
			},
		},
		{
			name:       "new style attribute filters pass through",
			in:         "https://www.vinted.be/catalog?attribute_ids%5Bbrand%5D=53,14&search_text=sweat",
			wantDomain: "vinted.be",
			want: map[string]string{
				"search_text":          "sweat",
				"attribute_ids[brand]": "14,53",
			},
		},
		{
			name:       "category page path",
			in:         "https://www.vinted.de/catalog/1037-outerwear",
			wantDomain: "vinted.de",
			want:       map[string]string{"attribute_ids[catalog]": "1037"},
		},
		{
			name:       "brand page path without scheme",
			in:         "vinted.co.uk/brand/53-nike?color_ids[]=1",
			wantDomain: "vinted.co.uk",
			want:       map[string]string{"attribute_ids[brand]": "53", "attribute_ids[color]": "1"},
		},
		{
			name:       "empty values are dropped",
			in:         "https://www.vinted.it/catalog?search_text=&price_from=&brand_ids[]=",
			wantDomain: "vinted.it",
			want:       map[string]string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := p.ParseSearch(provider.SearchInput{Text: tc.in})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Domain != tc.wantDomain {
				t.Fatalf("domain = %q, want %q", q.Domain, tc.wantDomain)
			}
			if len(q.Params) != len(tc.want) {
				t.Fatalf("params = %v, want %v", q.Params, tc.want)
			}
			for k, v := range tc.want {
				if got := q.Params.Get(k); got != v {
					t.Fatalf("param %s = %q, want %q (all: %v)", k, got, v, q.Params)
				}
			}
		})
	}
}

func TestParseSearchKeywordsShareKeyWithEquivalentURL(t *testing.T) {
	p := New(nil)
	a, err := p.ParseSearch(provider.SearchInput{Text: "  Jeux   PS5 ", Domain: "vinted.fr"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.ParseSearch(provider.SearchInput{Text: "https://www.vinted.fr/catalog?search_text=jeux+ps5&order=newest_first"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Key() != b.Key() {
		t.Fatalf("keys differ:\n%s\n%s", a.Key(), b.Key())
	}
	if a.Label != "“jeux ps5” · vinted.fr" {
		t.Fatalf("unexpected label %q", a.Label)
	}
	if !strings.HasPrefix(a.WebURL, "https://www.vinted.fr/catalog?") {
		t.Fatalf("unexpected web URL %q", a.WebURL)
	}
}

func TestParseSearchErrors(t *testing.T) {
	p := New(nil)
	for _, in := range []provider.SearchInput{
		{Text: ""},
		{Text: "https://www.example.com/catalog?search_text=x"},
		{Text: "ps5", Domain: "vinted.xyz"},
	} {
		if _, err := p.ParseSearch(in); !errors.Is(err, provider.ErrInvalidSearch) {
			t.Errorf("%+v: expected ErrInvalidSearch, got %v", in, err)
		}
	}
}

// catalogFixture is a synthetic API response modelled on the real format.
const catalogFixture = `{
  "items": [
    {"id": 1002, "title": "God of War Ragnarok PS5", "url": "/items/1002-god-of-war-ragnarok-ps5",
     "content_source": "search", "promoted": false,
     "price": {"amount": "25.00", "currency_code": "EUR"},
     "item_box": {"first_line": "Sony", "second_line": "Very good"},
     "photo": {"url": "https://images.example/1002.webp", "full_size_url": "https://images.example/1002-full.webp"}},
    {"id": 1001, "title": "Hoodie", "url": "/items/1001-hoodie", "content_source": "search_promoted_items",
     "price": {"amount": "12.5", "currency_code": "EUR"},
     "item_box": {"first_line": "Nike", "second_line": "M · Good"}},
    {"id": 1003, "title": "Book", "url": "https://www.vinted.fr/items/1003-book", "promoted": true,
     "price": {"amount": "3.00", "currency_code": "EUR"},
     "item_box": {"first_line": "Book", "second_line": "11 years / 146 cm · New with tags"}}
  ],
  "pagination": {"current_page": 1}
}`

func TestParseCatalog(t *testing.T) {
	got, err := parseCatalog([]byte(catalogFixture), "vinted.fr")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 listings, got %d", len(got))
	}
	want := []model.Listing{
		{Provider: "vinted", ID: "1002", Title: "God of War Ragnarok PS5", Price: 25, Currency: "EUR", Brand: "Sony",
			Condition: "Very good", URL: "https://www.vinted.fr/items/1002-god-of-war-ragnarok-ps5",
			PhotoURL: "https://images.example/1002.webp", Sequence: 1002},
		{Provider: "vinted", ID: "1001", Title: "Hoodie", Price: 12.5, Currency: "EUR", Brand: "Nike", Size: "M",
			Condition: "Good", URL: "https://www.vinted.fr/items/1001-hoodie", Promoted: true, Sequence: 1001},
		{Provider: "vinted", ID: "1003", Title: "Book", Price: 3, Currency: "EUR", Size: "11 years / 146 cm",
			Condition: "New with tags", URL: "https://www.vinted.fr/items/1003-book", Promoted: true, Sequence: 1003},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("listing %d:\n got %+v\nwant %+v", i, got[i], want[i])
		}
	}
}

const itemPageFixture = `<!DOCTYPE html><html><head>
<meta property="og:description" content="Short &amp; truncated…">
</head><body>
<script type="application/ld+json">{"@type":"BreadcrumbList","itemListElement":[]}</script>
<div>` + "%s" + `</div>
<script type="application/ld+json">{"@type":"Product","name":"Hoodie","description":"Full description\nwith \"quotes\" & accents: éàü"}</script>
</body></html>`

func TestExtractDescriptionFromJSONLD(t *testing.T) {
	page := fmt.Sprintf(itemPageFixture, strings.Repeat("x", 100_000))
	// OneByteReader stresses the incremental parsing across chunk boundaries.
	for name, r := range map[string]io.Reader{
		"whole":    strings.NewReader(page),
		"one byte": iotest.OneByteReader(strings.NewReader(page)),
		"half":     iotest.HalfReader(strings.NewReader(page)),
	} {
		got, err := extractDescription(r)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if want := "Full description\nwith \"quotes\" & accents: éàü"; got != want {
			t.Fatalf("%s: got %q want %q", name, got, want)
		}
	}
}

func TestExtractDescriptionFallsBackToOpenGraph(t *testing.T) {
	page := `<html><head><meta property="og:description" content="Short &amp; sweet"></head><body></body></html>`
	got, err := extractDescription(strings.NewReader(page))
	if err != nil || got != "Short & sweet" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := extractDescription(strings.NewReader("<html></html>")); !errors.Is(err, errNoDescription) {
		t.Fatalf("expected errNoDescription, got %v", err)
	}
}

func TestTokenExpired(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	jwt := func(exp int64) string {
		payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, exp)))
		return "header." + payload + ".signature"
	}
	if tokenExpired(jwt(now.Unix()+3600), now) {
		t.Fatal("token valid for an hour reported as expired")
	}
	if !tokenExpired(jwt(now.Unix()+30), now) {
		t.Fatal("token expiring in 30s should be renewed")
	}
	if tokenExpired("not-a-jwt", now) {
		t.Fatal("opaque tokens are considered valid")
	}
}

// fakeVinted simulates the session, search and item endpoints.
type fakeVinted struct {
	mu          sync.Mutex
	hasSession  bool
	searchCodes []int // status codes returned by successive searches
	requests    []string
}

func (f *fakeVinted) Do(_ context.Context, req *httpx.Request) (*httpx.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req.Method+" "+req.URL)
	switch {
	case req.Method == "HEAD":
		f.hasSession = true
		return &httpx.Response{Status: 200, Header: http.Header{}}, nil
	case strings.Contains(req.URL, "/svc-catalogue/items"):
		code := 200
		if len(f.searchCodes) > 0 {
			code, f.searchCodes = f.searchCodes[0], f.searchCodes[1:]
		}
		if !hasHeader(req, "authorization", "Bearer tok") {
			code = 401
		}
		return &httpx.Response{Status: code, Header: http.Header{}, Body: []byte(catalogFixture)}, nil
	case strings.Contains(req.URL, "/items/"):
		resp := &httpx.Response{Status: 200, Header: http.Header{}}
		page := fmt.Sprintf(itemPageFixture, "")
		return resp, req.Consume(strings.NewReader(page))
	}
	return &httpx.Response{Status: 404, Header: http.Header{}}, nil
}

func (f *fakeVinted) Cookies(string) []httpx.Cookie {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.hasSession {
		return nil
	}
	return []httpx.Cookie{{Name: "access_token_web", Value: "tok"}, {Name: "anon_id", Value: "anon"}}
}

func (f *fakeVinted) Close() {}

func hasHeader(req *httpx.Request, name, value string) bool {
	for _, h := range req.Headers {
		if h.Name == name && h.Value == value {
			return true
		}
	}
	return false
}

func newFakeProvider(t *testing.T, fake *fakeVinted) *Provider {
	t.Helper()
	f := httpx.New(activity.NewBus(50, nil), func(string, time.Duration) (httpx.Transport, error) { return fake, nil })
	if err := f.Configure(ID, httpx.Settings{MinGapMs: 1}); err != nil {
		t.Fatal(err)
	}
	return New(f)
}

func TestSearchBootstrapsSessionAndRetriesOnce(t *testing.T) {
	fake := &fakeVinted{searchCodes: []int{401, 200}}
	p := newFakeProvider(t, fake)
	q, _ := p.ParseSearch(provider.SearchInput{Text: "ps5", Domain: "vinted.fr"})

	listings, err := p.Search(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if len(listings) != 3 {
		t.Fatalf("expected 3 listings, got %d", len(listings))
	}
	// HEAD (session) → search (401) → HEAD (renew) → search (200)
	if len(fake.requests) != 4 || !strings.HasPrefix(fake.requests[0], "HEAD") || !strings.HasPrefix(fake.requests[2], "HEAD") {
		t.Fatalf("unexpected request sequence: %v", fake.requests)
	}
	if !strings.Contains(fake.requests[1], "order=newest_first") || !strings.Contains(fake.requests[1], "per_page=48") {
		t.Fatalf("search must request the newest items: %s", fake.requests[1])
	}

	// The session is reused: the next search costs a single request.
	fake.requests = nil
	if _, err := p.Search(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if len(fake.requests) != 1 {
		t.Fatalf("expected one request with a cached session, got %v", fake.requests)
	}
}

func TestSearchBlockedPutsLaneInCooldown(t *testing.T) {
	fake := &fakeVinted{searchCodes: []int{403, 403}}
	p := newFakeProvider(t, fake)
	q, _ := p.ParseSearch(provider.SearchInput{Text: "ps5", Domain: "vinted.fr"})
	if _, err := p.Search(context.Background(), q); err == nil {
		t.Fatal("expected an error")
	}
	_, err := p.Search(context.Background(), q)
	var cool *httpx.CooldownError
	if !errors.As(err, &cool) {
		t.Fatalf("expected cooldown, got %v", err)
	}
}

func TestFetchDescription(t *testing.T) {
	fake := &fakeVinted{}
	p := newFakeProvider(t, fake)
	desc, err := p.FetchDescription(context.Background(), model.Listing{URL: "https://www.vinted.fr/items/1-x"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(desc, "Full description") {
		t.Fatalf("unexpected description %q", desc)
	}
}
