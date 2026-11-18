package vinted

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/httpx"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
)

// maxItemScan bounds how much of an item page is read while looking for the
// description (the JSON-LD block usually sits within the first 150 KB).
const maxItemScan = 2 << 20

// errNoDescription is returned when an item page holds no description.
var errNoDescription = errors.New("no description found in the item page")

// FetchDescription implements provider.Provider by reading the item page.
func (p *Provider) FetchDescription(ctx context.Context, l model.Listing) (string, error) {
	u, err := url.Parse(l.URL)
	if err != nil {
		return "", err
	}
	domain := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")

	lease, err := p.fetcher.Acquire(ctx, ID)
	if err != nil {
		return "", err
	}
	defer lease.Release()

	var desc string
	resp, err := lease.Do(ctx, &httpx.Request{
		Method:  "GET",
		URL:     l.URL,
		Headers: documentHeaders(domain, "same-origin", "https://www."+domain+"/catalog"),
		Purpose: "item details",
		Consume: func(r io.Reader) error {
			var err error
			desc, err = extractDescription(r)
			return err
		},
	})
	if resp != nil {
		switch {
		case resp.Status == 403:
			lease.Block("item page refused (HTTP 403)", 0)
			return "", fmt.Errorf("vinted refused the item page (HTTP 403)")
		case resp.Status == 404 || resp.Status == 410:
			return "", fmt.Errorf("%w (HTTP %d)", provider.ErrListingUnavailable, resp.Status)
		case resp.Status != 200:
			return "", fmt.Errorf("unexpected response from vinted (HTTP %d)", resp.Status)
		}
	}
	if err != nil {
		return "", err
	}
	return desc, nil
}

var (
	ldScriptOpen = []byte(`application/ld+json`)
	scriptClose  = []byte(`</script>`)
	ogDescFirst  = regexp.MustCompile(`<meta[^>]+property="og:description"[^>]+content="([^"]*)"`)
	ogDescLast   = regexp.MustCompile(`<meta[^>]+content="([^"]*)"[^>]+property="og:description"`)
)

// extractDescription streams an item page and returns the description from
// the JSON-LD Product block, falling back to the og:description meta tag
// (which may be truncated) when no JSON-LD block is found.
func extractDescription(r io.Reader) (string, error) {
	buf := make([]byte, 0, 256<<10)
	chunk := make([]byte, 32<<10)
	scanned := 0 // offset up to which complete JSON-LD blocks were checked
	for len(buf) < maxItemScan {
		n, err := r.Read(chunk)
		buf = append(buf, chunk[:n]...)

		desc, next, found := findProductDescription(buf, scanned)
		if found {
			return desc, nil
		}
		scanned = next

		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
	}
	if m := ogDescFirst.FindSubmatch(buf); m != nil {
		return html.UnescapeString(string(m[1])), nil
	}
	if m := ogDescLast.FindSubmatch(buf); m != nil {
		return html.UnescapeString(string(m[1])), nil
	}
	return "", errNoDescription
}

// findProductDescription looks for complete JSON-LD blocks starting at
// offset from. It returns the description of the first Product block, or the
// offset from which the next call should resume.
func findProductDescription(buf []byte, from int) (desc string, next int, found bool) {
	for {
		i := bytes.Index(buf[from:], ldScriptOpen)
		if i < 0 {
			// Keep a small overlap in case the marker is split across chunks.
			return "", max(from, len(buf)-len(ldScriptOpen)), false
		}
		start := from + i
		gt := bytes.IndexByte(buf[start:], '>')
		if gt < 0 {
			return "", start, false
		}
		bodyStart := start + gt + 1
		end := bytes.Index(buf[bodyStart:], scriptClose)
		if end < 0 {
			return "", start, false // block not fully received yet
		}
		block := buf[bodyStart : bodyStart+end]
		if d, ok := productDescription(block); ok {
			return d, 0, true
		}
		from = bodyStart + end + len(scriptClose)
	}
}

func productDescription(block []byte) (string, bool) {
	var doc struct {
		Type        any    `json:"@type"`
		Description string `json:"description"`
	}
	if json.Unmarshal(bytes.TrimSpace(block), &doc) != nil {
		return "", false
	}
	if t, _ := doc.Type.(string); t != "Product" {
		return "", false
	}
	return strings.TrimSpace(doc.Description), true
}
