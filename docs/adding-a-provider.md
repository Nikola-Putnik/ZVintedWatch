# Adding a marketplace

ZVintedWatch is designed so that a new marketplace is **one Go package plus one line of
registration**. The engine (grouping, scheduling, new-listing detection, keyword matching), the
Telegram notifier and the web interface (a new section appears in the sidebar) work unchanged.

This guide uses a hypothetical module for 2ememain.be / 2dehands.be / Marktplaats as an example.

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
`Currency`, `URL` (absolute), and when available `Brand`, `Size`, `Condition`, `PhotoURL`,
`Promoted` (sponsored results are ignored), `Description` + `HasDescription` (if the search
results include it), and `Sequence` (a value that grows with listing creation, such as a
sequential id, used to ignore old listings pushed back to the top).

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

That's all: the site appears in the sidebar, the dashboard and the network settings, and alerts
can be created for it.

## 4. Test it

- Unit tests next to the code (`<site>_test.go`): URL parsing table, response parsing from a
  **synthetic** fixture (never commit real responses: they contain other people's data), and a
  fake `httpx.Transport` for the request flow. See `internal/provider/vinted/vinted_test.go`.
- Live check without touching the database:
  ```sh
  go run ./cmd/zvintedwatch debug search -provider 2ememain "lego technic" -domain 2ememain.be
  ```
- Run the full application, create an alert and watch the **Live activity** page.

## Guidelines

- Keep the number of requests per refresh to **one** whenever possible.
- Prefer endpoints that return JSON over HTML scraping; when you must read HTML, stream it with
  `Request.Consume` and stop as soon as you have what you need.
- Report refusals with `lease.Block(...)` so the lane pauses instead of hammering the site.
- Use realistic browser headers (`httpx.UserAgent`, `httpx.SecChUa`…) in a browser-like order.
- Document the endpoints and parameters in `docs/providers/<site>.md`, as for
  [Vinted](providers/vinted.md).
