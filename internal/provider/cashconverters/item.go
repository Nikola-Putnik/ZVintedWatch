package cashconverters

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/httpx"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
)

// maxPageScan bounds how much of a product page is read while looking for
// the store block (it usually ends around 105 KB into a ~146 KB page).
const maxPageScan = 1 << 20

// errNoStore is returned when a product page holds no store information.
var errNoStore = errors.New("no store information found on the product page")

// Enrich implements provider.Enricher: it reads the store holding the item
// from the product page. It is only called for listings that matched an alert.
func (p *Provider) Enrich(ctx context.Context, l *model.Listing) error {
	u, err := url.Parse(l.URL)
	if err != nil {
		return err
	}
	lease, err := p.fetcher.Acquire(ctx, ID)
	if err != nil {
		return err
	}
	defer lease.Release()

	var store string
	resp, err := lease.Do(ctx, &httpx.Request{
		Method:  "GET",
		URL:     l.URL,
		Headers: documentHeaders(languageOf(u.Path)),
		Purpose: "store details",
		Consume: func(r io.Reader) error {
			var err error
			store, err = extractStore(r)
			return err
		},
	})
	if resp != nil {
		switch {
		case resp.Status == 403:
			lease.Block("product page refused (HTTP 403)", 0)
			return fmt.Errorf("cash converters refused the product page (HTTP 403)")
		case resp.Status == 404 || resp.Status == 410:
			return fmt.Errorf("%w (HTTP %d)", provider.ErrListingUnavailable, resp.Status)
		case resp.Status != 200:
			return fmt.Errorf("unexpected response from cash converters (HTTP %d)", resp.Status)
		}
	}
	if err != nil {
		return err
	}
	l.Location = store
	return nil
}

// FetchDescription implements provider.Provider. Search results already carry
// the description (Info.DescriptionInSearch), so the engine never calls it.
func (p *Provider) FetchDescription(_ context.Context, l model.Listing) (string, error) {
	return l.Description, nil
}

var (
	shopInfosMarker = []byte(`class="shop-infos"`)
	paragraphClose  = []byte(`</p>`)
)

// extractStore streams a product page and returns "Store – address" from the
// block:
//
//	<div class="shop-infos">
//	  <p>L'article est disponible dans le magasin: Mouscron</p>
//	  <p> Rue de la Liesse 96, 7700 Mouscron </p>
//	  <p>Numéro de téléphone: …</p>
//
// Reading stops as soon as the first two paragraphs of the block are received.
func extractStore(r io.Reader) (string, error) {
	buf := make([]byte, 0, 192<<10)
	chunk := make([]byte, 32<<10)
	for len(buf) < maxPageScan {
		n, err := r.Read(chunk)
		buf = append(buf, chunk[:n]...)

		if start := bytes.Index(buf, shopInfosMarker); start >= 0 {
			block := buf[start:]
			if bytes.Count(block, paragraphClose) >= 2 {
				return parseStoreBlock(block)
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
	}
	if start := bytes.Index(buf, shopInfosMarker); start >= 0 {
		return parseStoreBlock(buf[start:]) // a block with a single paragraph
	}
	return "", errNoStore
}

// parseStoreBlock reads the store name and address from the first two
// paragraphs of the store block.
func parseStoreBlock(block []byte) (string, error) {
	var paragraphs []string
	rest := block
	for len(paragraphs) < 2 {
		open := bytes.Index(rest, []byte("<p"))
		if open < 0 {
			break
		}
		gt := bytes.IndexByte(rest[open:], '>')
		if gt < 0 {
			break
		}
		body := rest[open+gt+1:]
		end := bytes.Index(body, paragraphClose)
		if end < 0 {
			break
		}
		if text := cleanText(string(body[:end])); text != "" {
			paragraphs = append(paragraphs, text)
		}
		rest = body[end+len(paragraphClose):]
	}
	if len(paragraphs) == 0 {
		return "", errNoStore
	}

	// "L'article est disponible dans le magasin: Mouscron" → "Mouscron"
	name := paragraphs[0]
	if i := strings.LastIndex(name, ":"); i >= 0 && i < len(name)-1 {
		name = strings.TrimSpace(name[i+1:])
	}
	if len(paragraphs) == 1 || strings.Contains(strings.ToLower(paragraphs[1]), "téléphone") {
		return name, nil
	}
	return name + " – " + paragraphs[1], nil
}
