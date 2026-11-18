// Package vinted implements the Vinted marketplace provider.
//
// How it talks to Vinted (no account needed):
//
//  1. Session: a HEAD request to https://www.vinted.<tld>/ makes Vinted set an
//     anonymous session in the lane's cookie jar (access_token_web, anon_id).
//     The session is reused until it expires or a request is refused.
//  2. Search: GET https://api.vinted.<tld>/svc-catalogue/items with the token
//     as a Bearer header. Results contain title, price, brand, size,
//     condition and photo, but not the description.
//  3. Description: the item web page embeds a JSON-LD "Product" block holding
//     the full description. The page is streamed and reading stops as soon as
//     the block is found (roughly the first 150 KB of a ~2 MB page).
//
// Vinted changes its private API from time to time. Endpoints, parameter
// names and response fields are isolated in this package; see
// docs/providers/vinted.md for maintenance notes.
package vinted

import (
	"github.com/Nikola-Putnik/ZVintedWatch/internal/httpx"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
)

// ID is the provider identifier.
const ID = "vinted"

// perPage is the number of listings requested per poll. A larger page costs
// the same single request and leaves more margin for busy searches between
// two refreshes.
const perPage = 48

// Provider is the Vinted implementation of provider.Provider.
type Provider struct {
	fetcher *httpx.Fetcher
}

// New creates the Vinted provider using the shared fetcher.
func New(f *httpx.Fetcher) *Provider {
	return &Provider{fetcher: f}
}

// Info implements provider.Provider.
func (p *Provider) Info() provider.Info {
	return provider.Info{
		ID:                     ID,
		Name:                   "Vinted",
		Color:                  "#09B1BA",
		Website:                "https://www.vinted.com",
		Domains:                domainList(),
		DefaultDomain:          "vinted.fr",
		MinIntervalSec:         10,
		RecommendedIntervalSec: 60,
		DescriptionInSearch:    false,
		SearchPlaceholder:      "https://www.vinted.fr/catalog?search_text=… or plain keywords",
		SearchHelp: "Paste the address of a Vinted search page (apply the category, brand, size, " +
			"condition and price filters you want first), or simply type keywords. " +
			"Results are always sorted by newest first.",
	}
}

var _ provider.Provider = (*Provider)(nil)
