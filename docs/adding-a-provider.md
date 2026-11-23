# Adding a marketplace

ZVintedWatch is designed so that a new marketplace is **one Go package plus one line of
registration**. The engine (grouping, scheduling, new-listing detection, keyword matching), the
Telegram notifier and the web interface (a new section appears in the sidebar) work unchanged.

Two real modules show the pattern: [Vinted](../internal/provider/vinted) (private JSON API, session,
HTML description) and [Cash Converters](../internal/provider/cashconverters) (PrestaShop JSON
listing, store read from the product page). This guide uses a hypothetical module for
2ememain.be / 2dehands.be / Marktplaats as an example.

## 1. Understand the contract

[`internal/provider/provider.go`](../internal/provider/provider.go):

```go
type Provider interface {
    Info() Info
    ParseSearch(in SearchInput) (Query, error)
    Search(ctx context.Context, q Query) ([]model.Listing, error)
    FetchDescription(ctx context.Context, l model.Listing) (string, error)
}
```

| Method | Responsibility |
|---|---|
| `Info` | Static metadata: id, display name, brand colour, country domains, minimum and recommended refresh interval, help texts for the alert editor, and capabilities (`DescriptionInSearch`, `SequenceTolerance`). |
| `ParseSearch` | Turn what the user typed (a website URL or keywords + domain) into a normalized `Query`. Two inputs meaning the same search **must** produce the same `Query.Key()` so their alerts share one request: normalize case, drop empty, paging and sorting parameters, sort values. Return errors wrapping `provider.ErrInvalidSearch`. |
| `Search` | Return the **first page of results, newest first**, as `model.Listing`s. One request when possible. |
| `FetchDescription` | Return the full description of a listing. Only called when a listing passed the title rules of an alert that has description rules. Return an error wrapping `provider.ErrListingUnavailable` when the listing was removed (it will not be retried). |

`model.Listing` fields to fill: `Provider`, `ID` (unique within the site), `Title`, `Price`,
`Currency`, `URL` (absolute), and when available `Brand`, `Size`, `Condition`, `Category`,
`Location`, `PhotoURL`, `Promoted` (sponsored results are ignored), `Description` +
`HasDescription` (if the search results include it), and `Sequence` (a value that grows with
listing creation, such as a sequential id, used to ignore old listings pushed back to the top).

### Optional: details for matched listings

Some information is only available on the listing page (for example the store holding the item on
Cash Converters). Implement the optional `provider.Enricher` interface:

```go
type Enricher interface {
    Enrich(ctx context.Context, l *model.Listing) error
}
```

The engine calls `Enrich` **once per listing that matched an alert**, never for the other
listings, then sends the enriched listing to Telegram and stores it in the history. If it fails,
the notification is sent anyway.

## 2. Write the package

Create `internal/provider/<site>/`. All HTTP requests **must** go through the shared
`httpx.Fetcher`, so that pacing, cooldowns, proxies, the browser fingerprint and the live console
apply automatically:

```go
package marktplaats

import (
    "context"
    "encoding/json"
    "fmt"
    "net/url"
    "strings"

    "github.com/Nikola-Putnik/ZVintedWatch/internal/httpx"
    "github.com/Nikola-Putnik/ZVintedWatch/internal/model"
    "github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
)

const ID = "2ememain"

type Provider struct{ fetcher *httpx.Fetcher }

func New(f *httpx.Fetcher) *Provider { return &Provider{fetcher: f} }

func (p *Provider) Info() provider.Info {
    return provider.Info{
        ID:                     ID,
        Name:                   "2ememain",
        Color:                  "#f68a1f",
        Website:                "https://www.2ememain.be",
        Domains:                []string{"2ememain.be", "2dehands.be", "marktplaats.nl"},
        DefaultDomain:          "2ememain.be",
        MinIntervalSec:         20,
        RecommendedIntervalSec: 90,
        DescriptionInSearch:    true, // the search API returns descriptions
        SearchPlaceholder:      "https://www.2ememain.be/q/… or plain keywords",
        SearchHelp:             "Paste the address of a 2ememain search or type keywords.",
    }
}

func (p *Provider) ParseSearch(in provider.SearchInput) (provider.Query, error) {
    kw := strings.Join(strings.Fields(strings.ToLower(in.Text)), " ")
    if kw == "" {
        return provider.Query{}, fmt.Errorf("%w: enter keywords", provider.ErrInvalidSearch)
    }
    // A real module would also accept website URLs and translate their filters.
    params := url.Values{"query": {kw}}
    return provider.Query{
        Provider: ID, Domain: in.Domain, Params: params,
        Label:  "“" + kw + "” · " + in.Domain,
        WebURL: "https://www." + in.Domain + "/q/" + url.PathEscape(kw) + "/",
    }, nil
}

func (p *Provider) Search(ctx context.Context, q provider.Query) ([]model.Listing, error) {
    lease, err := p.fetcher.Acquire(ctx, ID) // waits for a free, non-paused lane
    if err != nil {
        return nil, err
    }
    defer lease.Release()

    params := url.Values{}
    for k, v := range q.Params {
        params[k] = v
    }
    params.Set("sortBy", "SORT_INDEX")
    params.Set("sortOrder", "DECREASING")
    params.Set("limit", "50")

    resp, err := lease.Do(ctx, &httpx.Request{
        Method:  "GET",
        URL:     "https://www." + q.Domain + "/lrp/api/search?" + params.Encode(),
        Headers: []httpx.Header{
            {Name: "user-agent", Value: httpx.UserAgent},
            {Name: "accept", Value: "application/json"},
            {Name: "accept-encoding", Value: httpx.AcceptEncoding},
        },
        Purpose: "search", // label shown in the live console
    })
    if err != nil {
        return nil, err
    }
    if resp.Status == 403 {
        lease.Block("search refused (HTTP 403)", 0) // pause this lane with backoff
        return nil, fmt.Errorf("access refused (HTTP 403)")
    }
    if resp.Status != 200 {
        return nil, fmt.Errorf("unexpected response (HTTP %d)", resp.Status)
    }
    // HTTP 429 responses put the lane in cooldown automatically.

    var body struct {
        Listings []struct {
            ItemID      string `json:"itemId"`
            Title       string `json:"title"`
            Description string `json:"description"`
            VipURL      string `json:"vipUrl"`
            PriceInfo   struct {
                PriceCents int `json:"priceCents"`
            } `json:"priceInfo"`
            PriorityProduct string `json:"priorityProduct"`
        } `json:"listings"`
    }
    if err := json.Unmarshal(resp.Body, &body); err != nil {
        return nil, err
    }
    out := make([]model.Listing, 0, len(body.Listings))
    for _, it := range body.Listings {
        out = append(out, model.Listing{
            Provider: ID, ID: it.ItemID, Title: it.Title,
            Description: it.Description, HasDescription: true,
            Price: float64(it.PriceInfo.PriceCents) / 100, Currency: "EUR",
            URL:      "https://www." + q.Domain + it.VipURL,
            Promoted: it.PriorityProduct != "" && it.PriorityProduct != "NONE",
        })
    }
    return out, nil
}

func (p *Provider) FetchDescription(ctx context.Context, l model.Listing) (string, error) {
    return l.Description, nil // never called: DescriptionInSearch is true
}
```

The response format above is illustrative; check the real one with your browser's developer
tools.

## 3. Register it

Add the provider to `NewRegistry` in [`internal/app/app.go`](../internal/app/app.go):

```go
func NewRegistry(f *httpx.Fetcher) *provider.Registry {
    return provider.NewRegistry(
        vinted.New(f),
        marktplaats.New(f),
    )
}
```

That's all: the site appears in the sidebar, the dashboard, the network settings and the
*Sites* settings, alerts can be created for it, and the `debug` command recognises its URLs
(`Registry.ForURL` uses `Info.Domains`).

## 4. Test it

- Unit tests next to the code (`<site>_test.go`): URL parsing table, response parsing from a
  **synthetic** fixture (never commit real responses: they contain other people's data), and a
  fake `httpx.Transport` for the request flow. See `internal/provider/vinted/vinted_test.go`.
- Live check without touching the database:
  ```sh
  go run ./cmd/zvintedwatch debug search -provider 2ememain "lego technic" -domain 2ememain.be
  ```
- Run the full application, create an alert and watch the **Live activity** page.

## Module lifecycle

Site modules are independent from each other and from the rest of the application:

| I want to… | What to do | What happens to the rest |
|---|---|---|
| **Pause** a site | Toggle it off on its page or in *Settings → Sites* (no code change). | No request is sent to that site; its alerts, history and settings are kept. Other sites are not affected. When resumed, its searches start from a fresh baseline (listings posted meanwhile are not sent). |
| **Update** a module | Change files in `internal/provider/<site>/` (and its doc page). | Nothing else changes. Each site has its own network lanes, pacing and cooldowns. |
| **Remove** a module | Delete its folder and its line in `app.NewRegistry`. | Its alerts stay in the database untouched, are not refreshed, and are reported on the dashboard. They work again if the module comes back. |

A bug in a module cannot stop the application: a crash (panic) during a search, a description
read or an enrichment is turned into an ordinary error for that refresh, shown in the live
console, and the other sites keep running.

## Guidelines

- Keep the number of requests per refresh to **one** whenever possible.
- Prefer endpoints that return JSON over HTML scraping; when you must read HTML, stream it with
  `Request.Consume` and stop as soon as you have what you need.
- Report refusals with `lease.Block(...)` so the lane pauses instead of hammering the site.
- Use realistic browser headers (`httpx.UserAgent`, `httpx.SecChUa`…) in a browser-like order.
- Document the endpoints and parameters in `docs/providers/<site>.md`, as for
  [Vinted](providers/vinted.md).
