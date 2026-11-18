package telegram

import (
	"fmt"
	"html"
	"strings"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/engine"
)

// captionLimit is Telegram's maximum caption length for photos.
const captionLimit = 1024

// formatCaption renders the HTML message of a notification.
//
//	<b>God of War Ragnarök PS5</b>
//	<b>25.00 €</b> · Sony · Very good
//	🔔 GoW deals, PS5 games · Vinted
func formatCaption(n engine.Notification) string {
	l := n.Listing
	var b strings.Builder

	b.WriteString("<b>" + esc(truncate(l.Title, 200)) + "</b>\n")

	facts := []string{"<b>" + esc(engine.FormatPrice(l.Price, l.Currency)) + "</b>"}
	for _, f := range []string{l.Brand, l.Size, l.Condition} {
		if f = strings.TrimSpace(f); f != "" {
			facts = append(facts, esc(truncate(f, 60)))
		}
	}
	b.WriteString(strings.Join(facts, " · ") + "\n")

	names := make([]string, 0, len(n.Alerts))
	for _, a := range n.Alerts {
		names = append(names, esc(truncate(a.Name, 60)))
	}
	site := n.Provider.Name
	if site == "" {
		site = l.Provider
	}
	b.WriteString(fmt.Sprintf("🔔 %s · %s", strings.Join(names, ", "), esc(site)))

	out := b.String()
	if len([]rune(out)) > captionLimit {
		out = string([]rune(out)[:captionLimit-1]) + "…"
	}
	return out
}

// openButton is the URL button under each notification.
func openButton(n engine.Notification) []InlineButton {
	if n.Listing.URL == "" {
		return nil
	}
	site := n.Provider.Name
	if site == "" {
		site = "the website"
	}
	return []InlineButton{{Text: "Open on " + site, URL: n.Listing.URL}}
}

func esc(s string) string { return html.EscapeString(s) }

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
