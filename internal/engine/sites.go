package engine

import (
	"context"
	"fmt"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/activity"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
)

// This file keeps every site module isolated from the others:
//
//   - a whole site can be paused at runtime without touching its alerts;
//   - a crash (panic) inside a site module is turned into an ordinary error,
//     so a faulty module cannot stop the application or the other sites.

// SiteSettingsKey returns the settings key holding the state of a site.
func SiteSettingsKey(providerID string) string { return "site." + providerID }

// siteSettings is the persisted state of a site. The zero value (no stored
// setting) means the site is active.
type siteSettings struct {
	Paused bool `json:"paused"`
}

// SiteEnabled reports whether a site is active (not paused).
func (e *Engine) SiteEnabled(ctx context.Context, providerID string) (bool, error) {
	var s siteSettings
	if _, err := e.store.GetSetting(ctx, SiteSettingsKey(providerID), &s); err != nil {
		return false, err
	}
	return !s.Paused, nil
}

// SetSiteEnabled pauses or resumes a whole site. Paused sites send no request;
// their alerts are kept unchanged. When a site is resumed, its searches start
// with a fresh baseline, so listings posted during the pause are not sent.
func (e *Engine) SetSiteEnabled(ctx context.Context, providerID string, enabled bool) error {
	p, ok := e.registry.Get(providerID)
	if !ok {
		return fmt.Errorf("%w: unknown site %q", ErrValidation, providerID)
	}
	if err := e.store.SetSetting(ctx, SiteSettingsKey(providerID), siteSettings{Paused: !enabled}); err != nil {
		return err
	}
	state := "resumed"
	if !enabled {
		state = "paused"
	}
	e.system(activity.LevelInfo, providerID, fmt.Sprintf("%s %s", p.Info().Name, state))
	return e.Reload(ctx)
}

// pausedSites returns the ids of the paused sites.
func (e *Engine) pausedSites(ctx context.Context) (map[string]bool, error) {
	paused := map[string]bool{}
	for _, p := range e.registry.All() {
		id := p.Info().ID
		enabled, err := e.SiteEnabled(ctx, id)
		if err != nil {
			return nil, err
		}
		paused[id] = !enabled
	}
	return paused, nil
}

// safeCall runs fn and converts a panic raised by a site module into an
// error that names the module and the operation.
func safeCall(p provider.Provider, op string, fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("the %s module crashed during %s: %v", p.Info().Name, op, r)
		}
	}()
	return fn()
}

// parseSearch calls Provider.ParseSearch with crash protection.
func parseSearch(p provider.Provider, in provider.SearchInput) (q provider.Query, err error) {
	err = safeCall(p, "search parsing", func() error {
		var perr error
		q, perr = p.ParseSearch(in)
		return perr
	})
	return q, err
}

// warnUnknownSite reports, once per site, alerts whose site module is not
// installed.
func (e *Engine) warnUnknownSite(providerID string) {
	e.mu.Lock()
	seen := e.warned[providerID]
	e.warned[providerID] = true
	e.mu.Unlock()
	if !seen {
		e.system(activity.LevelWarn, providerID, fmt.Sprintf(
			"Some alerts belong to the site %q, which is not available in this version: they are kept but not refreshed",
			providerID))
	}
}
