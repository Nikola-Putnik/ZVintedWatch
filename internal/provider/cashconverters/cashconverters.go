// Package cashconverters implements the Cash Converters Belgium provider
// (www.cashconverters.be), a chain of second-hand stores whose online shop
// runs on PrestaShop.
//
// How it talks to the website (no account needed):
//
//  1. Search: any product list page (search or category) returns JSON when it
//     is requested with ajax=1 and XHR headers. The module asks for the newest
//     items first (order=product.date_add.desc, a sort order the website
//     supports but does not offer in its menu), so one request per refresh is
//     enough. Results include the short description, so description rules cost
//     no extra request.
//  2. Store: the store holding an item is only shown on the product page. It is
//     read (streamed, stopping as soon as the store block is complete) for
//     listings that matched an alert, through the provider.Enricher hook.
//
// Everything specific to Cash Converters lives in this package; see
// docs/providers/cashconverters.md for maintenance notes.
package cashconverters

import (
	"github.com/Nikola-Putnik/ZVintedWatch/internal/httpx"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
)

const (
	// ID is the provider identifier.
	ID = "cashconverters"
	// domain is the only website supported by this module.
	domain = "cashconverters.be"
	// baseURL is the website origin.
	baseURL = "https://www.cashconverters.be"
	// perPage is the number of newest items requested per refresh. Cash
	// Converters adds far fewer items per minute than Vinted; 36 leaves a
	// comfortable margin between two refreshes.
	perPage = 36
	// pathParam carries the page path inside Query.Params so that it is part
	// of the query key. It is removed before the request is sent.
	pathParam = "_path"
)

// Provider is the Cash Converters implementation of provider.Provider and
// provider.Enricher.
type Provider struct {
	fetcher *httpx.Fetcher
}

// New creates the Cash Converters provider using the shared fetcher.
func New(f *httpx.Fetcher) *Provider {
	return &Provider{fetcher: f}
}

// Info implements provider.Provider.
func (p *Provider) Info() provider.Info {
	return provider.Info{
		ID:            ID,
		Name:          "Cash Converters",
		Color:         "#E2001A",
		Website:       baseURL,
		Domains:       []string{domain},
		DefaultDomain: domain,
		// A small website without a CDN: stay polite.
		MinIntervalSec:         30,
		RecommendedIntervalSec: 120,
		// Search results include the description (description_short).
		DescriptionInSearch: true,
		// Items are not "bumped" back to the top on this website.
		SequenceTolerance: 0,
		SearchPlaceholder: baseURL + "/fr/recherche?controller=search&s=… or plain keywords",
		SearchHelp: "Paste the address of a Cash Converters search or category page (store and price " +
			"filters are kept), or simply type keywords. The newest items are always checked first.",
	}
}

var (
	_ provider.Provider = (*Provider)(nil)
	_ provider.Enricher = (*Provider)(nil)
)
