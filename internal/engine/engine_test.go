package engine

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/activity"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/store"
)

// fakeProvider returns scripted search results and descriptions.
type fakeProvider struct {
	mu          sync.Mutex
	results     [][]model.Listing
	searchCalls int
	descCalls   map[string]int
	descs       map[string]string
	descErr     error
	tolerance   int64
}

func (f *fakeProvider) Info() provider.Info {
	return provider.Info{ID: "fake", Name: "Fake", MinIntervalSec: 10, SequenceTolerance: f.tolerance}
}

func (f *fakeProvider) ParseSearch(in provider.SearchInput) (provider.Query, error) {
	if strings.TrimSpace(in.Text) == "" {
		return provider.Query{}, provider.ErrInvalidSearch
	}
	kw := strings.Join(strings.Fields(strings.ToLower(in.Text)), " ")
	return provider.Query{Provider: "fake", Domain: "fake.test", Params: url.Values{"q": {kw}}, Label: kw}, nil
}

func (f *fakeProvider) Search(context.Context, provider.Query) ([]model.Listing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searchCalls++
	if len(f.results) == 0 {
		return nil, nil
	}
	r := f.results[0]
	if len(f.results) > 1 {
		f.results = f.results[1:]
	}
	return r, nil
}

func (f *fakeProvider) FetchDescription(_ context.Context, l model.Listing) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.descCalls == nil {
		f.descCalls = map[string]int{}
	}
	f.descCalls[l.ID]++
	if f.descErr != nil {
		return "", f.descErr
	}
	return f.descs[l.ID], nil
}

type recordingNotifier struct {
	mu   sync.Mutex
	sent []Notification
}

func (r *recordingNotifier) Notify(n Notification) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, n)
}

type fixture struct {
	t     *testing.T
	ctx   context.Context
	store *store.Store
	fake  *fakeProvider
	notif *recordingNotifier
	eng   *Engine
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "engine.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	fake := &fakeProvider{}
	notif := &recordingNotifier{}
	eng := New(st, provider.NewRegistry(fake), activity.NewBus(200, nil), notif, Options{})
	return &fixture{t: t, ctx: context.Background(), store: st, fake: fake, notif: notif, eng: eng}
}

func (f *fixture) addAlert(a model.Alert) model.Alert {
	f.t.Helper()
	a.Provider = "fake"
	a.Enabled = true
	if a.IntervalMinSec == 0 {
		a.IntervalMinSec, a.IntervalMaxSec = 60, 60
	}
	if err := f.eng.Validate(a); err != nil {
		f.t.Fatalf("validate: %v", err)
	}
	created, err := f.store.CreateAlert(f.ctx, a)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := f.eng.Reload(f.ctx); err != nil {
		f.t.Fatal(err)
	}
	return created
}

// poll runs every job once, synchronously.
func (f *fixture) poll() {
	f.t.Helper()
	f.eng.mu.Lock()
	var jobs []*job
	for _, j := range f.eng.jobs {
		j.running = true
		jobs = append(jobs, j)
	}
	f.eng.mu.Unlock()
	for _, j := range jobs {
		f.eng.runJob(f.ctx, j)
	}
}

func listing(id, title string, price float64) model.Listing {
	return model.Listing{Provider: "fake", ID: id, Title: title, Price: price, Currency: "EUR", URL: "https://fake.test/" + id}
}

func TestAlertsSharingASearchUseOneJob(t *testing.T) {
	f := newFixture(t)
	f.addAlert(model.Alert{Name: "GoW", SearchInput: "Jeux PS5", IntervalMinSec: 30, IntervalMaxSec: 180,
		Title: model.TextFilter{Include: []string{"god of war"}}})
	f.addAlert(model.Alert{Name: "Spider", SearchInput: "jeux   ps5", IntervalMinSec: 60, IntervalMaxSec: 60,
		Title: model.TextFilter{Include: []string{"spider*"}}})
	f.addAlert(model.Alert{Name: "Other", SearchInput: "lego"})

	searches := f.eng.Searches("fake")
	if len(searches) != 2 {
		t.Fatalf("expected 2 searches, got %d", len(searches))
	}
	var ps5 SearchStatus
	for _, s := range searches {
		if s.Label == "jeux ps5" {
			ps5 = s
		}
	}
	if len(ps5.Alerts) != 2 {
		t.Fatalf("expected the ps5 search to serve 2 alerts, got %+v", ps5.Alerts)
	}
	// Merged interval: [min(30, 60), min(180, 60)] = [30, 60]
	if ps5.MinIntervalSec != 30 || ps5.MaxIntervalSec != 60 {
		t.Fatalf("unexpected merged interval %d-%d", ps5.MinIntervalSec, ps5.MaxIntervalSec)
	}

	f.fake.results = [][]model.Listing{{listing("1", "God of War", 20)}}
	f.poll()
	if f.fake.searchCalls != 2 {
		t.Fatalf("expected one request per distinct search (2), got %d", f.fake.searchCalls)
	}
}

func TestSeedThenNotifyNewListings(t *testing.T) {
	f := newFixture(t)
	dest, _ := f.store.CreateDestination(f.ctx, model.Destination{Name: "me", ChatID: "1"})
	maxPrice := 30.0
	gow := f.addAlert(model.Alert{Name: "GoW", SearchInput: "jeux ps5", PriceMax: &maxPrice,
		Title:          model.TextFilter{Include: []string{"god of war|gow"}},
		DestinationIDs: []int64{dest.ID}})
	ragnarok := f.addAlert(model.Alert{Name: "Ragnarok", SearchInput: "jeux ps5",
		Title:          model.TextFilter{Include: []string{"ragnarok"}},
		DestinationIDs: []int64{dest.ID}})

	f.fake.results = [][]model.Listing{
		{listing("1", "God of War Ragnarok", 20)}, // seed: never notified
		{
			listing("3", "God of War Ragnarok PS5", 25), // matches both alerts
			listing("2", "GOW too expensive", 80),       // price filter rejects
			{Provider: "fake", ID: "9", Title: "God of War promo", Price: 10, Promoted: true},
			listing("1", "God of War Ragnarok", 20), // already seen
		},
	}

	f.poll()
	if len(f.notif.sent) != 0 {
		t.Fatalf("seed poll must not notify, got %d notifications", len(f.notif.sent))
	}
	f.poll()
	if len(f.notif.sent) != 1 {
		t.Fatalf("expected exactly one notification, got %d", len(f.notif.sent))
	}
	n := f.notif.sent[0]
	if n.Listing.ID != "3" || len(n.Alerts) != 2 || len(n.DestinationIDs) != 1 || len(n.MatchIDs) != 2 {
		t.Fatalf("unexpected notification %+v", n)
	}
	names := []string{n.Alerts[0].Name, n.Alerts[1].Name}
	if !(contains(names, gow.Name) && contains(names, ragnarok.Name)) {
		t.Fatalf("unexpected alerts %v", names)
	}
	matches, _ := f.store.ListMatches(f.ctx, store.MatchFilter{})
	if len(matches) != 2 {
		t.Fatalf("expected 2 recorded matches, got %d", len(matches))
	}
}

func TestDescriptionFetchedLazilyOncePerListing(t *testing.T) {
	f := newFixture(t)
	f.addAlert(model.Alert{Name: "Sealed", SearchInput: "lego",
		Title:       model.TextFilter{Include: []string{"lego"}},
		Description: model.TextFilter{Include: []string{"sealed|neuf"}}})
	f.addAlert(model.Alert{Name: "Complete", SearchInput: "lego",
		Title:       model.TextFilter{Include: []string{"lego"}},
		Description: model.TextFilter{Exclude: []string{"incomplete"}}})
	f.addAlert(model.Alert{Name: "Technic only", SearchInput: "lego",
		Title:       model.TextFilter{Include: []string{"technic"}},
		Description: model.TextFilter{Include: []string{"sealed"}}})

	f.fake.descs = map[string]string{"10": "Brand new, sealed box", "11": "Box opened"}
	f.fake.results = [][]model.Listing{
		{},
		{listing("10", "Lego City", 50), listing("11", "Lego Star Wars", 40), listing("12", "Playmobil", 10)},
	}
	f.poll()
	f.poll()

	if f.fake.descCalls["12"] != 0 {
		t.Fatal("description of a listing whose title matches no alert must not be fetched")
	}
	if f.fake.descCalls["10"] != 1 || f.fake.descCalls["11"] != 1 {
		t.Fatalf("each description must be fetched exactly once, got %v", f.fake.descCalls)
	}
	byItem := map[string][]string{}
	matches, _ := f.store.ListMatches(f.ctx, store.MatchFilter{})
	for _, m := range matches {
		byItem[m.Listing.ID] = append(byItem[m.Listing.ID], m.AlertName)
	}
	if len(byItem["10"]) != 2 || len(byItem["11"]) != 1 || byItem["11"][0] != "Complete" {
		t.Fatalf("unexpected matches %v", byItem)
	}
	cached, ok, _ := f.store.GetCachedDescription(f.ctx, "fake", "10")
	if !ok || cached != "Brand new, sealed box" {
		t.Fatalf("description not cached: %q %v", cached, ok)
	}
}

func TestDescriptionFailureIsRetriedOnNextPoll(t *testing.T) {
	f := newFixture(t)
	f.addAlert(model.Alert{Name: "Sealed", SearchInput: "lego",
		Description: model.TextFilter{Include: []string{"sealed"}}})
	f.fake.results = [][]model.Listing{{}, {listing("10", "Lego", 50)}, {listing("10", "Lego", 50)}}
	f.fake.descErr = errors.New("temporary failure")
	f.fake.descs = map[string]string{"10": "sealed"}

	f.poll() // seed
	f.poll() // description fails → kept pending
	if n := len(f.eng.Searches("")[0:1]); n != 1 || f.eng.Searches("")[0].PendingItems != 1 {
		t.Fatalf("expected one pending item, got %+v", f.eng.Searches(""))
	}
	f.fake.descErr = nil
	f.poll() // retried even though the listing is no longer new
	matches, _ := f.store.ListMatches(f.ctx, store.MatchFilter{})
	if len(matches) != 1 {
		t.Fatalf("expected the pending listing to match after retry, got %d matches", len(matches))
	}
}

func TestListingGoneIsNotRetried(t *testing.T) {
	f := newFixture(t)
	f.addAlert(model.Alert{Name: "Sealed", SearchInput: "lego",
		Description: model.TextFilter{Include: []string{"sealed"}}})
	f.fake.results = [][]model.Listing{{}, {listing("10", "Lego", 50)}}
	f.fake.descErr = provider.ErrListingUnavailable
	f.poll()
	f.poll()
	if p := f.eng.Searches("")[0].PendingItems; p != 0 {
		t.Fatalf("expected no pending item, got %d", p)
	}
}

func TestBumpedListingsAreIgnored(t *testing.T) {
	f := newFixture(t)
	f.fake.tolerance = 100
	f.addAlert(model.Alert{Name: "All", SearchInput: "lego", DestinationIDs: []int64{1}})
	seq := func(l model.Listing, s int64) model.Listing { l.Sequence = s; return l }
	f.fake.results = [][]model.Listing{
		{seq(listing("1000", "Lego", 1), 1000)},
		{seq(listing("1990", "Lego new", 1), 1990), seq(listing("5", "Lego bumped", 1), 5), seq(listing("950", "Lego slightly late", 1), 950)},
	}
	f.poll()
	f.poll()
	var got []string
	for _, n := range f.notif.sent {
		got = append(got, n.Listing.ID)
	}
	if strings.Join(got, ",") != "950,1990" {
		t.Fatalf("expected listings 950 and 1990 to be notified, got %v", got)
	}
	searches, _ := f.store.ListSearches(f.ctx)
	if searches[0].Watermark != 1990 {
		t.Fatalf("watermark not persisted: %d", searches[0].Watermark)
	}
}

func TestReseedAfterLongPause(t *testing.T) {
	f := newFixture(t)
	f.addAlert(model.Alert{Name: "All", SearchInput: "lego", DestinationIDs: []int64{1}})
	f.fake.results = [][]model.Listing{{listing("1", "Lego", 1)}, {listing("2", "Lego 2", 1)}}
	f.poll() // seed

	// Simulate a long downtime.
	f.eng.mu.Lock()
	for _, j := range f.eng.jobs {
		j.lastSuccess = time.Now().Add(-2 * time.Hour)
	}
	f.eng.mu.Unlock()

	f.poll()
	if len(f.notif.sent) != 0 {
		t.Fatal("listings accumulated during a long pause must not be notified")
	}
}

func TestReloadKeepsStateAndDropsUnusedSearches(t *testing.T) {
	f := newFixture(t)
	a := f.addAlert(model.Alert{Name: "A", SearchInput: "lego"})
	f.poll()
	if !f.eng.Searches("")[0].Seeded {
		t.Fatal("search should be seeded")
	}
	if err := f.store.SetAlertEnabled(f.ctx, a.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := f.eng.Reload(f.ctx); err != nil {
		t.Fatal(err)
	}
	if n := len(f.eng.Searches("")); n != 0 {
		t.Fatalf("expected no job, got %d", n)
	}
	if list, _ := f.store.ListSearches(f.ctx); len(list) != 0 {
		t.Fatalf("unused searches must be deleted, got %d", len(list))
	}
}

func TestPreviewReusesRecentResults(t *testing.T) {
	f := newFixture(t)
	f.addAlert(model.Alert{Name: "A", SearchInput: "lego"})
	f.fake.results = [][]model.Listing{{listing("1", "Lego Technic", 100), listing("2", "Lego City", 10)}}
	f.poll()
	calls := f.fake.searchCalls

	maxPrice := 50.0
	res, err := f.eng.Preview(f.ctx, model.Alert{Provider: "fake", SearchInput: "LEGO", PriceMax: &maxPrice,
		Title: model.TextFilter{Include: []string{"lego"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Cached || f.fake.searchCalls != calls {
		t.Fatal("preview of a polled search should reuse its results")
	}
	if res.Matches != 1 || len(res.Items) != 2 || res.Items[0].PriceOK {
		t.Fatalf("unexpected preview %+v", res)
	}

	res, err = f.eng.Preview(f.ctx, model.Alert{Provider: "fake", SearchInput: "playmobil"})
	if err != nil || res.Cached || f.fake.searchCalls != calls+1 {
		t.Fatalf("preview of a new search should send one request: %+v %v", res, err)
	}
}

func TestValidate(t *testing.T) {
	f := newFixture(t)
	bad := []model.Alert{
		{Provider: "fake", Name: "", SearchInput: "x", IntervalMinSec: 60, IntervalMaxSec: 60},
		{Provider: "nope", Name: "n", SearchInput: "x", IntervalMinSec: 60, IntervalMaxSec: 60},
		{Provider: "fake", Name: "n", SearchInput: "", IntervalMinSec: 60, IntervalMaxSec: 60},
		{Provider: "fake", Name: "n", SearchInput: "x", IntervalMinSec: 5, IntervalMaxSec: 60},
		{Provider: "fake", Name: "n", SearchInput: "x", IntervalMinSec: 60, IntervalMaxSec: 30},
		{Provider: "fake", Name: "n", SearchInput: "x", IntervalMinSec: 60, IntervalMaxSec: 60,
			Title: model.TextFilter{Include: []string{"/[/"}}},
	}
	for i, a := range bad {
		if err := f.eng.Validate(a); !errors.Is(err, ErrValidation) {
			t.Errorf("case %d: expected a validation error, got %v", i, err)
		}
	}
}

func TestMergeIntervals(t *testing.T) {
	mk := func(lo, hi int) *compiledAlert {
		return &compiledAlert{alert: model.Alert{IntervalMinSec: lo, IntervalMaxSec: hi}}
	}
	lo, hi := mergeIntervals([]*compiledAlert{mk(30, 180), mk(60, 60)}, 10)
	if lo != 30*time.Second || hi != 60*time.Second {
		t.Fatalf("got %v-%v", lo, hi)
	}
	lo, hi = mergeIntervals([]*compiledAlert{mk(5, 8)}, 10)
	if lo != 10*time.Second || hi != 10*time.Second {
		t.Fatalf("floor not applied: %v-%v", lo, hi)
	}
}

func TestFormatPrice(t *testing.T) {
	for in, want := range map[[2]string]string{
		{"12.5", "EUR"}: "12.50 €",
		{"3", "GBP"}:    "£3.00",
		{"0", "EUR"}:    "price n/a",
	} {
		var p float64
		switch in[0] {
		case "12.5":
			p = 12.5
		case "3":
			p = 3
		}
		if got := FormatPrice(p, in[1]); got != want {
			t.Errorf("FormatPrice(%v, %s) = %q, want %q", p, in[1], got, want)
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
