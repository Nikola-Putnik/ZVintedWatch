package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/activity"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/engine"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/store"
)

// fakeBotAPI records calls made to a fake Telegram Bot API.
type fakeBotAPI struct {
	mu         sync.Mutex
	calls      []call
	floodOnce  bool // answer the first send with HTTP 429
	failChatID string
}

type call struct {
	method  string
	chatID  string
	caption string
	photo   bool // multipart upload
}

func (f *fakeBotAPI) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/photo.png", func(w http.ResponseWriter, r *http.Request) {
		img := image.NewRGBA(image.Rect(0, 0, 4, 4))
		img.Set(1, 1, color.RGBA{R: 255, A: 255})
		_ = png.Encode(w, img)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		method := parts[len(parts)-1]
		if !strings.HasPrefix(parts[1], "botTEST") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		c := call{method: method}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("multipart: %v", err)
			}
			c.chatID, c.caption, c.photo = r.FormValue("chat_id"), r.FormValue("caption"), true
		} else {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			c.chatID, _ = body["chat_id"].(string)
			c.caption, _ = body["caption"].(string)
			if txt, ok := body["text"].(string); ok {
				c.caption = txt
			}
		}

		f.mu.Lock()
		flood := f.floodOnce && strings.HasPrefix(method, "send")
		if flood {
			f.floodOnce = false
		} else {
			f.calls = append(f.calls, c)
		}
		fail := c.chatID != "" && c.chatID == f.failChatID
		f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case flood:
			io.WriteString(w, `{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":1}}`)
		case fail:
			io.WriteString(w, `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`)
		case method == "getMe":
			io.WriteString(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Bot","username":"test_bot"}}`)
		case method == "getUpdates":
			io.WriteString(w, `{"ok":true,"result":[
				{"update_id":1,"message":{"chat":{"id":42,"type":"private","first_name":"Alex"}}},
				{"update_id":2,"channel_post":{"chat":{"id":-100123,"type":"channel","title":"Deals"}}},
				{"update_id":3,"message":{"chat":{"id":42,"type":"private","first_name":"Alex"}}}]}`)
		default:
			io.WriteString(w, `{"ok":true,"result":{}}`)
		}
	})
	return mux
}

func (f *fakeBotAPI) sends() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []call
	for _, c := range f.calls {
		if strings.HasPrefix(c.method, "send") {
			out = append(out, c)
		}
	}
	return out
}

func setup(t *testing.T) (*Service, *store.Store, *fakeBotAPI, *httptest.Server) {
	t.Helper()
	api := &fakeBotAPI{}
	srv := httptest.NewServer(api.handler(t))
	t.Cleanup(srv.Close)
	st, err := store.Open(filepath.Join(t.TempDir(), "tg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := NewService(st, activity.NewBus(100, nil), srv.URL)
	t.Cleanup(svc.Close)
	if _, err := svc.SetToken(context.Background(), "TEST:token"); err != nil {
		t.Fatal(err)
	}
	return svc, st, api, srv
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

func TestSetTokenStoresBotUsername(t *testing.T) {
	svc, st, _, _ := setup(t)
	if got := svc.Settings().BotUsername; got != "test_bot" {
		t.Fatalf("bot username = %q", got)
	}
	var saved Settings
	if ok, _ := st.GetSetting(context.Background(), SettingsKey, &saved); !ok || saved.Token != "TEST:token" {
		t.Fatalf("settings not persisted: %+v", saved)
	}
}

func TestNotifyDeliversOncePerChat(t *testing.T) {
	svc, st, api, srv := setup(t)
	ctx := context.Background()
	dest, _ := st.CreateDestination(ctx, model.Destination{Name: "Me", ChatID: "42"})

	l := model.Listing{Provider: "vinted", ID: "7", Title: "Lego <Technic> & co", Price: 99.5, Currency: "EUR",
		Brand: "LEGO", Condition: "New", URL: "https://www.vinted.fr/items/7", PhotoURL: srv.URL + "/photo.png"}
	m := model.Match{AlertID: 1, AlertName: "Lego", Provider: "vinted", Listing: l, MatchedAt: time.Now(), Delivery: model.DeliveryPending}
	if _, err := st.InsertMatch(ctx, &m); err != nil {
		t.Fatal(err)
	}
	n := engine.Notification{
		Provider:       provider.Info{ID: "vinted", Name: "Vinted"},
		Listing:        l,
		Alerts:         []model.Alert{{ID: 1, Name: "Lego"}, {ID: 2, Name: "Technic"}},
		MatchIDs:       []int64{m.ID},
		DestinationIDs: []int64{dest.ID},
	}
	svc.Notify(n)
	svc.Notify(n) // same listing, same chat: must not be sent twice

	waitFor(t, func() bool {
		ms, _ := st.ListMatches(ctx, store.MatchFilter{})
		return len(api.sends()) > 0 && len(ms) == 1 && ms[0].Delivery == model.DeliverySent
	})
	time.Sleep(50 * time.Millisecond) // give a hypothetical duplicate time to show up
	sends := api.sends()
	if len(sends) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sends))
	}
	c := sends[0]
	if !c.photo || c.chatID != "42" {
		t.Fatalf("expected an uploaded photo to chat 42, got %+v", c)
	}
	for _, want := range []string{"<b>Lego &lt;Technic&gt; &amp; co</b>", "99.50 €", "LEGO", "Lego, Technic", "Vinted"} {
		if !strings.Contains(c.caption, want) {
			t.Errorf("caption %q does not contain %q", c.caption, want)
		}
	}
}

func TestNotifyRetriesAfterFloodControl(t *testing.T) {
	svc, st, api, _ := setup(t)
	ctx := context.Background()
	api.floodOnce = true
	dest, _ := st.CreateDestination(ctx, model.Destination{Name: "Channel", ChatID: "@deals"})
	svc.Notify(engine.Notification{
		Provider:       provider.Info{ID: "vinted", Name: "Vinted"},
		Listing:        model.Listing{Provider: "vinted", ID: "8", Title: "Switch", URL: "https://x.test/8"},
		Alerts:         []model.Alert{{Name: "Switch"}},
		DestinationIDs: []int64{dest.ID},
	})
	waitFor(t, func() bool { return len(api.sends()) == 1 })
}

func TestFailedDeliveryReleasesClaim(t *testing.T) {
	svc, st, api, _ := setup(t)
	ctx := context.Background()
	api.failChatID = "404"
	dest, _ := st.CreateDestination(ctx, model.Destination{Name: "Broken", ChatID: "404"})
	l := model.Listing{Provider: "vinted", ID: "9", Title: "Item", URL: "https://x.test/9"}
	m := model.Match{AlertID: 1, AlertName: "A", Provider: "vinted", Listing: l, MatchedAt: time.Now(), Delivery: model.DeliveryPending}
	st.InsertMatch(ctx, &m)
	svc.Notify(engine.Notification{Provider: provider.Info{ID: "vinted"}, Listing: l,
		Alerts: []model.Alert{{Name: "A"}}, MatchIDs: []int64{m.ID}, DestinationIDs: []int64{dest.ID}})

	waitFor(t, func() bool {
		ms, _ := st.ListMatches(ctx, store.MatchFilter{})
		return ms[0].Delivery == model.DeliveryFailed
	})
	if ok, _ := st.ClaimDelivery(ctx, dest.ID, "vinted", "9"); !ok {
		t.Fatal("a failed delivery must release its claim")
	}
}

func TestDiscoverChats(t *testing.T) {
	svc, _, _, _ := setup(t)
	chats, err := svc.DiscoverChats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 2 {
		t.Fatalf("expected 2 unique chats, got %+v", chats)
	}
	if chats[0].Title != "Alex" || chats[0].ID != "42" || chats[1].ID != "-100123" || chats[1].Type != "channel" {
		t.Fatalf("unexpected chats %+v", chats)
	}
}

func TestToJPEG(t *testing.T) {
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	out, err := toJPEG(buf.Bytes())
	if err != nil || len(out) < 2 || out[0] != 0xFF || out[1] != 0xD8 {
		t.Fatalf("expected JPEG output, got %d bytes, %v", len(out), err)
	}
	if _, err := toJPEG([]byte("not an image")); err == nil {
		t.Fatal("expected an error")
	}
}

func TestCaptionIsTruncated(t *testing.T) {
	n := engine.Notification{
		Listing: model.Listing{Title: strings.Repeat("a", 300), Brand: strings.Repeat("b", 500)},
	}
	for i := 0; i < 40; i++ {
		n.Alerts = append(n.Alerts, model.Alert{Name: strings.Repeat("c", 60)})
	}
	if got := len([]rune(formatCaption(n))); got > captionLimit {
		t.Fatalf("caption has %d characters", got)
	}
}
