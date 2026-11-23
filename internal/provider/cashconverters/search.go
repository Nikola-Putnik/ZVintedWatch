package cashconverters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/httpx"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
)

// Search implements provider.Provider: one JSON request returning the newest
// items of a search or category page.
func (p *Provider) Search(ctx context.Context, q provider.Query) ([]model.Listing, error) {
	lease, err := p.fetcher.Acquire(ctx, ID)
	if err != nil {
		return nil, err
	}
	defer lease.Release()

	params := url.Values{}
	for k, v := range q.Params {
		params[k] = v
	}
	path := params.Get(pathParam)
	params.Del(pathParam)
	if path == "" {
		return nil, fmt.Errorf("invalid Cash Converters query (no page path)")
	}
	params.Set("order", "product.date_add.desc") // newest items first
	params.Set("resultsPerPage", strconv.Itoa(perPage))
	params.Set("page", "1")
	params.Set("ajax", "1")

	resp, err := lease.Do(ctx, &httpx.Request{
		Method:  "GET",
		URL:     baseURL + path + "?" + params.Encode(),
		Headers: xhrHeaders(q.WebURL, languageOf(path)),
		Purpose: "search",
	})
	if err != nil {
		return nil, err
	}
	switch {
	case resp.Status == 200:
		return parseSearchResponse(resp.Body)
	case resp.Status == 403:
		lease.Block("search refused (HTTP 403)", 0)
		return nil, fmt.Errorf("cash converters refused the search (HTTP 403)")
	case resp.Status == 429:
		return nil, fmt.Errorf("cash converters rate limit reached (HTTP 429)")
	default:
		return nil, fmt.Errorf("unexpected response from cash converters (HTTP %d)", resp.Status)
	}
}

// flexString accepts a JSON string or number (PrestaShop versions differ).
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	*f = flexString(string(b))
	return nil
}

type image struct {
	URL string `json:"url"`
}

// searchResponse is the subset of the PrestaShop listing JSON used here.
type searchResponse struct {
	Products []struct {
		ID                 flexString `json:"id_product"`
		Name               string     `json:"name"`
		DescriptionShort   string     `json:"description_short"`
		PriceAmount        float64    `json:"price_amount"`
		RegularPriceAmount float64    `json:"regular_price_amount"`
		HasDiscount        bool       `json:"has_discount"`
		URL                string     `json:"url"`
		CategoryName       string     `json:"category_name"`
		Cover              *struct {
			BySize map[string]image `json:"bySize"`
			Large  *image           `json:"large"`
		} `json:"cover"`
	} `json:"products"`
}

// parseSearchResponse converts the JSON listing into listings.
func parseSearchResponse(body []byte) ([]model.Listing, error) {
	var resp searchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode cash converters search response: %w", err)
	}
	listings := make([]model.Listing, 0, len(resp.Products))
	for _, it := range resp.Products {
		id := strings.TrimSpace(string(it.ID))
		if id == "" || it.URL == "" {
			continue
		}
		l := model.Listing{
			Provider:       ID,
			ID:             id,
			Title:          cleanText(it.Name),
			Description:    cleanText(it.DescriptionShort),
			HasDescription: true,
			Price:          it.PriceAmount,
			Currency:       "EUR",
			URL:            it.URL,
			Category:       cleanText(it.CategoryName),
		}
		// Product ids are assigned sequentially.
		l.Sequence, _ = strconv.ParseInt(id, 10, 64)
		if it.HasDiscount && it.RegularPriceAmount > it.PriceAmount && it.RegularPriceAmount > 0 {
			pct := math.Round((it.RegularPriceAmount - it.PriceAmount) / it.RegularPriceAmount * 100)
			l.Condition = fmt.Sprintf("−%.0f%% (was %.2f €)", pct, it.RegularPriceAmount)
		}
		if c := it.Cover; c != nil {
			if img, ok := c.BySize["large_default"]; ok && img.URL != "" {
				l.PhotoURL = img.URL
			} else if c.Large != nil {
				l.PhotoURL = c.Large.URL
			}
		}
		listings = append(listings, l)
	}
	return listings, nil
}

var (
	htmlTag    = regexp.MustCompile(`<[^>]*>`)
	whitespace = regexp.MustCompile(`\s+`)
)

// cleanText removes HTML tags and entities and collapses whitespace.
func cleanText(s string) string {
	s = htmlTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.TrimSpace(whitespace.ReplaceAllString(s, " "))
}

// languageOf returns "fr" or "nl" from a page path such as "/nl/zoeken".
func languageOf(path string) string {
	if strings.HasPrefix(path, "/nl/") {
		return "nl"
	}
	return "fr"
}

func acceptLanguage(lang string) string {
	if lang == "nl" {
		return "nl-BE,nl;q=0.9,fr;q=0.8,en;q=0.7"
	}
	return "fr-BE,fr;q=0.9,nl;q=0.8,en;q=0.7"
}

// xhrHeaders are the headers the website sends when it refreshes a product
// list with JavaScript.
func xhrHeaders(referer, lang string) []httpx.Header {
	return []httpx.Header{
		{Name: "sec-ch-ua-platform", Value: httpx.SecChUaPlatform},
		{Name: "x-requested-with", Value: "XMLHttpRequest"},
		{Name: "user-agent", Value: httpx.UserAgent},
		{Name: "accept", Value: "application/json, text/javascript, */*; q=0.01"},
		{Name: "sec-ch-ua", Value: httpx.SecChUa},
		{Name: "sec-ch-ua-mobile", Value: "?0"},
		{Name: "sec-fetch-site", Value: "same-origin"},
		{Name: "sec-fetch-mode", Value: "cors"},
		{Name: "sec-fetch-dest", Value: "empty"},
		{Name: "referer", Value: referer},
		{Name: "accept-encoding", Value: httpx.AcceptEncoding},
		{Name: "accept-language", Value: acceptLanguage(lang)},
	}
}

// documentHeaders are the headers of a top-level page navigation.
func documentHeaders(lang string) []httpx.Header {
	return []httpx.Header{
		{Name: "sec-ch-ua", Value: httpx.SecChUa},
		{Name: "sec-ch-ua-mobile", Value: "?0"},
		{Name: "sec-ch-ua-platform", Value: httpx.SecChUaPlatform},
		{Name: "upgrade-insecure-requests", Value: "1"},
		{Name: "user-agent", Value: httpx.UserAgent},
		{Name: "accept", Value: "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"},
		{Name: "sec-fetch-site", Value: "same-origin"},
		{Name: "sec-fetch-mode", Value: "navigate"},
		{Name: "sec-fetch-user", Value: "?1"},
		{Name: "sec-fetch-dest", Value: "document"},
		{Name: "referer", Value: baseURL + "/" + lang + "/"},
		{Name: "accept-encoding", Value: httpx.AcceptEncoding},
		{Name: "accept-language", Value: acceptLanguage(lang)},
	}
}
