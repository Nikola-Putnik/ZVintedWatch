package store

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func ptr(f float64) *float64 { return &f }

func TestMigrationsAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	for i := 0; i < 2; i++ {
		s, err := Open(path)
		if err != nil {
			t.Fatalf("open #%d: %v", i, err)
		}
		s.Close()
	}
}

func TestAlertCRUD(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	created, err := s.CreateAlert(ctx, model.Alert{
		Provider:    "vinted",
		Name:        "PS5 games",
		Enabled:     true,
		SearchInput: "jeux ps5",
		Domain:      "vinted.fr",
		Title: model.TextFilter{
			Include:     []string{"god of war|gow"},
			IncludeMode: model.MatchAll,
			Exclude:     []string{"boite vide"},
		},
		PriceMax:       ptr(30),
		IntervalMinSec: 30,
		IntervalMaxSec: 120,
		DestinationIDs: []int64{1, 2},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected an id")
	}

	got, err := s.GetAlert(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "PS5 games" || got.PriceMin != nil || got.PriceMax == nil || *got.PriceMax != 30 {
		t.Fatalf("unexpected alert: %+v", got)
	}
	if !reflect.DeepEqual(got.Title.Include, []string{"god of war|gow"}) || got.Description.IncludeMode != model.MatchAll {
		t.Fatalf("filters not round-tripped: %+v", got)
	}
	if !reflect.DeepEqual(got.DestinationIDs, []int64{1, 2}) {
		t.Fatalf("destinations not round-tripped: %v", got.DestinationIDs)
	}

	got.Name = "Renamed"
	got.PriceMax = nil
	if _, err := s.UpdateAlert(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := s.SetAlertEnabled(ctx, got.ID, false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	list, err := s.ListAlerts(ctx, "vinted")
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", list, err)
	}
	if list[0].Name != "Renamed" || list[0].Enabled || list[0].PriceMax != nil {
		t.Fatalf("update not applied: %+v", list[0])
	}

	if err := s.DeleteAlert(ctx, got.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetAlert(ctx, got.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := s.DeleteAlert(ctx, got.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound on second delete, got %v", err)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	type cfg struct {
		Token string
		Gap   int
	}
	var out cfg
	ok, err := s.GetSetting(ctx, "telegram", &out)
	if err != nil || ok {
		t.Fatalf("expected missing key, got ok=%v err=%v", ok, err)
	}
	if err := s.SetSetting(ctx, "telegram", cfg{"abc", 3}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting(ctx, "telegram", cfg{"def", 4}); err != nil {
		t.Fatal(err)
	}
	ok, err = s.GetSetting(ctx, "telegram", &out)
	if err != nil || !ok || out.Token != "def" || out.Gap != 4 {
		t.Fatalf("unexpected value %+v ok=%v err=%v", out, ok, err)
	}
}

func TestSeenItemsAndSearchLifecycle(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	a, err := s.EnsureSearch(ctx, "vinted", "k1", "ps5", "https://example.test/a")
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.EnsureSearch(ctx, "vinted", "k1", "ps5 (renamed)", "https://example.test/a")
	if err != nil || again.ID != a.ID || again.Label != "ps5 (renamed)" {
		t.Fatalf("EnsureSearch should be idempotent: %+v %v", again, err)
	}

	now := time.Now()
	fresh, err := s.MarkSeen(ctx, a.ID, []string{"1", "2", "3"}, now)
	if err != nil || !reflect.DeepEqual(fresh, []string{"1", "2", "3"}) {
		t.Fatalf("first MarkSeen: %v %v", fresh, err)
	}
	fresh, err = s.MarkSeen(ctx, a.ID, []string{"4", "2", "5"}, now)
	if err != nil || !reflect.DeepEqual(fresh, []string{"4", "5"}) {
		t.Fatalf("second MarkSeen: %v %v", fresh, err)
	}

	if err := s.UpdateSearchState(ctx, a.ID, true, 42, now, "ok", ""); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSearch(ctx, a.ID)
	if err != nil || !got.Seeded || got.Watermark != 42 || got.LastStatus != "ok" || got.LastPolledAt.UnixMilli() != now.UnixMilli() {
		t.Fatalf("state not saved: %+v %v", got, err)
	}

	// Deleting a search removes its seen items (ON DELETE CASCADE).
	if _, err := s.EnsureSearch(ctx, "vinted", "k2", "other", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSearchesExcept(ctx, []string{"k2"}); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListSearches(ctx)
	if len(list) != 1 || list[0].Key != "k2" {
		t.Fatalf("unexpected searches: %+v", list)
	}
	b, _ := s.EnsureSearch(ctx, "vinted", "k1", "ps5", "")
	fresh, _ = s.MarkSeen(ctx, b.ID, []string{"1"}, now)
	if len(fresh) != 1 {
		t.Fatalf("seen items should have been deleted with their search, got %v", fresh)
	}
}

func TestMatchesAndDeliveries(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	m := model.Match{
		AlertID:   7,
		AlertName: "Lego",
		Provider:  "vinted",
		Listing:   model.Listing{Provider: "vinted", ID: "42", Title: "Lego 42115", Price: 250, Currency: "EUR"},
		MatchedAt: time.Now(),
		Delivery:  model.DeliveryPending,
	}
	inserted, err := s.InsertMatch(ctx, &m)
	if err != nil || !inserted || m.ID == 0 {
		t.Fatalf("insert: %v %v %d", inserted, err, m.ID)
	}
	dup := m
	inserted, err = s.InsertMatch(ctx, &dup)
	if err != nil || inserted {
		t.Fatalf("duplicate match should be ignored: %v %v", inserted, err)
	}
	if err := s.SetMatchDelivery(ctx, []int64{m.ID}, model.DeliverySent); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListMatches(ctx, MatchFilter{Provider: "vinted"})
	if err != nil || len(list) != 1 || list[0].Delivery != model.DeliverySent || list[0].Listing.Title != "Lego 42115" {
		t.Fatalf("unexpected matches: %+v %v", list, err)
	}
	counts, err := s.CountMatchesSince(ctx, time.Now().Add(-time.Hour))
	if err != nil || counts["vinted"] != 1 {
		t.Fatalf("unexpected counts: %v %v", counts, err)
	}

	ok, err := s.ClaimDelivery(ctx, 1, "vinted", "42")
	if err != nil || !ok {
		t.Fatalf("first claim: %v %v", ok, err)
	}
	ok, _ = s.ClaimDelivery(ctx, 1, "vinted", "42")
	if ok {
		t.Fatal("second claim should fail")
	}
	if err := s.ReleaseDelivery(ctx, 1, "vinted", "42"); err != nil {
		t.Fatal(err)
	}
	ok, _ = s.ClaimDelivery(ctx, 1, "vinted", "42")
	if !ok {
		t.Fatal("claim after release should succeed")
	}
}

func TestPrune(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	se, _ := s.EnsureSearch(ctx, "vinted", "k", "k", "")
	old := time.Now().Add(-48 * time.Hour)
	if _, err := s.MarkSeen(ctx, se.ID, []string{"old"}, old); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MarkSeen(ctx, se.ID, []string{"new"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	res, err := s.Prune(ctx, 24*time.Hour, 24*time.Hour)
	if err != nil || res.SeenItems != 1 {
		t.Fatalf("prune: %+v %v", res, err)
	}
}

func TestDestinations(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	d, err := s.CreateDestination(ctx, model.Destination{Name: "Me", ChatID: "123"})
	if err != nil {
		t.Fatal(err)
	}
	d.Name = "Channel"
	d.ChatID = "@mychannel"
	if err := s.UpdateDestination(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetDestination(ctx, d.ID)
	if err != nil || got.ChatID != "@mychannel" {
		t.Fatalf("unexpected destination: %+v %v", got, err)
	}
	if err := s.DeleteDestination(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListDestinations(ctx)
	if len(list) != 0 {
		t.Fatalf("expected no destinations, got %v", list)
	}
}
