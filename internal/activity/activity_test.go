package activity

import (
	"context"
	"testing"
	"time"
)

func TestRecentKeepsTheLastEventsInOrder(t *testing.T) {
	b := NewBus(3, nil)
	for i := 0; i < 5; i++ {
		b.Publish(Event{Message: string(rune('a' + i))})
	}
	got := b.Recent(0)
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d", len(got))
	}
	want := []string{"c", "d", "e"}
	for i, e := range got {
		if e.Message != want[i] {
			t.Fatalf("event %d: got %q want %q", i, e.Message, want[i])
		}
	}
	if got[2].ID != 5 {
		t.Fatalf("expected id 5, got %d", got[2].ID)
	}
	if last := b.Recent(1); len(last) != 1 || last[0].Message != "e" {
		t.Fatalf("Recent(1) = %+v", last)
	}
}

func TestSubscribeReceivesNewEvents(t *testing.T) {
	b := NewBus(10, nil)
	ch, cancel := b.Subscribe(4)
	defer cancel()

	b.Publish(Event{Message: "hello", Kind: KindSystem})
	select {
	case e := <-ch:
		if e.Message != "hello" || e.Level != LevelInfo || e.Time.IsZero() {
			t.Fatalf("unexpected event %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}

	cancel()
	cancel() // idempotent
	if _, ok := <-ch; ok {
		t.Fatal("channel should be closed after cancel")
	}
	b.Publish(Event{Message: "after cancel"}) // must not panic
}

func TestSlowSubscriberDoesNotBlock(t *testing.T) {
	b := NewBus(10, nil)
	_, cancel := b.Subscribe(1)
	defer cancel()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			b.Publish(Event{Message: "x"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publisher blocked by a slow subscriber")
	}
}

func TestScope(t *testing.T) {
	ctx := WithScope(context.Background(), Scope{Provider: "vinted", SearchID: 3, Search: "ps5"})
	e := Event{Provider: "explicit"}
	ScopeFrom(ctx).Apply(&e)
	if e.Provider != "explicit" || e.SearchID != 3 || e.Search != "ps5" {
		t.Fatalf("unexpected event %+v", e)
	}
	if (ScopeFrom(context.Background()) != Scope{}) {
		t.Fatal("expected empty scope")
	}
}
