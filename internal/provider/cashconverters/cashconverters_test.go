package cashconverters

import (
	"context"
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

func TestParseSearch(t *testing.T) {
	p := New(nil)
	cases := []struct {
		name, in  string
		wantPath  string
		wantParam map[string]string
		wantLabel string
	}{
		{
			name:      "french search from the website",
			in:        "https://www.cashconverters.be/fr/recherche?controller=search&s=Mac+Mini++M4",
			wantPath:  "/fr/recherche",
			wantParam: map[string]string{"controller": "search", "s": "mac mini m4"},
			wantLabel: "“mac mini m4” · cashconverters.be",
		},
		{
			name:      "dutch search with store filter and website-only parameters",
			in:        "https://www.cashconverters.be/nl/zoeken?controller=search&s=iphone&order=product.price.asc&page=3&q=Winkels-WAVRE-BERCHEM+STE%5C-AGATHE",
			wantPath:  "/nl/zoeken",
			wantParam: map[string]string{"controller": "search", "s": "iphone", "q": `Winkels-WAVRE-BERCHEM STE\-AGATHE`},
			wantLabel: "“iphone” · cashconverters.be · WAVRE, BERCHEM STE-AGATHE",
		},
		{
			name:      "category page with price filter",
			in:        "cashconverters.be/fr/134-informatique?q=Prix-€-100-500&resultsPerPage=12",
			wantPath:  "/fr/134-informatique",
			wantParam: map[string]string{"q": "Prix-€-100-500"},
			wantLabel: "Category “informatique” · cashconverters.be · 1 filter",
		},
		{
			name:      "plain keywords",
			in:        "  Nintendo   Switch ",
			wantPath:  "/fr/recherche",
			wantParam: map[string]string{"controller": "search", "s": "nintendo switch"},
			wantLabel: "“nintendo switch” · cashconverters.be",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := p.ParseSearch(provider.SearchInput{Text: tc.in})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := q.Params.Get(pathParam); got != tc.wantPath {
				t.Fatalf("path = %q, want %q", got, tc.wantPath)
			}
			if len(q.Params) != len(tc.wantParam)+1 {
				t.Fatalf("params = %v, want %v", q.Params, tc.wantParam)
			}
			for k, v := range tc.wantParam {
				if got := q.Params.Get(k); got != v {
					t.Fatalf("param %s = %q, want %q", k, got, v)
				}
			}
			if q.Label != tc.wantLabel {
				t.Fatalf("label = %q, want %q", q.Label, tc.wantLabel)
			}
			if !strings.HasPrefix(q.WebURL, baseURL+tc.wantPath) {
				t.Fatalf("web URL = %q", q.WebURL)
			}
		})
	}
}

func TestParseSearchSharesKeys(t *testing.T) {
	p := New(nil)
	a, _ := p.ParseSearch(provider.SearchInput{Text: "Mac mini M4"})
	b, _ := p.ParseSearch(provider.SearchInput{Text: "https://www.cashconverters.be/fr/recherche?controller=search&s=mac+mini+m4&order=product.name.asc"})
	if a.Key() != b.Key() {
		t.Fatalf("equivalent searches must share a key:\n%s\n%s", a.Key(), b.Key())
	}
	c, _ := p.ParseSearch(provider.SearchInput{Text: "https://www.cashconverters.be/fr/134-informatique"})
	d, _ := p.ParseSearch(provider.SearchInput{Text: "https://www.cashconverters.be/fr/135-telephonie"})
	if c.Key() == d.Key() {
		t.Fatal("different categories must have different keys")
	}
}

func TestParseSearchErrors(t *testing.T) {
	p := New(nil)
	for _, in := range []string{
		"",
		"https://www.example.com/fr/recherche?s=x",
		"https://www.cashconverters.be/fr/recherche?controller=search&s=",
		"https://www.cashconverters.be/fr/ordinateurs-de-bureau/1-mac-mini.html",
		"https://www.cashconverters.be/fr/content/1-livraison",
	} {
		if _, err := p.ParseSearch(provider.SearchInput{Text: in}); !errors.Is(err, provider.ErrInvalidSearch) {
			t.Errorf("%q: expected ErrInvalidSearch, got %v", in, err)
		}
	}
}

// searchFixture is a synthetic response modelled on the website's JSON.
const searchFixture = `{
  "rendered_products": "<div>…</div>",
  "products": [
    {"id_product": "2000002", "name": "Mac mini M4 16 GB 256 GB ",
     "description_short": "<p>Mac mini M4 &amp; power cable</p>",
     "price_amount": 499.99, "regular_price_amount": 499.99, "has_discount": false,
     "url": "https://www.cashconverters.be/fr/ordinateurs/2000002-mac-mini.html",
     "category_name": "ordinateurs de bureau",
     "cover": {"bySize": {"large_default": {"url": "https://www.cashconverters.be/1-large_default/mac.jpg"}}}},
    {"id_product": 2000001, "name": "Dock for Mac mini",
     "description_short": "Dock", "price_amount": 60, "regular_price_amount": 80, "has_discount": true,
     "url": "https://www.cashconverters.be/fr/accessoires/2000001-dock.html", "category_name": "accessoires",
     "cover": {"large": {"url": "https://www.cashconverters.be/2-large/dock.jpg"}}},
    {"id_product": "", "name": "broken", "url": ""}
  ],
  "pagination": {"total_items": 2}
}`

func TestParseSearchResponse(t *testing.T) {
	got, err := parseSearchResponse([]byte(searchFixture))
	if err != nil {
		t.Fatal(err)
	}
	want := []model.Listing{
		{Provider: ID, ID: "2000002", Title: "Mac mini M4 16 GB 256 GB", Description: "Mac mini M4 & power cable",
			HasDescription: true, Price: 499.99, Currency: "EUR", Category: "ordinateurs de bureau",
			URL:      "https://www.cashconverters.be/fr/ordinateurs/2000002-mac-mini.html",
			PhotoURL: "https://www.cashconverters.be/1-large_default/mac.jpg", Sequence: 2000002},
		{Provider: ID, ID: "2000001", Title: "Dock for Mac mini", Description: "Dock", HasDescription: true,
			Price: 60, Currency: "EUR", Category: "accessoires", Condition: "−25% (was 80.00 €)",
			URL:      "https://www.cashconverters.be/fr/accessoires/2000001-dock.html",
			PhotoURL: "https://www.cashconverters.be/2-large/dock.jpg", Sequence: 2000001},
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d listings, got %d: %+v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("listing %d:\n got %+v\nwant %+v", i, got[i], want[i])
		}
	}
}

const productPage = `<!DOCTYPE html><html><head><title>Item</title></head><body>
<div>` + "%s" + `</div>
<div class="product-shop" style="margin-top: 20px">
  <h2>Magasin</h2>
  <div class="shop-infos">
    <p>L'article est disponible dans le magasin: Testville</p>
    <p> Rue de l'Exemple 1, 1000 Testville </p>
    <p>Numéro de téléphone: <a href="tel:000/000.000">000/000.000</a></p>
  </div>
</div>
<footer>` + "%s" + `</footer></body></html>`

func TestExtractStore(t *testing.T) {
	page := fmt.Sprintf(productPage, strings.Repeat("x", 120_000), strings.Repeat("y", 50_000))
	for name, r := range map[string]io.Reader{
		"whole":    strings.NewReader(page),
		"one byte": iotest.OneByteReader(strings.NewReader(page)),
	} {
		got, err := extractStore(r)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if want := "Testville – Rue de l'Exemple 1, 1000 Testville"; got != want {
			t.Fatalf("%s: got %q want %q", name, got, want)
		}
	}
	if _, err := extractStore(strings.NewReader("<html><body>sold</body></html>")); !errors.Is(err, errNoStore) {
		t.Fatalf("expected errNoStore, got %v", err)
	}
}

// fakeSite simulates the search endpoint and product pages.
type fakeSite struct {
	mu          sync.Mutex
	status      int
	lastURL     string
	lastHeaders []httpx.Header
}

func (f *fakeSite) Do(_ context.Context, req *httpx.Request) (*httpx.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastURL, f.lastHeaders = req.URL, req.Headers
	status := f.status
	if status == 0 {
		status = 200
	}
	resp := &httpx.Response{Status: status, Header: http.Header{}}
	if status != 200 {
		return resp, nil
	}
	if strings.Contains(req.URL, "ajax=1") {
		resp.Body = []byte(searchFixture)
		return resp, nil
	}
	return resp, req.Consume(strings.NewReader(fmt.Sprintf(productPage, "", "")))
}
func (f *fakeSite) Cookies(string) []httpx.Cookie { return nil }
func (f *fakeSite) Close()                        {}

func newFakeProvider(t *testing.T, site *fakeSite) *Provider {
	t.Helper()
	f := httpx.New(activity.NewBus(50, nil), func(string, time.Duration) (httpx.Transport, error) { return site, nil })
	if err := f.Configure(ID, httpx.Settings{MinGapMs: 1}); err != nil {
		t.Fatal(err)
	}
	return New(f)
}

func TestSearchRequestsNewestItemsAsJSON(t *testing.T) {
	site := &fakeSite{}
	p := newFakeProvider(t, site)
	q, _ := p.ParseSearch(provider.SearchInput{Text: "https://www.cashconverters.be/fr/recherche?controller=search&s=mac+mini&q=Magasins-WAVRE"})

	listings, err := p.Search(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if len(listings) != 2 {
		t.Fatalf("expected 2 listings, got %d", len(listings))
	}
	for _, want := range []string{baseURL + "/fr/recherche?", "order=product.date_add.desc", "ajax=1",
		"resultsPerPage=36", "s=mac+mini", "q=Magasins-WAVRE"} {
		if !strings.Contains(site.lastURL, want) {
			t.Errorf("request URL %q does not contain %q", site.lastURL, want)
		}
	}
	if strings.Contains(site.lastURL, pathParam) {
		t.Errorf("internal parameter leaked into the request: %s", site.lastURL)
	}
	found := false
	for _, h := range site.lastHeaders {
		if h.Name == "x-requested-with" && h.Value == "XMLHttpRequest" {
			found = true
		}
	}
	if !found {
		t.Error("the XHR header is required to receive JSON")
	}
}

func TestSearchRefusedPutsLaneInCooldown(t *testing.T) {
	site := &fakeSite{status: 403}
	p := newFakeProvider(t, site)
	q, _ := p.ParseSearch(provider.SearchInput{Text: "ps5"})
	if _, err := p.Search(context.Background(), q); err == nil {
		t.Fatal("expected an error")
	}
	var cool *httpx.CooldownError
	if _, err := p.Search(context.Background(), q); !errors.As(err, &cool) {
		t.Fatalf("expected a cooldown, got %v", err)
	}
}

func TestEnrich(t *testing.T) {
	site := &fakeSite{}
	p := newFakeProvider(t, site)
	l := model.Listing{URL: baseURL + "/nl/desktops/2000002-mac-mini.html"}
	if err := p.Enrich(context.Background(), &l); err != nil {
		t.Fatal(err)
	}
	if l.Location != "Testville – Rue de l'Exemple 1, 1000 Testville" {
		t.Fatalf("unexpected location %q", l.Location)
	}

	gone := &fakeSite{status: 404}
	p = newFakeProvider(t, gone)
	if err := p.Enrich(context.Background(), &l); !errors.Is(err, provider.ErrListingUnavailable) {
		t.Fatalf("expected ErrListingUnavailable, got %v", err)
	}
}
