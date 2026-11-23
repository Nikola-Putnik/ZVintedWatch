package cashconverters

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
)

// categorySegment matches category paths such as "134-informatique".
var categorySegment = regexp.MustCompile(`^\d+-[\p{L}0-9-]+$`)

// searchSlug is the path of the search page in each language.
var searchSlug = map[string]string{"fr": "recherche", "nl": "zoeken"}

// ParseSearch implements provider.Provider. It accepts the address of a
// search or category page of cashconverters.be (French or Dutch), or plain
// keywords searched on the French website.
func (p *Provider) ParseSearch(in provider.SearchInput) (provider.Query, error) {
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return provider.Query{}, fmt.Errorf("%w: enter a Cash Converters search address or keywords", provider.ErrInvalidSearch)
	}
	if looksLikeURL(text) {
		return parseURL(text)
	}
	params := url.Values{}
	params.Set("controller", "search")
	params.Set("s", normalizeKeywords(text))
	return buildQuery("/fr/recherche", params), nil
}

func looksLikeURL(s string) bool {
	l := strings.ToLower(s)
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") ||
		strings.HasPrefix(l, "www.cashconverters.") || strings.HasPrefix(l, "cashconverters.")
}

func parseURL(raw string) (provider.Query, error) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return provider.Query{}, fmt.Errorf("%w: %v", provider.ErrInvalidSearch, err)
	}
	if host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www."); host != domain {
		return provider.Query{}, fmt.Errorf("%w: %q is not a cashconverters.be address", provider.ErrInvalidSearch, u.Hostname())
	}

	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	lang := "fr"
	if len(segments) > 0 && (segments[0] == "fr" || segments[0] == "nl") {
		lang, segments = segments[0], segments[1:]
	}
	if len(segments) == 1 && segments[0] == "" {
		segments = nil
	}

	query := u.Query()
	params := url.Values{}
	var path string
	switch {
	case query.Get("controller") == "search" || (len(segments) == 1 &&
		(segments[0] == "recherche" || segments[0] == "zoeken" || segments[0] == "search")):
		kw := normalizeKeywords(query.Get("s"))
		if kw == "" {
			kw = normalizeKeywords(query.Get("search_query"))
		}
		if kw == "" {
			return provider.Query{}, fmt.Errorf("%w: this search address has no keywords", provider.ErrInvalidSearch)
		}
		path = "/" + lang + "/" + searchSlug[lang]
		params.Set("controller", "search")
		params.Set("s", kw)
	case len(segments) == 1 && categorySegment.MatchString(segments[0]):
		path = "/" + lang + "/" + segments[0]
	case len(segments) >= 2 && strings.HasSuffix(segments[len(segments)-1], ".html"):
		return provider.Query{}, fmt.Errorf("%w: this is a product page; paste the address of a search or category page instead",
			provider.ErrInvalidSearch)
	default:
		return provider.Query{}, fmt.Errorf("%w: paste the address of a search or category page of cashconverters.be",
			provider.ErrInvalidSearch)
	}

	// Facet filters (store, price) are kept exactly as the website encodes them.
	if facets := strings.TrimSpace(query.Get("q")); facets != "" {
		params.Set("q", facets)
	}
	return buildQuery(path, params), nil
}

// buildQuery assembles the Query, its label and its website URL.
func buildQuery(path string, params url.Values) provider.Query {
	web := baseURL + path
	if len(params) > 0 {
		web += "?" + params.Encode()
	}

	var label string
	if kw := params.Get("s"); kw != "" {
		label = "“" + kw + "”"
	} else {
		slug := path[strings.LastIndex(path, "/")+1:]
		if i := strings.Index(slug, "-"); i >= 0 {
			slug = slug[i+1:]
		}
		label = "Category “" + strings.ReplaceAll(slug, "-", " ") + "”"
	}
	label += " · " + domain
	if facets := params.Get("q"); facets != "" {
		if stores := storeNames(facets); len(stores) > 0 {
			label += " · " + strings.Join(stores, ", ")
		} else if n := len(strings.Split(facets, "/")); n == 1 {
			label += " · 1 filter"
		} else {
			label += " · " + strconv.Itoa(n) + " filters"
		}
	}

	key := url.Values{}
	for k, v := range params {
		key[k] = v
	}
	key.Set(pathParam, path)
	return provider.Query{Provider: ID, Domain: domain, Params: key, Label: label, WebURL: web}
}

// storeNames extracts the selected stores from a facet string such as
// "Magasins-WAVRE-ETTERBEEK/Prix-€-100-500" (dashes inside a value are
// escaped as "\-").
func storeNames(facets string) []string {
	for _, facet := range strings.Split(facets, "/") {
		name, values, ok := strings.Cut(facet, "-")
		if !ok || (name != "Magasins" && name != "Winkels") {
			continue
		}
		return splitFacetValues(values)
	}
	return nil
}

// splitFacetValues splits "WAVRE-BERCHEM STE\-AGATHE" into
// ["WAVRE", "BERCHEM STE-AGATHE"].
func splitFacetValues(s string) []string {
	var values []string
	var cur strings.Builder
	flush := func() {
		if v := strings.TrimSpace(cur.String()); v != "" {
			values = append(values, v)
		}
		cur.Reset()
	}
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && i+1 < len(s) && s[i+1] == '-':
			cur.WriteByte('-')
			i++
		case s[i] == '-':
			flush()
		default:
			cur.WriteByte(s[i])
		}
	}
	flush()
	return values
}

// normalizeKeywords lowercases and collapses whitespace so that equivalent
// searches typed differently share the same polling job.
func normalizeKeywords(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}
