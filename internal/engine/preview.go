package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/activity"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
)

// ErrValidation is wrapped by Validate errors.
var ErrValidation = errors.New("invalid alert")

// Validate checks an alert before it is saved: known provider, parsable
// search, valid keyword rules and an interval above the provider minimum.
func (e *Engine) Validate(a model.Alert) error {
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("%w: the name is required", ErrValidation)
	}
	p, ok := e.registry.Get(a.Provider)
	if !ok {
		return fmt.Errorf("%w: unknown site %q", ErrValidation, a.Provider)
	}
	if _, err := p.ParseSearch(provider.SearchInput{Text: a.SearchInput, Domain: a.Domain}); err != nil {
		return fmt.Errorf("%w: %v", ErrValidation, err)
	}
	if _, err := compileAlert(a); err != nil {
		return fmt.Errorf("%w: %v", ErrValidation, err)
	}
	floor := p.Info().MinIntervalSec
	if a.IntervalMinSec < floor {
		return fmt.Errorf("%w: the refresh interval must be at least %d seconds", ErrValidation, floor)
	}
	if a.IntervalMaxSec < a.IntervalMinSec {
		return fmt.Errorf("%w: the maximum interval must be greater than or equal to the minimum", ErrValidation)
	}
	if a.IntervalMaxSec > 24*3600 {
		return fmt.Errorf("%w: the refresh interval cannot exceed 24 hours", ErrValidation)
	}
	if a.PriceMin != nil && a.PriceMax != nil && *a.PriceMin > *a.PriceMax {
		return fmt.Errorf("%w: the minimum price is greater than the maximum price", ErrValidation)
	}
	return nil
}

// SearchInfo describes how an alert's search will be polled.
type SearchInfo struct {
	Label  string `json:"label"`
	WebURL string `json:"webUrl"`
	Key    string `json:"key"`
	// SharedWith lists the other enabled alerts polling the same search: the
	// alert will not add any request.
	SharedWith []AlertRef `json:"sharedWith"`
}

// DescribeSearch parses a search input and reports which existing alerts
// already poll it. excludeAlertID removes the edited alert from the list.
func (e *Engine) DescribeSearch(providerID, input, domain string, excludeAlertID int64) (SearchInfo, error) {
	p, ok := e.registry.Get(providerID)
	if !ok {
		return SearchInfo{}, fmt.Errorf("%w: unknown site %q", provider.ErrInvalidSearch, providerID)
	}
	q, err := p.ParseSearch(provider.SearchInput{Text: input, Domain: domain})
	if err != nil {
		return SearchInfo{}, err
	}
	info := SearchInfo{Label: q.Label, WebURL: q.WebURL, Key: q.Key(), SharedWith: []AlertRef{}}
	e.mu.Lock()
	defer e.mu.Unlock()
	if j := e.jobs[q.Key()]; j != nil {
		for _, a := range j.alerts {
			if a.alert.ID != excludeAlertID {
				info.SharedWith = append(info.SharedWith, AlertRef{ID: a.alert.ID, Name: a.alert.Name})
			}
		}
	}
	return info, nil
}

// PreviewItem is one listing of a preview with its evaluation.
type PreviewItem struct {
	Listing model.Listing `json:"listing"`
	PriceOK bool          `json:"priceOk"`
	TitleOK bool          `json:"titleOk"`
	// Match is true when price and title rules pass. Description rules are
	// not evaluated by previews: that would cost one request per listing.
	Match bool `json:"match"`
}

// PreviewResult is the outcome of an alert preview.
type PreviewResult struct {
	Label  string `json:"label"`
	WebURL string `json:"webUrl"`
	// Cached is true when the results of the running job were reused, so the
	// preview did not send any request.
	Cached           bool          `json:"cached"`
	FetchedAt        time.Time     `json:"fetchedAt"`
	DescriptionRules bool          `json:"descriptionRules"`
	Matches          int           `json:"matches"`
	Items            []PreviewItem `json:"items"`
}

// Preview runs the search of an alert once and evaluates its price and title
// rules against the current results. When the search is already polled and
// its results are recent, they are reused instead of sending a request.
func (e *Engine) Preview(ctx context.Context, a model.Alert) (PreviewResult, error) {
	p, ok := e.registry.Get(a.Provider)
	if !ok {
		return PreviewResult{}, fmt.Errorf("%w: unknown site %q", ErrValidation, a.Provider)
	}
	q, err := p.ParseSearch(provider.SearchInput{Text: a.SearchInput, Domain: a.Domain})
	if err != nil {
		return PreviewResult{}, err
	}
	ca, err := compileAlert(a)
	if err != nil {
		return PreviewResult{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}

	res := PreviewResult{Label: q.Label, WebURL: q.WebURL, DescriptionRules: !ca.desc.IsEmpty(), Items: []PreviewItem{}}

	var listings []model.Listing
	e.mu.Lock()
	if j := e.jobs[q.Key()]; j != nil && len(j.lastResults) > 0 && time.Since(j.lastSuccess) < e.opts.PreviewCacheTTL {
		listings = j.lastResults
		res.Cached, res.FetchedAt = true, j.lastSuccess
	}
	e.mu.Unlock()

	if !res.Cached {
		sctx := activity.WithScope(ctx, activity.Scope{Provider: p.Info().ID, Search: "Preview: " + q.Label})
		all, err := p.Search(sctx, q)
		if err != nil {
			return PreviewResult{}, err
		}
		for _, l := range all {
			if !l.Promoted {
				listings = append(listings, l)
			}
		}
		res.FetchedAt = time.Now()
	}

	for _, l := range listings {
		it := PreviewItem{Listing: l, PriceOK: a.PriceAccepts(l.Price), TitleOK: ca.title.Match(l.Title)}
		it.Match = it.PriceOK && it.TitleOK
		if it.Match {
			res.Matches++
		}
		res.Items = append(res.Items, it)
	}
	return res, nil
}
