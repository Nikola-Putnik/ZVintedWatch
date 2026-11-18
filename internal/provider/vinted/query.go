package vinted

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
)

// Website filter parameters and the attribute they map to in the catalogue
// API. The API expects "attribute_ids[<attribute>]=1,2,3". Old-style names are
// silently ignored by the API (results come back unfiltered), so every known
// alias must be translated here.
var attributeAliases = map[string]string{
	"catalog":                 "catalog",
	"catalog_ids":             "catalog",
	"catalog_id":              "catalog",
	"brand_ids":               "brand",
	"brand_id":                "brand",
	"size_ids":                "size",
	"status_ids":              "status",
	"status":                  "status",
	"color_ids":               "color",
	"material_ids":            "material",
	"video_game_platform_ids": "video_game_platform",
	"video_game_rating_ids":   "video_game_rating",
	"patterns_ids":            "patterns",
	"pattern_ids":             "patterns",
}

// passthroughParams keep their name in the API.
var passthroughParams = map[string]bool{
	"price_from": true,
	"price_to":   true,
	"currency":   true,
}

// listParams keep their name but are comma-joined.
var listParams = map[string]bool{
	"country_ids": true,
	"city_ids":    true,
}

// ignoredParams are website-only parameters (paging, sorting, tracking).
var ignoredParams = map[string]bool{
	"order": true, "page": true, "per_page": true, "time": true, "search_id": true,
	"disabled_personalization": true, "search_by_image_uuid": true, "ref": true,
}

var (
	attributeParam = regexp.MustCompile(`^attribute_ids\[([a-z0-9_]+)\]$`)
	pathFilter     = regexp.MustCompile(`^/(catalog|brand)/(\d+)(?:-[^/]*)?/?$`)
)

// ParseSearch implements provider.Provider. It accepts a Vinted search URL
// (any country domain) or plain keywords searched on in.Domain.
func (p *Provider) ParseSearch(in provider.SearchInput) (provider.Query, error) {
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return provider.Query{}, fmt.Errorf("%w: enter a Vinted search URL or keywords", provider.ErrInvalidSearch)
	}
	if looksLikeURL(text) {
		return parseURL(text)
	}

	domain := strings.ToLower(strings.TrimSpace(in.Domain))
	if domain == "" {
		domain = p.Info().DefaultDomain
	}
	if _, ok := domains[domain]; !ok {
		return provider.Query{}, fmt.Errorf("%w: unknown Vinted domain %q", provider.ErrInvalidSearch, domain)
	}
	params := url.Values{}
	params.Set("search_text", normalizeKeywords(text))
	return buildQuery(domain, params, ""), nil
}

func looksLikeURL(s string) bool {
	l := strings.ToLower(s)
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") ||
		strings.HasPrefix(l, "www.vinted.") || strings.HasPrefix(l, "vinted.")
}

func parseURL(raw string) (provider.Query, error) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return provider.Query{}, fmt.Errorf("%w: %v", provider.ErrInvalidSearch, err)
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if _, ok := domains[host]; !ok {
		return provider.Query{}, fmt.Errorf("%w: %q is not a Vinted website address", provider.ErrInvalidSearch, u.Hostname())
	}

	attrs := map[string][]string{}
	params := url.Values{}

	// Category and brand pages: /catalog/1037-outerwear, /brand/53-nike
	if m := pathFilter.FindStringSubmatch(u.Path); m != nil {
		attrs[m[1]] = append(attrs[m[1]], m[2])
	}

	for rawKey, values := range u.Query() {
		key := strings.TrimSuffix(rawKey, "[]")
		switch {
		case key == "search_text":
			if kw := normalizeKeywords(strings.Join(values, " ")); kw != "" {
				params.Set("search_text", kw)
			}
		case passthroughParams[key]:
			if v := strings.TrimSpace(values[0]); v != "" {
				params.Set(key, v)
			}
		case listParams[key]:
			if ids := splitIDs(values); len(ids) > 0 {
				params.Set(key, strings.Join(ids, ","))
			}
		case ignoredParams[key]:
		default:
			if m := attributeParam.FindStringSubmatch(rawKey); m != nil {
				attrs[m[1]] = append(attrs[m[1]], values...)
			} else if attr, ok := attributeAliases[key]; ok {
				attrs[attr] = append(attrs[attr], values...)
			} else if strings.HasSuffix(key, "_ids") {
				// Unknown filter family: follow the documented naming scheme.
				attr := strings.TrimSuffix(key, "_ids")
				attrs[attr] = append(attrs[attr], values...)
			}
		}
	}

	for attr, values := range attrs {
		if ids := splitIDs(values); len(ids) > 0 {
			params.Set("attribute_ids["+attr+"]", strings.Join(ids, ","))
		}
	}
	return buildQuery(host, params, raw), nil
}

// buildQuery assembles the Query, its label and its website URL.
func buildQuery(domain string, params url.Values, webURL string) provider.Query {
	if webURL == "" {
		w := url.Values{}
		w.Set("search_text", params.Get("search_text"))
		w.Set("order", "newest_first")
		webURL = "https://www." + domain + "/catalog?" + w.Encode()
	}

	label := "All items"
	if kw := params.Get("search_text"); kw != "" {
		label = "“" + kw + "”"
	}
	filters := 0
	for k := range params {
		if k != "search_text" && k != "currency" {
			filters++
		}
	}
	label += " · " + domain
	switch filters {
	case 0:
	case 1:
		label += " · 1 filter"
	default:
		label += " · " + strconv.Itoa(filters) + " filters"
	}

	return provider.Query{Provider: ID, Domain: domain, Params: params, Label: label, WebURL: webURL}
}

// normalizeKeywords lowercases and collapses whitespace so that equivalent
// searches typed differently share the same polling job (Vinted search is
// case-insensitive).
func normalizeKeywords(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// splitIDs flattens values that may already be comma-separated, keeps numeric
// ids only, removes duplicates and sorts them numerically for a stable key.
func splitIDs(values []string) []string {
	seen := map[int64]bool{}
	var ids []int64
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			n, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err != nil || seen[n] {
				continue
			}
			seen[n] = true
			ids = append(ids, n)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]string, len(ids))
	for i, n := range ids {
		out[i] = strconv.FormatInt(n, 10)
	}
	return out
}
