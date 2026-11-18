package telegram

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // register PNG decoding for listing photos
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/webp"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/activity"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/engine"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/store"
)

// SettingsKey is the settings entry holding the Telegram configuration.
const SettingsKey = "telegram"

// Settings is the persisted Telegram configuration.
type Settings struct {
	Token       string `json:"token"`
	BotUsername string `json:"botUsername"`
	// SystemDestinationID optionally receives system notices (site blocked,
	// cooldowns). 0 disables them.
	SystemDestinationID int64 `json:"systemDestinationId"`
}

// ErrNotConfigured is returned when no bot token is set.
var ErrNotConfigured = errors.New("telegram bot token is not configured")

// Delays between two messages to the same chat, below Telegram's limits
// (about 1 message per second per chat, 20 per minute in groups/channels).
const (
	privateChatGap = 1100 * time.Millisecond
	groupChatGap   = 3100 * time.Millisecond
	maxAttempts    = 4
)

// Service delivers notifications. It implements engine.Notifier.
type Service struct {
	store   *store.Store
	bus     *activity.Bus
	baseURL string
	images  *http.Client

	mu       sync.Mutex
	settings Settings
	client   *Client
	queues   map[string]chan *delivery
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// delivery is one message to one chat.
type delivery struct {
	chatID   string
	destName string
	destID   int64
	n        engine.Notification
	batch    *batch
	text     string // plain system notice (no listing) when set
}

// batch tracks the destinations of one notification to set the final
// delivery state of its matches once every destination was attempted.
type batch struct {
	mu        sync.Mutex
	remaining int
	sent      bool
}

// NewService creates the service. baseURL may be empty (public Bot API).
func NewService(st *store.Store, bus *activity.Bus, baseURL string) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		store:   st,
		bus:     bus,
		baseURL: baseURL,
		images:  &http.Client{Timeout: 20 * time.Second},
		queues:  map[string]chan *delivery{},
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Load reads the persisted settings.
func (s *Service) Load(ctx context.Context) error {
	var cfg Settings
	if _, err := s.store.GetSetting(ctx, SettingsKey, &cfg); err != nil {
		return err
	}
	s.apply(cfg)
	return nil
}

func (s *Service) apply(cfg Settings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = cfg
	s.client = nil
	if cfg.Token != "" {
		s.client = NewClient(cfg.Token, s.baseURL)
	}
}

// Settings returns the current configuration.
func (s *Service) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

// SetToken validates a bot token with getMe and saves it. An empty token
// disables Telegram.
func (s *Service) SetToken(ctx context.Context, token string) (Settings, error) {
	cfg := s.Settings()
	token = strings.TrimSpace(token)
	cfg.Token, cfg.BotUsername = token, ""
	if token != "" {
		me, err := NewClient(token, s.baseURL).GetMe(ctx)
		if err != nil {
			return cfg, fmt.Errorf("the bot token was rejected: %w", err)
		}
		cfg.BotUsername = me.Username
	}
	if err := s.store.SetSetting(ctx, SettingsKey, cfg); err != nil {
		return cfg, err
	}
	s.apply(cfg)
	return cfg, nil
}

// SetSystemDestination selects the destination receiving system notices.
func (s *Service) SetSystemDestination(ctx context.Context, id int64) error {
	cfg := s.Settings()
	cfg.SystemDestinationID = id
	if err := s.store.SetSetting(ctx, SettingsKey, cfg); err != nil {
		return err
	}
	s.apply(cfg)
	return nil
}

func (s *Service) currentClient() (*Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == nil {
		return nil, ErrNotConfigured
	}
	return s.client, nil
}

// ChatInfo is a chat discovered through getUpdates.
type ChatInfo struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
}

// DiscoverChats lists the chats that recently interacted with the bot: people
// who sent it a message (e.g. /start) and groups/channels it was added to.
func (s *Service) DiscoverChats(ctx context.Context) ([]ChatInfo, error) {
	c, err := s.currentClient()
	if err != nil {
		return nil, err
	}
	updates, err := c.GetUpdates(ctx)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Code == 409 {
			return nil, errors.New("this bot uses a webhook; remove it (deleteWebhook) to discover chats")
		}
		return nil, err
	}
	seen := map[int64]bool{}
	out := []ChatInfo{}
	for _, u := range updates {
		for _, h := range []*chatHolder{u.Message, u.ChannelPost, u.MyChatMember} {
			if h == nil || h.Chat.ID == 0 || seen[h.Chat.ID] {
				continue
			}
			seen[h.Chat.ID] = true
			out = append(out, ChatInfo{ID: strconv.FormatInt(h.Chat.ID, 10), Type: h.Chat.Type, Title: chatTitle(h.Chat)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out, nil
}

func chatTitle(c Chat) string {
	switch {
	case c.Title != "":
		return c.Title
	case c.FirstName != "" || c.LastName != "":
		return strings.TrimSpace(c.FirstName + " " + c.LastName)
	case c.Username != "":
		return "@" + c.Username
	}
	return strconv.FormatInt(c.ID, 10)
}

// SendTest sends a test message to a chat, synchronously.
func (s *Service) SendTest(ctx context.Context, chatID string) error {
	c, err := s.currentClient()
	if err != nil {
		return err
	}
	return c.SendMessage(ctx, chatID,
		"✅ <b>ZVintedWatch is connected.</b>\nMatching listings for your alerts will be posted in this chat.", nil)
}

// Notify implements engine.Notifier: it enqueues one message per destination,
// skipping destinations that already received the listing.
func (s *Service) Notify(n engine.Notification) {
	ctx := s.ctx
	if _, err := s.currentClient(); err != nil {
		s.publish(activity.LevelWarn, n.Provider.ID, "Match not sent: "+err.Error())
		_ = s.store.SetMatchDelivery(ctx, n.MatchIDs, model.DeliveryFailed)
		return
	}

	var jobs []*delivery
	b := &batch{}
	for _, id := range n.DestinationIDs {
		d, err := s.store.GetDestination(ctx, id)
		if err != nil {
			continue // destination deleted
		}
		claimed, err := s.store.ClaimDelivery(ctx, d.ID, n.Listing.Provider, n.Listing.ID)
		if err != nil || !claimed {
			continue // already sent to this chat (another alert or search)
		}
		jobs = append(jobs, &delivery{chatID: d.ChatID, destName: d.Name, destID: d.ID, n: n, batch: b})
	}
	if len(jobs) == 0 {
		_ = s.store.SetMatchDelivery(ctx, n.MatchIDs, model.DeliverySent)
		return
	}
	b.remaining = len(jobs)
	for _, j := range jobs {
		s.enqueue(j)
	}
}

// SystemNotice sends a plain message to the system destination, if any.
func (s *Service) SystemNotice(text string) {
	cfg := s.Settings()
	if cfg.SystemDestinationID == 0 || cfg.Token == "" {
		return
	}
	d, err := s.store.GetDestination(s.ctx, cfg.SystemDestinationID)
	if err != nil {
		return
	}
	s.enqueue(&delivery{chatID: d.ChatID, destName: d.Name, destID: d.ID, text: "⚠️ " + esc(text)})
}

func (s *Service) enqueue(d *delivery) {
	s.mu.Lock()
	q := s.queues[d.chatID]
	if q == nil {
		q = make(chan *delivery, 500)
		s.queues[d.chatID] = q
		s.wg.Add(1)
		go s.worker(d.chatID, q)
	}
	s.mu.Unlock()

	select {
	case q <- d:
	default:
		s.publish(activity.LevelError, d.n.Provider.ID, "Telegram queue full for "+d.destName+"; message dropped")
		s.finish(d, errors.New("queue full"))
	}
}

// worker sends the messages of one chat sequentially, respecting the
// per-chat rate limit.
func (s *Service) worker(chatID string, q chan *delivery) {
	defer s.wg.Done()
	gap := privateChatGap
	if strings.HasPrefix(chatID, "-") || strings.HasPrefix(chatID, "@") {
		gap = groupChatGap
	}
	var last time.Time
	for {
		select {
		case <-s.ctx.Done():
			return
		case d := <-q:
			if wait := time.Until(last.Add(gap)); wait > 0 {
				if !sleep(s.ctx, wait) {
					return
				}
			}
			err := s.send(d)
			last = time.Now()
			s.finish(d, err)
		}
	}
}

// send delivers a message, retrying on flood control and transient errors.
func (s *Service) send(d *delivery) error {
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		c, cerr := s.currentClient()
		if cerr != nil {
			return cerr
		}
		ctx, cancel := context.WithTimeout(s.ctx, 60*time.Second)
		if d.text != "" {
			err = c.SendMessage(ctx, d.chatID, d.text, nil)
		} else {
			err = s.sendListing(ctx, c, d)
		}
		cancel()
		if err == nil {
			return nil
		}
		var apiErr *APIError
		switch {
		case errors.As(err, &apiErr) && apiErr.RetryAfter > 0:
			if !sleep(s.ctx, time.Duration(apiErr.RetryAfter)*time.Second+time.Second) {
				return err
			}
		case errors.As(err, &apiErr) && apiErr.Code >= 400 && apiErr.Code < 500:
			return err // permanent: chat not found, bot blocked, bad request...
		default:
			if !sleep(s.ctx, time.Duration(attempt*attempt)*2*time.Second) {
				return err
			}
		}
	}
	return err
}

// sendListing sends the listing photo with its caption. The photo is
// downloaded and re-encoded as JPEG because marketplaces often serve WebP,
// which Telegram does not display everywhere. Fallbacks: let Telegram fetch
// the photo URL itself, then a text message.
func (s *Service) sendListing(ctx context.Context, c *Client, d *delivery) error {
	caption := formatCaption(d.n)
	buttons := openButton(d.n)
	if photo := d.n.Listing.PhotoURL; photo != "" {
		if jpg, err := s.fetchJPEG(ctx, photo); err == nil {
			if err := c.SendPhotoFile(ctx, d.chatID, jpg, caption, buttons); err == nil || isFatal(err) {
				return err
			}
		}
		if err := c.SendPhotoURL(ctx, d.chatID, photo, caption, buttons); err == nil || isFatal(err) {
			return err
		}
	}
	return c.SendMessage(ctx, d.chatID, caption, buttons)
}

// isFatal reports errors that no fallback can fix (wrong chat, bot blocked, flood control).
func isFatal(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.RetryAfter > 0 || apiErr.Code == 401 || apiErr.Code == 403 {
		return true
	}
	d := strings.ToLower(apiErr.Description)
	return strings.Contains(d, "chat not found") || strings.Contains(d, "not enough rights")
}

func (s *Service) fetchJPEG(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")
	resp, err := s.images.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("photo download failed (HTTP %d)", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	return toJPEG(raw)
}

// toJPEG converts a WebP, PNG or JPEG image to JPEG.
func toJPEG(raw []byte) ([]byte, error) {
	var img image.Image
	var err error
	if len(raw) > 12 && string(raw[0:4]) == "RIFF" && string(raw[8:12]) == "WEBP" {
		img, err = webp.Decode(bytes.NewReader(raw))
	} else {
		img, _, err = image.Decode(bytes.NewReader(raw))
	}
	if err != nil {
		return nil, fmt.Errorf("decode photo: %w", err)
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 88}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// finish records the outcome of a delivery.
func (s *Service) finish(d *delivery, err error) {
	ctx := context.WithoutCancel(s.ctx)
	if d.text != "" { // system notice
		if err != nil {
			s.publish(activity.LevelWarn, "", "System notice to "+d.destName+" failed: "+err.Error())
		}
		return
	}
	if err != nil {
		_ = s.store.ReleaseDelivery(ctx, d.destID, d.n.Listing.Provider, d.n.Listing.ID)
		s.publish(activity.LevelError, d.n.Provider.ID, fmt.Sprintf("Telegram message to %s failed: %v", d.destName, err))
	} else {
		s.publish(activity.LevelInfo, d.n.Provider.ID, fmt.Sprintf("Sent to %s: %s", d.destName, d.n.Listing.Title))
	}

	b := d.batch
	b.mu.Lock()
	b.remaining--
	b.sent = b.sent || err == nil
	done, sent := b.remaining == 0, b.sent
	b.mu.Unlock()
	if done {
		state := model.DeliveryFailed
		if sent {
			state = model.DeliverySent
		}
		_ = s.store.SetMatchDelivery(ctx, d.n.MatchIDs, state)
	}
}

func (s *Service) publish(level activity.Level, providerID, msg string) {
	if s.bus != nil {
		s.bus.Publish(activity.Event{Level: level, Kind: activity.KindNotify, Provider: providerID, Message: msg})
	}
}

// Close stops the workers. Queued messages are dropped.
func (s *Service) Close() {
	s.cancel()
	s.wg.Wait()
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

var _ engine.Notifier = (*Service)(nil)
