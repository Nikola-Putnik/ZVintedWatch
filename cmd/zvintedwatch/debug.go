package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/activity"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/engine"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/httpx"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider/vinted"
)

// debugCmd runs a provider operation outside the application. It is meant
// for checking that a marketplace module still works after the site changed.
// It uses the default network settings and does not touch the database.
func debugCmd(args []string) error {
	if len(args) == 0 {
		return errors.New(`usage: zvintedwatch debug search <url|keywords> [-domain vinted.fr] | debug item <listing url>`)
	}
	sub, args := args[0], args[1:]

	fs := flag.NewFlagSet("debug "+sub, flag.ExitOnError)
	providerID := fs.String("provider", "vinted", "site module to use")
	domain := fs.String("domain", "", "country domain for keyword searches (e.g. vinted.fr)")
	proxy := fs.String("proxy", "", "optional proxy URL (http://, socks5://)")
	_ = fs.Parse(reorder(args))
	if fs.NArg() == 0 {
		return errors.New("missing argument")
	}
	input := strings.Join(fs.Args(), " ")

	bus := activity.NewBus(100, nil)
	events, cancel := bus.Subscribe(100)
	defer cancel()
	go func() {
		for e := range events {
			if e.HTTP != nil {
				fmt.Printf("  · %s %s → %d (%d ms, %d bytes)\n", e.HTTP.Method, e.HTTP.URL, e.HTTP.Status, e.HTTP.DurationMs, e.HTTP.Bytes)
			} else {
				fmt.Printf("  · %s\n", e.Message)
			}
		}
	}()

	fetcher := httpx.New(bus, nil)
	defer fetcher.Close()
	settings := httpx.DefaultSettings()
	if *proxy != "" {
		settings.Proxies = []string{*proxy}
	}
	registry := provider.NewRegistry(vinted.New(fetcher))
	p, ok := registry.Get(*providerID)
	if !ok {
		return fmt.Errorf("unknown provider %q", *providerID)
	}
	if err := fetcher.Configure(*providerID, settings); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	switch sub {
	case "search":
		q, err := p.ParseSearch(provider.SearchInput{Text: input, Domain: *domain})
		if err != nil {
			return err
		}
		fmt.Printf("Search: %s\nAPI parameters: %s\nKey: %s\n\n", q.Label, q.Params.Encode(), q.Key())
		listings, err := p.Search(ctx, q)
		if err != nil {
			return err
		}
		fmt.Println()
		for _, l := range listings {
			flag := " "
			if l.Promoted {
				flag = "P"
			}
			extra := strings.Join(nonEmpty(l.Brand, l.Size, l.Condition), " · ")
			fmt.Printf("%s %-12s %-10s %s  [%s]\n", flag, l.ID, engine.FormatPrice(l.Price, l.Currency), l.Title, extra)
		}
		fmt.Printf("\n%d listings (P = promoted, ignored by alerts)\n", len(listings))
	case "item":
		desc, err := p.FetchDescription(ctx, model.Listing{URL: input})
		if err != nil {
			return err
		}
		fmt.Printf("\n%s\n", desc)
	default:
		return fmt.Errorf("unknown debug command %q (use search or item)", sub)
	}
	return nil
}

// reorder moves flags before positional arguments so that both
// "debug search ps5 -domain vinted.fr" and "debug search -domain vinted.fr ps5" work.
func reorder(args []string) []string {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			if !strings.Contains(a, "=") && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		rest = append(rest, a)
	}
	return append(flags, rest...)
}

func nonEmpty(values ...string) []string {
	var out []string
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
