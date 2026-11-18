package vinted

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/httpx"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
)

// Search implements provider.Provider: one catalogue request returning the
// newest listings (plus a session request when the session must be renewed).
func (p *Provider) Search(ctx context.Context, q provider.Query) ([]model.Listing, error) {
	lease, err := p.fetcher.Acquire(ctx, ID)
	if err != nil {
		return nil, err
	}
	defer lease.Release()

	sess, err := p.ensureSession(ctx, lease, q.Domain, false)
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	for k, v := range q.Params {
		params[k] = v
	}
	params.Set("order", "newest_first")
	params.Set("page", "1")
	params.Set("per_page", strconv.Itoa(perPage))
	apiURL := "https://api." + q.Domain + "/svc-catalogue/items?" + params.Encode()

	for attempt := 0; ; attempt++ {
		resp, err := lease.Do(ctx, &httpx.Request{
			Method:  "GET",
			URL:     apiURL,
			Headers: apiHeaders(q.Domain, sess),
			Purpose: "search",
		})
		if err != nil {
			return nil, err
		}
		switch {
		case resp.Status == 200:
			return parseCatalog(resp.Body, q.Domain)
		case (resp.Status == 401 || resp.Status == 403) && attempt == 0:
			// The session expired or was invalidated: renew it once.
			if sess, err = p.ensureSession(ctx, lease, q.Domain, true); err != nil {
				return nil, err
			}
		case resp.Status == 403:
			lease.Block("search refused (HTTP 403)", 0)
			return nil, fmt.Errorf("vinted refused the search (HTTP 403)")
		case resp.Status == 429:
			return nil, fmt.Errorf("vinted rate limit reached (HTTP 429)")
		default:
			return nil, fmt.Errorf("unexpected response from vinted (HTTP %d)", resp.Status)
		}
	}
}

// catalogResponse is the subset of the svc-catalogue/items response we use.
type catalogResponse struct {
	Items []catalogItem `json:"items"`
}

type money struct {
	Amount       string `json:"amount"`
	CurrencyCode string `json:"currency_code"`
}

type catalogItem struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	URL           string `json:"url"`
	ContentSource string `json:"content_source"`
	Promoted      bool   `json:"promoted"`
	Price         money  `json:"price"`
	ItemBox       struct {
		FirstLine  string `json:"first_line"`
		SecondLine string `json:"second_line"`
	} `json:"item_box"`
	Photo *struct {
		URL         string `json:"url"`
		FullSizeURL string `json:"full_size_url"`
	} `json:"photo"`
}

// parseCatalog converts the API response into listings.
func parseCatalog(body []byte, domain string) ([]model.Listing, error) {
	var resp catalogResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode vinted search response: %w", err)
	}
	listings := make([]model.Listing, 0, len(resp.Items))
	for _, it := range resp.Items {
		if it.ID == 0 {
			continue
		}
		l := model.Listing{
			Provider: ID,
			ID:       strconv.FormatInt(it.ID, 10),
			Title:    strings.TrimSpace(it.Title),
			Currency: it.Price.CurrencyCode,
			URL:      absoluteURL(domain, it.URL, it.ID),
			Promoted: it.Promoted || strings.Contains(it.ContentSource, "promoted"),
			// Listing ids are assigned sequentially when listings are created.
			Sequence: it.ID,
		}
		l.Price, _ = strconv.ParseFloat(it.Price.Amount, 64)
		if b := strings.TrimSpace(it.ItemBox.FirstLine); b != "" && !strings.EqualFold(b, l.Title) {
			l.Brand = b
		}
		l.Size, l.Condition = splitSecondLine(it.ItemBox.SecondLine)
		if it.Photo != nil {
			l.PhotoURL = it.Photo.URL
			if l.PhotoURL == "" {
				l.PhotoURL = it.Photo.FullSizeURL
			}
		}
		listings = append(listings, l)
	}
	return listings, nil
}

// splitSecondLine splits "M · Very good" into size and condition. Items
// without a size (games, books...) only carry the condition.
func splitSecondLine(s string) (size, condition string) {
	parts := strings.Split(s, "·")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	switch len(parts) {
	case 0:
		return "", ""
	case 1:
		return "", parts[0]
	default:
		return strings.Join(parts[:len(parts)-1], " · "), parts[len(parts)-1]
	}
}

func absoluteURL(domain, path string, id int64) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if path == "" {
		path = "/items/" + strconv.FormatInt(id, 10)
	}
	return "https://www." + domain + path
}
