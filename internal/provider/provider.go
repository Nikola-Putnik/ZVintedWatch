// Package provider defines the contract every marketplace module implements
// and the registry the rest of the application uses to find them.
//
// Adding a marketplace means writing one package that implements Provider
// and registering it in internal/app. The engine, the web UI (a section per
// site is generated from Info) and the notifier work unchanged. See
// docs/adding-a-provider.md for a walkthrough.
package provider

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strings"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
)

var (
	// ErrInvalidSearch is wrapped by ParseSearch errors caused by user input.
	ErrInvalidSearch = errors.New("invalid search")
	// ErrListingUnavailable is wrapped by FetchDescription errors when the
	// listing was removed or sold; the engine then stops retrying it.
	ErrListingUnavailable = errors.New("listing no longer available")
)

// Info describes a provider to the UI and the engine.
type Info struct {
	// ID is a stable lowercase identifier stored in the database ("vinted").
	ID string `json:"id"`
	// Name is the display name ("Vinted").
	Name string `json:"name"`
	// Color is the brand accent colour used by the UI (CSS colour).
	Color string `json:"color"`
	// Website is the public homepage of the marketplace.
	Website string `json:"website"`
	// Domains lists the country domains a keyword search can target.
	Domains       []string `json:"domains"`
	DefaultDomain string   `json:"defaultDomain"`
	// MinIntervalSec is the hard lower bound for refresh intervals.
	MinIntervalSec int `json:"minIntervalSec"`
	// RecommendedIntervalSec is suggested by the UI for new alerts.
	RecommendedIntervalSec int `json:"recommendedIntervalSec"`
	// DescriptionInSearch is true when search results already include the
	// listing description, so FetchDescription is never needed.
	DescriptionInSearch bool `json:"descriptionInSearch"`
	// SequenceTolerance applies to providers that fill Listing.Sequence. A
	// listing never seen before is only treated as new when its Sequence is
	// at least the highest Sequence already seen minus this tolerance; older
	// listings that resurface (paid "bumps", relisting) are ignored. The
	// tolerance absorbs indexing delays. 0 disables the check.
	SequenceTolerance int64 `json:"-"`
	// SearchPlaceholder and SearchHelp are shown in the alert editor.
	SearchPlaceholder string `json:"searchPlaceholder"`
	SearchHelp        string `json:"searchHelp"`
}

// SearchInput is what the user typed in the alert editor.
type SearchInput struct {
	// Text is a search URL copied from the website or plain keywords.
	Text string
	// Domain is the country domain used for keyword searches.
	Domain string
}

// Query is a normalized, provider-specific search. Two alerts whose inputs
// normalize to the same Query share a single polling job.
type Query struct {
	Provider string `json:"provider"`
	Domain   string `json:"domain"`
	// Params are the provider API parameters, without paging and sorting.
	Params url.Values `json:"params"`
	// Label is a short human-readable summary ("ps5 · vinted.fr · 2 filters").
	Label string `json:"label"`
	// WebURL opens the same search on the marketplace website.
	WebURL string `json:"webUrl"`
}

// Key returns the canonical identity of the query. url.Values.Encode sorts
// parameters by name, so the key does not depend on parameter order.
func (q Query) Key() string {
	return q.Provider + "|" + q.Domain + "|" + q.Params.Encode()
}

// Provider is implemented by every marketplace module.
type Provider interface {
	// Info returns static metadata.
	Info() Info
	// ParseSearch validates user input and turns it into a Query. Errors
	// caused by the input should wrap ErrInvalidSearch.
	ParseSearch(in SearchInput) (Query, error)
	// Search fetches the newest listings of a query (first page only, sorted
	// newest first). Implementations must perform their HTTP requests through
	// the shared httpx.Fetcher so pacing and cooldowns apply.
	Search(ctx context.Context, q Query) ([]model.Listing, error)
	// FetchDescription returns the full description of a listing. It is only
	// called when an alert has description rules and the title already matched.
	FetchDescription(ctx context.Context, l model.Listing) (string, error)
}

// Enricher is optionally implemented by providers that can add details to a
// listing that matched an alert, such as the store or city holding the item,
// when search results do not include them. The engine calls Enrich at most
// once per matched listing, never for listings that match no alert, so the
// extra request only happens when a notification is about to be sent.
type Enricher interface {
	Enrich(ctx context.Context, l *model.Listing) error
}

// Registry holds the available providers.
type Registry struct {
	byID map[string]Provider
}

// NewRegistry creates a registry containing the given providers.
func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{byID: map[string]Provider{}}
	for _, p := range providers {
		r.byID[p.Info().ID] = p
	}
	return r
}

// Get returns a provider by id.
func (r *Registry) Get(id string) (Provider, bool) {
	p, ok := r.byID[id]
	return p, ok
}

// ForURL returns the provider whose domains include the host of rawURL
// ("https://www.vinted.fr/…" → Vinted). It returns false for plain keywords
// and unknown websites.
func (r *Registry) ForURL(rawURL string) (Provider, bool) {
	raw := strings.TrimSpace(rawURL)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil, false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	for _, p := range r.All() {
		for _, d := range p.Info().Domains {
			if host == d || strings.HasSuffix(host, "."+d) {
				return p, true
			}
		}
	}
	return nil, false
}

// All returns every provider sorted by display name.
func (r *Registry) All() []Provider {
	out := make([]Provider, 0, len(r.byID))
	for _, p := range r.byID {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Info().Name < out[j].Info().Name })
	return out
}
