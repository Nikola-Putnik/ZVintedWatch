// Package engine schedules searches and turns new listings into matches.
//
// Request economy is the central design goal:
//
//   - Alerts are grouped by their normalized search (provider.Query.Key). Each
//     group is one job, polled with a single request no matter how many alerts
//     it serves. Price ranges and keyword rules are evaluated locally.
//   - A job's refresh delay is drawn at random in [min(alert minimums),
//     min(alert maximums)]: the pace of its most demanding alert.
//   - Listing descriptions are fetched only when a listing's title (and price)
//     already satisfies an alert that has description rules, at most once per
//     listing (cached and shared by every alert).
//   - The first poll of a search only records the current results ("seeding")
//     so enabling an alert never floods the chat with old listings. A search
//     that has not been polled for a long time (e.g. the app was stopped) is
//     seeded again for the same reason.
package engine

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sort"
	"sync"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/activity"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/httpx"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/matcher"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/store"
)

// Notification is handed to the Notifier when a new listing matches alerts.
type Notification struct {
	Provider provider.Info
	Listing  model.Listing
	// Alerts are the alerts the listing matched (at least one).
	Alerts []model.Alert
	// MatchIDs are the ids of the recorded matches, for delivery tracking.
	MatchIDs []int64
	// DestinationIDs is the union of the alerts' destinations.
	DestinationIDs []int64
}

// Notifier delivers notifications. Notify must not block for long; the
// Telegram implementation enqueues messages.
type Notifier interface {
	Notify(n Notification)
}

// Options tune the engine. Zero values select the defaults.
type Options struct {
	// ReseedAfter is how long a search may stay unpolled before its next poll
	// is treated as a seed (default 30 minutes).
	ReseedAfter time.Duration
	// MaxDescriptionAttempts bounds how many polls retry a listing whose
	// description could not be fetched (default 3).
	MaxDescriptionAttempts int
	// PreviewCacheTTL is how long the last results of a job can be reused to
	// answer an alert preview without a new request (default 2 minutes).
	PreviewCacheTTL time.Duration
}

func (o Options) withDefaults() Options {
	if o.ReseedAfter <= 0 {
		o.ReseedAfter = 30 * time.Minute
	}
	if o.MaxDescriptionAttempts <= 0 {
		o.MaxDescriptionAttempts = 3
	}
	if o.PreviewCacheTTL <= 0 {
		o.PreviewCacheTTL = 2 * time.Minute
	}
	return o
}

// Engine runs the polling jobs. Create it with New, call Reload after every
// alert change and run it with Run.
type Engine struct {
	store    *store.Store
	registry *provider.Registry
	bus      *activity.Bus
	notifier Notifier
	opts     Options

	mu   sync.Mutex
	jobs map[string]*job
	wake chan struct{}
}

// job polls one search on behalf of one or more alerts. Fields are guarded
// by Engine.mu.
type job struct {
	key      string
	provider provider.Provider
	query    provider.Query
	searchID int64
	alerts   []*compiledAlert
	minDelay time.Duration
	maxDelay time.Duration

	seeded      bool
	lastSuccess time.Time
	nextRun     time.Time
	running     bool
	lastStatus  string
	lastError   string
	lastResults []model.Listing
	lastNew     int
	pending     []pendingItem
}

type compiledAlert struct {
	alert model.Alert
	title *matcher.Filter
	desc  *matcher.Filter
}

// pendingItem is a new listing whose description could not be fetched yet.
type pendingItem struct {
	listing  model.Listing
	attempts int
}

// New creates an engine. notifier may be nil (matches are then only recorded).
func New(st *store.Store, reg *provider.Registry, bus *activity.Bus, notifier Notifier, opts Options) *Engine {
	return &Engine{
		store:    st,
		registry: reg,
		bus:      bus,
		notifier: notifier,
		opts:     opts.withDefaults(),
		jobs:     map[string]*job{},
		wake:     make(chan struct{}, 1),
	}
}

func (e *Engine) signal() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

// compileAlert compiles the keyword rules of an alert.
func compileAlert(a model.Alert) (*compiledAlert, error) {
	title, err := matcher.Compile(a.Title)
	if err != nil {
		return nil, fmt.Errorf("title rules: %w", err)
	}
	desc, err := matcher.Compile(a.Description)
	if err != nil {
		return nil, fmt.Errorf("description rules: %w", err)
	}
	return &compiledAlert{alert: a, title: title, desc: desc}, nil
}

// Reload rebuilds the jobs from the enabled alerts. Jobs whose search did not
// change keep their schedule and state.
func (e *Engine) Reload(ctx context.Context) error {
	alerts, err := e.store.ListAlerts(ctx, "")
	if err != nil {
		return err
	}

	type group struct {
		p      provider.Provider
		q      provider.Query
		alerts []*compiledAlert
	}
	groups := map[string]*group{}
	for _, a := range alerts {
		if !a.Enabled {
			continue
		}
		p, ok := e.registry.Get(a.Provider)
		if !ok {
			e.system(activity.LevelWarn, a.Provider, fmt.Sprintf("Alert %q uses an unknown site %q", a.Name, a.Provider))
			continue
		}
		q, err := p.ParseSearch(provider.SearchInput{Text: a.SearchInput, Domain: a.Domain})
		if err != nil {
			e.system(activity.LevelWarn, a.Provider, fmt.Sprintf("Alert %q skipped: %v", a.Name, err))
			continue
		}
		ca, err := compileAlert(a)
		if err != nil {
			e.system(activity.LevelWarn, a.Provider, fmt.Sprintf("Alert %q skipped: %v", a.Name, err))
			continue
		}
		g := groups[q.Key()]
		if g == nil {
			g = &group{p: p, q: q}
			groups[q.Key()] = g
		}
		g.alerts = append(g.alerts, ca)
	}

	// Persist the searches (creating rows for new ones) before touching jobs.
	searches := map[string]model.Search{}
	keys := make([]string, 0, len(groups))
	for key, g := range groups {
		s, err := e.store.EnsureSearch(ctx, g.p.Info().ID, key, g.q.Label, g.q.WebURL)
		if err != nil {
			return err
		}
		searches[key] = s
		keys = append(keys, key)
	}
	if err := e.store.DeleteSearchesExcept(ctx, keys); err != nil {
		return err
	}

	now := time.Now()
	e.mu.Lock()
	next := map[string]*job{}
	for key, g := range groups {
		minDelay, maxDelay := mergeIntervals(g.alerts, g.p.Info().MinIntervalSec)
		j := e.jobs[key]
		if j == nil {
			s := searches[key]
			j = &job{
				key:         key,
				seeded:      s.Seeded,
				lastSuccess: s.LastPolledAt,
				lastStatus:  s.LastStatus,
				lastError:   s.LastError,
				// Spread the first polls a little; lanes pace them anyway.
				nextRun: now.Add(time.Duration(rand.Int64N(int64(3 * time.Second)))),
			}
		}
		j.provider, j.query, j.searchID = g.p, g.q, searches[key].ID
		j.alerts = g.alerts
		j.minDelay, j.maxDelay = minDelay, maxDelay
		// A shorter interval takes effect immediately.
		if limit := now.Add(maxDelay); j.nextRun.After(limit) {
			j.nextRun = limit
		}
		next[key] = j
	}
	e.jobs = next
	e.mu.Unlock()

	e.signal()
	return nil
}

// mergeIntervals returns the delay range of a job: [min of minimums, min of
// maximums], clamped to the provider's minimum interval.
func mergeIntervals(alerts []*compiledAlert, floorSec int) (time.Duration, time.Duration) {
	minSec, maxSec := 0, 0
	for i, a := range alerts {
		lo, hi := a.alert.IntervalMinSec, a.alert.IntervalMaxSec
		if hi < lo {
			hi = lo
		}
		if i == 0 || lo < minSec {
			minSec = lo
		}
		if i == 0 || hi < maxSec {
			maxSec = hi
		}
	}
	if minSec < floorSec {
		minSec = floorSec
	}
	if maxSec < minSec {
		maxSec = minSec
	}
	return time.Duration(minSec) * time.Second, time.Duration(maxSec) * time.Second
}

func randomDelay(lo, hi time.Duration) time.Duration {
	if hi <= lo {
		return lo
	}
	return lo + time.Duration(rand.Int64N(int64(hi-lo)+1))
}

// Run executes due jobs until ctx is cancelled. It also prunes old
// housekeeping data every hour.
func (e *Engine) Run(ctx context.Context) {
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	housekeeping := time.NewTicker(time.Hour)
	defer housekeeping.Stop()
	e.prune(ctx)

	for {
		now := time.Now()
		var due []*job
		var next time.Time
		e.mu.Lock()
		for _, j := range e.jobs {
			if j.running {
				continue
			}
			if !j.nextRun.After(now) {
				j.running = true
				due = append(due, j)
			} else if next.IsZero() || j.nextRun.Before(next) {
				next = j.nextRun
			}
		}
		e.mu.Unlock()

		for _, j := range due {
			go e.runJob(ctx, j)
		}

		wait := time.Hour
		if !next.IsZero() {
			wait = time.Until(next)
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(wait)

		select {
		case <-ctx.Done():
			return
		case <-e.wake:
		case <-timer.C:
		case <-housekeeping.C:
			e.prune(ctx)
		}
	}
}

func (e *Engine) prune(ctx context.Context) {
	res, err := e.store.Prune(ctx, 14*24*time.Hour, 90*24*time.Hour)
	if err != nil {
		e.system(activity.LevelError, "", "Housekeeping failed: "+err.Error())
		return
	}
	if total := res.SeenItems + res.CachedItems + res.Matches + res.Deliveries; total > 0 {
		e.system(activity.LevelDebug, "", fmt.Sprintf("Housekeeping removed %d old records", total))
	}
}

// RefreshNow schedules an immediate poll of a search.
func (e *Engine) RefreshNow(searchID int64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, j := range e.jobs {
		if j.searchID == searchID {
			j.nextRun = time.Now()
			e.signal()
			return true
		}
	}
	return false
}

// runJob performs one poll of a job.
func (e *Engine) runJob(ctx context.Context, j *job) {
	e.mu.Lock()
	p, q, searchID := j.provider, j.query, j.searchID
	alerts := j.alerts
	seeded, lastSuccess := j.seeded, j.lastSuccess
	pending := j.pending
	j.pending = nil
	e.mu.Unlock()

	info := p.Info()
	sctx := activity.WithScope(ctx, activity.Scope{Provider: info.ID, SearchID: searchID, Search: q.Label})
	now := time.Now()

	outcome := e.poll(sctx, p, q, searchID, alerts, seeded, lastSuccess, pending, now)

	e.mu.Lock()
	j.running = false
	j.lastStatus, j.lastError = outcome.status, outcome.errText
	if outcome.err == nil {
		j.seeded = true
		j.lastSuccess = now
		j.lastResults = outcome.listings
		j.lastNew = outcome.newCount
		j.pending = append(outcome.pending, j.pending...)
	} else {
		j.pending = append(pending, j.pending...) // keep them for the next poll
	}
	j.nextRun = time.Now().Add(randomDelay(j.minDelay, j.maxDelay))
	var cool *httpx.CooldownError
	if errors.As(outcome.err, &cool) && cool.Until.After(j.nextRun) {
		j.nextRun = cool.Until.Add(randomDelay(5*time.Second, 30*time.Second))
	}
	e.mu.Unlock()

	if ctx.Err() == nil {
		polled := lastSuccess
		if outcome.err == nil {
			polled = now
		}
		_ = e.store.UpdateSearchState(context.WithoutCancel(ctx), searchID, seeded || outcome.err == nil,
			polled, outcome.status, outcome.errText)
	}
	e.signal()
}

type pollOutcome struct {
	listings []model.Listing
	newCount int
	pending  []pendingItem
	status   string
	errText  string
	err      error
}

func (e *Engine) poll(ctx context.Context, p provider.Provider, q provider.Query, searchID int64,
	alerts []*compiledAlert, seeded bool, lastSuccess time.Time, pending []pendingItem, now time.Time) pollOutcome {

	listings, err := p.Search(ctx, q)
	if err != nil {
		var cool *httpx.CooldownError
		status := "error"
		level := activity.LevelError
		if errors.As(err, &cool) {
			status, level = "cooldown", activity.LevelWarn
		}
		if ctx.Err() == nil {
			e.publish(ctx, activity.Event{Level: level, Kind: activity.KindPoll, Message: "Refresh skipped: " + err.Error()})
		}
		return pollOutcome{status: status, errText: err.Error(), err: err}
	}

	organic := make([]model.Listing, 0, len(listings))
	ids := make([]string, 0, len(listings))
	for _, l := range listings {
		if l.Promoted {
			continue
		}
		organic = append(organic, l)
		ids = append(ids, l.ID)
	}
	freshIDs, err := e.store.MarkSeen(ctx, searchID, ids, now)
	if err != nil {
		e.publish(ctx, activity.Event{Level: activity.LevelError, Kind: activity.KindPoll, Message: "Database error: " + err.Error()})
		return pollOutcome{status: "error", errText: err.Error(), err: err}
	}

	reseed := !lastSuccess.IsZero() && now.Sub(lastSuccess) > e.opts.ReseedAfter
	if !seeded || reseed {
		reason := "first refresh"
		if reseed && seeded {
			reason = fmt.Sprintf("not refreshed for %s", now.Sub(lastSuccess).Round(time.Minute))
		}
		e.publish(ctx, activity.Event{Kind: activity.KindPoll, Message: fmt.Sprintf(
			"Baseline recorded (%s): %d listings marked as seen, alerts start with the next new listing",
			reason, len(organic))})
		return pollOutcome{listings: organic, status: "ok"}
	}

	// New listings, oldest first so notifications arrive in listing order.
	freshSet := make(map[string]bool, len(freshIDs))
	for _, id := range freshIDs {
		freshSet[id] = true
	}
	var fresh []model.Listing
	for i := len(organic) - 1; i >= 0; i-- {
		if freshSet[organic[i].ID] {
			fresh = append(fresh, organic[i])
		}
	}

	candidates := make([]pendingItem, 0, len(pending)+len(fresh))
	candidates = append(candidates, pending...)
	for _, l := range fresh {
		candidates = append(candidates, pendingItem{listing: l})
	}

	var retry []pendingItem
	matches := 0
	for _, c := range candidates {
		matched, needRetry := e.evaluate(ctx, p, &c.listing, alerts)
		if needRetry {
			c.attempts++
			if c.attempts < e.opts.MaxDescriptionAttempts {
				retry = append(retry, c)
			}
		}
		if len(matched) > 0 {
			matches += e.record(ctx, p.Info(), c.listing, matched, now)
		}
	}

	details := make([]string, 0, len(fresh))
	for _, l := range fresh {
		details = append(details, fmt.Sprintf("NEW  %s — %s", l.Title, FormatPrice(l.Price, l.Currency)))
	}
	msg := fmt.Sprintf("%d listings · %d new · %d match", len(organic), len(fresh), matches)
	if matches != 1 {
		msg += "es"
	}
	if promoted := len(listings) - len(organic); promoted > 0 {
		msg += fmt.Sprintf(" · %d promoted ignored", promoted)
	}
	if len(organic) > 0 && len(fresh) == len(organic) {
		msg += " · every listing was new: refresh more often to avoid missing some"
	}
	e.publish(ctx, activity.Event{Kind: activity.KindPoll, Message: msg, Details: details})

	return pollOutcome{listings: organic, newCount: len(fresh), pending: retry, status: "ok"}
}

// evaluate returns the alerts a listing matches. needRetry is true when a
// description was required but could not be fetched for a temporary reason.
func (e *Engine) evaluate(ctx context.Context, p provider.Provider, l *model.Listing, alerts []*compiledAlert) (matched []*compiledAlert, needRetry bool) {
	var titleOK []*compiledAlert
	needDesc := false
	for _, a := range alerts {
		if a.alert.PriceAccepts(l.Price) && a.title.Match(l.Title) {
			titleOK = append(titleOK, a)
			if !a.desc.IsEmpty() {
				needDesc = true
			}
		}
	}
	if len(titleOK) == 0 {
		return nil, false
	}

	descOK := true
	if needDesc && !l.HasDescription {
		if err := e.loadDescription(ctx, p, l); err != nil {
			descOK = false
			needRetry = !errors.Is(err, provider.ErrListingUnavailable) && ctx.Err() == nil
			e.publish(ctx, activity.Event{Level: activity.LevelWarn, Kind: activity.KindPoll,
				Message: fmt.Sprintf("Could not read the description of %q: %v", l.Title, err)})
		}
	}

	for _, a := range titleOK {
		if a.desc.IsEmpty() {
			matched = append(matched, a)
		} else if descOK && a.desc.Match(l.Description) {
			matched = append(matched, a)
		}
	}
	return matched, needRetry
}

// loadDescription fills l.Description from the cache or the provider.
func (e *Engine) loadDescription(ctx context.Context, p provider.Provider, l *model.Listing) error {
	id := p.Info().ID
	if desc, ok, err := e.store.GetCachedDescription(ctx, id, l.ID); err == nil && ok {
		l.Description, l.HasDescription = desc, true
		return nil
	}
	desc, err := p.FetchDescription(ctx, *l)
	if err != nil {
		return err
	}
	l.Description, l.HasDescription = desc, true
	_ = e.store.CacheDescription(ctx, id, l.ID, desc)
	return nil
}

// record stores the matches of a listing and hands them to the notifier.
// It returns the number of matches that were new.
func (e *Engine) record(ctx context.Context, info provider.Info, l model.Listing, matched []*compiledAlert, now time.Time) int {
	stored := l
	if len(stored.Description) > 600 {
		stored.Description = stored.Description[:600] + "…"
	}

	n := Notification{Provider: info, Listing: l}
	destSet := map[int64]bool{}
	for _, a := range matched {
		delivery := model.DeliveryPending
		if len(a.alert.DestinationIDs) == 0 {
			delivery = model.DeliveryNone
		}
		m := model.Match{AlertID: a.alert.ID, AlertName: a.alert.Name, Provider: info.ID, Listing: stored,
			MatchedAt: now, Delivery: delivery}
		inserted, err := e.store.InsertMatch(ctx, &m)
		if err != nil {
			e.publish(ctx, activity.Event{Level: activity.LevelError, Kind: activity.KindMatch, Message: "Database error: " + err.Error()})
			continue
		}
		if !inserted {
			continue // this alert already matched this listing
		}
		e.publish(ctx, activity.Event{Kind: activity.KindMatch, Message: fmt.Sprintf("Match for “%s”: %s — %s",
			a.alert.Name, l.Title, FormatPrice(l.Price, l.Currency)), Details: []string{l.URL}})
		n.Alerts = append(n.Alerts, a.alert)
		n.MatchIDs = append(n.MatchIDs, m.ID)
		for _, d := range a.alert.DestinationIDs {
			if !destSet[d] {
				destSet[d] = true
				n.DestinationIDs = append(n.DestinationIDs, d)
			}
		}
	}
	if len(n.Alerts) > 0 && len(n.DestinationIDs) > 0 && e.notifier != nil {
		e.notifier.Notify(n)
	}
	return len(n.Alerts)
}

func (e *Engine) publish(ctx context.Context, ev activity.Event) {
	if e.bus == nil {
		return
	}
	activity.ScopeFrom(ctx).Apply(&ev)
	e.bus.Publish(ev)
}

func (e *Engine) system(level activity.Level, providerID, msg string) {
	if e.bus == nil {
		return
	}
	e.bus.Publish(activity.Event{Level: level, Kind: activity.KindSystem, Provider: providerID, Message: msg})
}

// FormatPrice renders a price for messages ("12.50 EUR"); unknown prices
// (zero) render as "price n/a".
func FormatPrice(price float64, currency string) string {
	if price <= 0 {
		return "price n/a"
	}
	symbol := map[string]string{"EUR": "€", "GBP": "£", "USD": "$", "PLN": "zł", "CZK": "Kč", "SEK": "kr", "DKK": "kr", "HUF": "Ft", "RON": "lei"}[currency]
	if symbol == "" {
		symbol = currency
	}
	if currency == "GBP" || currency == "USD" {
		return fmt.Sprintf("%s%.2f", symbol, price)
	}
	return fmt.Sprintf("%.2f %s", price, symbol)
}

// SearchStatus describes a job for the UI.
type SearchStatus struct {
	model.Search
	Alerts          []AlertRef `json:"alerts"`
	MinIntervalSec  int        `json:"minIntervalSec"`
	MaxIntervalSec  int        `json:"maxIntervalSec"`
	NextRun         time.Time  `json:"nextRun"`
	Running         bool       `json:"running"`
	LastNew         int        `json:"lastNew"`
	LastResults     int        `json:"lastResults"`
	PendingItems    int        `json:"pendingItems"`
	RequestsPerHour float64    `json:"requestsPerHour"`
}

// AlertRef is a short reference to an alert.
type AlertRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Searches returns the active jobs of a provider ("" for all), sorted by label.
func (e *Engine) Searches(providerID string) []SearchStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := []SearchStatus{}
	for _, j := range e.jobs {
		info := j.provider.Info()
		if providerID != "" && info.ID != providerID {
			continue
		}
		s := SearchStatus{
			Search: model.Search{ID: j.searchID, Provider: info.ID, Key: j.key, Label: j.query.Label,
				WebURL: j.query.WebURL, Seeded: j.seeded, LastPolledAt: j.lastSuccess, LastStatus: j.lastStatus,
				LastError: j.lastError},
			MinIntervalSec:  int(j.minDelay / time.Second),
			MaxIntervalSec:  int(j.maxDelay / time.Second),
			NextRun:         j.nextRun,
			Running:         j.running,
			LastNew:         j.lastNew,
			LastResults:     len(j.lastResults),
			PendingItems:    len(j.pending),
			RequestsPerHour: requestsPerHour(j.minDelay, j.maxDelay),
		}
		for _, a := range j.alerts {
			s.Alerts = append(s.Alerts, AlertRef{ID: a.alert.ID, Name: a.alert.Name})
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Label < out[k].Label })
	return out
}

// EstimatedRequestsPerHour sums the expected search requests of a provider.
func (e *Engine) EstimatedRequestsPerHour(providerID string) float64 {
	total := 0.0
	for _, s := range e.Searches(providerID) {
		total += s.RequestsPerHour
	}
	return total
}

func requestsPerHour(lo, hi time.Duration) float64 {
	avg := (lo + hi) / 2
	if avg <= 0 {
		return 0
	}
	return float64(time.Hour) / float64(avg)
}
