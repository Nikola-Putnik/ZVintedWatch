package engine

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/activity"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/store"
)

// enrichingProvider adds a location to matched listings.
type enrichingProvider struct {
	*fakeProvider
	calls map[string]int
}

func (p *enrichingProvider) Enrich(_ context.Context, l *model.Listing) error {
	p.calls[l.ID]++
	l.Location = "Store of " + l.ID
	return nil
}

// panicProvider simulates a broken site module.
type panicProvider struct{}

func (panicProvider) Info() provider.Info {
	return provider.Info{ID: "broken", Name: "Broken", MinIntervalSec: 10}
}
func (panicProvider) ParseSearch(in provider.SearchInput) (provider.Query, error) {
	return provider.Query{Provider: "broken", Params: url.Values{"q": {in.Text}}, Label: in.Text}, nil
}
func (panicProvider) Search(context.Context, provider.Query) ([]model.Listing, error) {
	var m map[string]int
	m["boom"]++ // nil map write: panics
	return nil, nil
}
func (panicProvider) FetchDescription(context.Context, model.Listing) (string, error) { return "", nil }

// newFixtureWith builds a fixture whose registry holds the given providers.
func newFixtureWith(t *testing.T, providers ...provider.Provider) *fixture {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "sites.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	notif := &recordingNotifier{}
	eng := New(st, provider.NewRegistry(providers...), activity.NewBus(200, nil), notif, Options{})
	return &fixture{t: t, ctx: context.Background(), store: st, notif: notif, eng: eng}
}

func (f *fixture) addAlertFor(providerID string, a model.Alert) {
	f.t.Helper()
	a.Provider, a.Enabled = providerID, true
	a.IntervalMinSec, a.IntervalMaxSec = 60, 60
	if _, err := f.store.CreateAlert(f.ctx, a); err != nil {
		f.t.Fatal(err)
	}
	if err := f.eng.Reload(f.ctx); err != nil {
		f.t.Fatal(err)
	}
}

func TestEnrichOnlyMatchedListingsOnce(t *testing.T) {
	fake := &fakeProvider{}
	ep := &enrichingProvider{fakeProvider: fake, calls: map[string]int{}}
	f := newFixtureWith(t, ep)
	f.addAlertFor("fake", model.Alert{Name: "Mac", SearchInput: "mac", Title: model.TextFilter{Include: []string{"mac"}},
		DestinationIDs: []int64{1}})
	f.addAlertFor("fake", model.Alert{Name: "Mini", SearchInput: "mac", Title: model.TextFilter{Include: []string{"mini"}},
		DestinationIDs: []int64{1}})

	fake.results = [][]model.Listing{{}, {listing("1", "Mac mini M4", 499), listing("2", "Dell monitor", 99)}}
	f.poll() // baseline
	f.poll()

	if ep.calls["1"] != 1 {
		t.Fatalf("the matched listing must be enriched exactly once, got %d", ep.calls["1"])
	}
	if ep.calls["2"] != 0 {
		t.Fatal("a listing matching no alert must never be enriched")
	}
	if len(f.notif.sent) != 1 || f.notif.sent[0].Listing.Location != "Store of 1" {
		t.Fatalf("notification must carry the location: %+v", f.notif.sent)
	}
	matches, _ := f.store.ListMatches(f.ctx, store.MatchFilter{})
	for _, m := range matches {
		if m.Listing.Location != "Store of 1" {
			t.Fatalf("match history must be updated with the location: %+v", m.Listing)
		}
	}
}

func TestCrashingModuleDoesNotStopOtherSites(t *testing.T) {
	fake := &fakeProvider{results: [][]model.Listing{{listing("1", "Lego", 1)}}}
	f := newFixtureWith(t, fake, panicProvider{})
	f.addAlertFor("fake", model.Alert{Name: "Lego", SearchInput: "lego"})
	f.addAlertFor("broken", model.Alert{Name: "Broken", SearchInput: "anything"})

	f.poll() // must not panic

	if fake.searchCalls != 1 {
		t.Fatalf("the healthy site must still be polled, got %d calls", fake.searchCalls)
	}
	for _, s := range f.eng.Searches("broken") {
		if s.LastStatus != "error" || !strings.Contains(s.LastError, "Broken module crashed") {
			t.Fatalf("the crash must be reported as an error, got %q / %q", s.LastStatus, s.LastError)
		}
	}
	if _, err := f.eng.Preview(f.ctx, model.Alert{Provider: "broken", SearchInput: "x"}); err == nil {
		t.Fatal("a crashing preview must return an error")
	}
}

func TestPausedSiteSendsNoRequestAndResumesWithBaseline(t *testing.T) {
	fake := &fakeProvider{results: [][]model.Listing{{listing("1", "Lego", 1)}}}
	f := newFixtureWith(t, fake, panicProvider{})
	f.addAlertFor("fake", model.Alert{Name: "Lego", SearchInput: "lego", DestinationIDs: []int64{1}})
	f.poll()
	if !f.eng.Searches("fake")[0].Seeded {
		t.Fatal("search should be seeded")
	}

	if err := f.eng.SetSiteEnabled(f.ctx, "fake", false); err != nil {
		t.Fatal(err)
	}
	if enabled, _ := f.eng.SiteEnabled(f.ctx, "fake"); enabled {
		t.Fatal("site should be paused")
	}
	if n := len(f.eng.Searches("fake")); n != 0 {
		t.Fatalf("a paused site must have no active search, got %d", n)
	}
	calls := fake.searchCalls
	f.poll()
	if fake.searchCalls != calls {
		t.Fatal("a paused site must not send requests")
	}
	if alerts, _ := f.store.ListAlerts(f.ctx, "fake"); len(alerts) != 1 || !alerts[0].Enabled {
		t.Fatal("pausing a site must keep its alerts unchanged")
	}

	if err := f.eng.SetSiteEnabled(f.ctx, "fake", true); err != nil {
		t.Fatal(err)
	}
	searches := f.eng.Searches("fake")
	if len(searches) != 1 || searches[0].Seeded {
		t.Fatalf("a resumed site must start with a fresh baseline: %+v", searches)
	}
	if err := f.eng.SetSiteEnabled(f.ctx, "unknown", false); err == nil {
		t.Fatal("pausing an unknown site must fail")
	}
}

func TestAlertsOfMissingModulesAreKept(t *testing.T) {
	f := newFixtureWith(t, &fakeProvider{})
	if _, err := f.store.CreateAlert(f.ctx, model.Alert{Provider: "removed", Name: "Old", Enabled: true,
		SearchInput: "x", IntervalMinSec: 60, IntervalMaxSec: 60}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := f.eng.Reload(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if alerts, _ := f.store.ListAlerts(f.ctx, "removed"); len(alerts) != 1 {
		t.Fatal("alerts of a missing module must be kept")
	}
	if len(f.eng.warned) != 1 {
		t.Fatalf("a missing module must be reported once, got %v", f.eng.warned)
	}
}
