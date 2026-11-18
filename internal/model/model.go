// Package model defines the domain types shared across ZVintedWatch:
// alerts, listings, matches, Telegram destinations and polled searches.
//
// These types are plain data structures with JSON tags so they can be stored
// in SQLite and exposed through the HTTP API without extra mapping layers.
package model

import "time"

// MatchMode controls how the "include" keywords of a TextFilter are combined.
type MatchMode string

const (
	// MatchAll requires every include term to be present (logical AND).
	MatchAll MatchMode = "all"
	// MatchAny requires at least one include term to be present (logical OR).
	MatchAny MatchMode = "any"
)

// TextFilter describes keyword rules applied to a piece of text (a listing
// title or description). See docs/keyword-syntax.md for the term syntax.
type TextFilter struct {
	// Include lists the terms that must be present (combined with IncludeMode).
	Include []string `json:"include"`
	// IncludeMode is "all" (default) or "any".
	IncludeMode MatchMode `json:"includeMode"`
	// Exclude lists the terms that reject the text when any of them is present.
	Exclude []string `json:"exclude"`
}

// IsEmpty reports whether the filter has no rule at all (it accepts any text).
func (f TextFilter) IsEmpty() bool {
	return len(f.Include) == 0 && len(f.Exclude) == 0
}

// Alert is a user-defined watch rule. Alerts that resolve to the same search
// query share a single polling job, so adding alerts does not necessarily add
// requests to the marketplace.
type Alert struct {
	ID       int64  `json:"id"`
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`

	// SearchInput is either a search URL copied from the marketplace website
	// or plain keywords.
	SearchInput string `json:"searchInput"`
	// Domain is the marketplace domain used when SearchInput is keywords
	// (for example "vinted.fr"). Ignored for URLs, which carry their domain.
	Domain string `json:"domain"`

	Title       TextFilter `json:"title"`
	Description TextFilter `json:"description"`

	// PriceMin and PriceMax optionally restrict the listing price. They are
	// applied locally, which lets alerts with different budgets share a search.
	PriceMin *float64 `json:"priceMin"`
	PriceMax *float64 `json:"priceMax"`

	// IntervalMinSec and IntervalMaxSec define the refresh interval in seconds.
	// Equal values mean a fixed interval; otherwise a random delay in the range
	// is drawn before every refresh.
	IntervalMinSec int `json:"intervalMinSec"`
	IntervalMaxSec int `json:"intervalMaxSec"`

	// DestinationIDs lists the Telegram destinations that receive matches.
	DestinationIDs []int64 `json:"destinationIds"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// NeedsDescription reports whether evaluating the alert requires the listing
// description.
func (a Alert) NeedsDescription() bool {
	return !a.Description.IsEmpty()
}

// PriceAccepts reports whether a price satisfies the alert price range.
// A price of zero or less is treated as unknown and always accepted.
func (a Alert) PriceAccepts(price float64) bool {
	if price <= 0 {
		return true
	}
	if a.PriceMin != nil && price < *a.PriceMin {
		return false
	}
	if a.PriceMax != nil && price > *a.PriceMax {
		return false
	}
	return true
}

// Listing is a marketplace item as returned by a provider.
type Listing struct {
	Provider string `json:"provider"`
	// ID is the marketplace item identifier, unique within a provider.
	ID    string `json:"id"`
	Title string `json:"title"`
	// Description is only set when HasDescription is true. Providers that do
	// not return descriptions in search results fill it lazily, on demand.
	Description    string `json:"description,omitempty"`
	HasDescription bool   `json:"hasDescription"`

	Price    float64 `json:"price"`
	Currency string  `json:"currency"`

	Brand     string `json:"brand,omitempty"`
	Size      string `json:"size,omitempty"`
	Condition string `json:"condition,omitempty"`

	URL      string `json:"url"`
	PhotoURL string `json:"photoUrl,omitempty"`

	// Promoted marks sponsored results. They are ignored by the engine because
	// they are usually old listings pushed to the top of the results.
	Promoted bool `json:"promoted"`

	// Sequence is an optional, monotonically increasing creation order (for
	// example a sequential listing id). It lets the engine recognise old
	// listings that resurface at the top of the results ("bumped" listings).
	// Zero means unknown.
	Sequence int64 `json:"-"`
}

// Match records that a listing satisfied an alert.
type Match struct {
	ID        int64     `json:"id"`
	AlertID   int64     `json:"alertId"`
	AlertName string    `json:"alertName"`
	Provider  string    `json:"provider"`
	Listing   Listing   `json:"listing"`
	MatchedAt time.Time `json:"matchedAt"`
	// Delivery is "pending", "sent", "failed" or "none" (no destination).
	Delivery string `json:"delivery"`
}

// Delivery states of a Match.
const (
	DeliveryPending = "pending"
	DeliverySent    = "sent"
	DeliveryFailed  = "failed"
	DeliveryNone    = "none"
)

// Destination is a Telegram chat (private chat, group or channel) that
// receives notifications.
type Destination struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// ChatID is a numeric chat id ("123456", "-100123456") or a public channel
	// username ("@mychannel").
	ChatID    string    `json:"chatId"`
	CreatedAt time.Time `json:"createdAt"`
}

// Search is the persisted state of a polled search query. Searches are derived
// from alerts: one Search exists per distinct (provider, query key).
type Search struct {
	ID       int64  `json:"id"`
	Provider string `json:"provider"`
	// Key is the canonical query identity (see provider.Query.Key).
	Key string `json:"key"`
	// Label is a short human-readable summary of the query.
	Label string `json:"label"`
	// WebURL opens the same search on the marketplace website.
	WebURL string `json:"webUrl"`
	// Seeded is true once the current results have been recorded as "seen".
	// The first poll of a search never sends notifications.
	Seeded bool `json:"seeded"`
	// Watermark is the highest listing Sequence seen by the search (see
	// Listing.Sequence); 0 when the provider does not provide sequences.
	Watermark    int64     `json:"-"`
	LastPolledAt time.Time `json:"lastPolledAt"`
	LastStatus   string    `json:"lastStatus"`
	LastError    string    `json:"lastError"`
	CreatedAt    time.Time `json:"createdAt"`
}
