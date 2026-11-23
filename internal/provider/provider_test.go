package provider

import (
	"context"
	"testing"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
)

type stub struct{ info Info }

func (s stub) Info() Info                                             { return s.info }
func (s stub) ParseSearch(SearchInput) (Query, error)                 { return Query{}, nil }
func (s stub) Search(context.Context, Query) ([]model.Listing, error) { return nil, nil }
func (s stub) FetchDescription(context.Context, model.Listing) (string, error) {
	return "", nil
}

func TestRegistryForURL(t *testing.T) {
	r := NewRegistry(
		stub{Info{ID: "a", Name: "A", Domains: []string{"site-a.fr", "site-a.be"}}},
		stub{Info{ID: "b", Name: "B", Domains: []string{"site-b.be"}}},
	)
	cases := map[string]string{
		"https://www.site-a.be/catalog?x=1": "a",
		"site-a.fr/items/1":                 "a",
		"https://shop.site-b.be/fr/search":  "b",
		"https://example.com/":              "",
		"just keywords":                     "",
	}
	for in, want := range cases {
		p, ok := r.ForURL(in)
		got := ""
		if ok {
			got = p.Info().ID
		}
		if got != want {
			t.Errorf("ForURL(%q) = %q, want %q", in, got, want)
		}
	}
}
