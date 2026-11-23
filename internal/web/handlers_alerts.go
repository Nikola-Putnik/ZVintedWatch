package web

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/httpx"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/matcher"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/store"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/version"
)

// meta returns static information used to build the interface.
func (s *Server) meta(w http.ResponseWriter, r *http.Request) error {
	infos := []provider.Info{}
	for _, p := range s.Registry.All() {
		infos = append(infos, p.Info())
	}
	return ok(w, map[string]any{
		"appName":   version.AppName,
		"version":   version.Version,
		"providers": infos,
		"loopback":  s.Loopback,
	})
}

type providerStatus struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	// Enabled is false when the whole site is paused.
	Enabled         bool         `json:"enabled"`
	Health          httpx.Health `json:"health"`
	Alerts          int          `json:"alerts"`
	EnabledAlerts   int          `json:"enabledAlerts"`
	Searches        int          `json:"searches"`
	RequestsPerHour float64      `json:"requestsPerHour"`
	MatchesToday    int          `json:"matchesToday"`
}

// status returns the live dashboard figures.
func (s *Server) status(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	alerts, err := s.Store.ListAlerts(ctx, "")
	if err != nil {
		return err
	}
	today, err := s.Store.CountMatchesSince(ctx, startOfDay(time.Now()))
	if err != nil {
		return err
	}
	dests, err := s.Store.ListDestinations(ctx)
	if err != nil {
		return err
	}

	providers := []providerStatus{}
	for _, p := range s.Registry.All() {
		info := p.Info()
		enabled, err := s.Engine.SiteEnabled(ctx, info.ID)
		if err != nil {
			return err
		}
		ps := providerStatus{
			ID: info.ID, Name: info.Name, Color: info.Color, Enabled: enabled,
			Health:          s.Fetcher.Health(info.ID),
			Searches:        len(s.Engine.Searches(info.ID)),
			RequestsPerHour: s.Engine.EstimatedRequestsPerHour(info.ID),
			MatchesToday:    today[info.ID],
		}
		for _, a := range alerts {
			if a.Provider == info.ID {
				ps.Alerts++
				if a.Enabled {
					ps.EnabledAlerts++
				}
			}
		}
		providers = append(providers, ps)
	}
	// Alerts of sites whose module is not installed are kept untouched.
	orphans := 0
	for _, a := range alerts {
		if _, found := s.Registry.Get(a.Provider); !found {
			orphans++
		}
	}
	tg := s.Telegram.Settings()
	return ok(w, map[string]any{
		"providers":    providers,
		"orphanAlerts": orphans,
		"telegram": map[string]any{
			"configured":   tg.Token != "",
			"botUsername":  tg.BotUsername,
			"destinations": len(dests),
		},
	})
}

// alertView enriches an alert with the search it is polled with.
type alertView struct {
	model.Alert
	SearchLabel string `json:"searchLabel"`
	SearchKey   string `json:"searchKey"`
	SearchURL   string `json:"searchUrl"`
	SearchID    int64  `json:"searchId"`
	SearchError string `json:"searchError,omitempty"`
	// SharedWith is the number of other enabled alerts polling the same search.
	SharedWith int `json:"sharedWith"`
}

func (s *Server) alertViews(ctx context.Context, providerID string) ([]alertView, error) {
	alerts, err := s.Store.ListAlerts(ctx, providerID)
	if err != nil {
		return nil, err
	}
	type jobInfo struct {
		id      int64
		members int
	}
	jobs := map[string]jobInfo{}
	for _, se := range s.Engine.Searches(providerID) {
		jobs[se.Key] = jobInfo{id: se.ID, members: len(se.Alerts)}
	}

	views := make([]alertView, 0, len(alerts))
	for _, a := range alerts {
		v := alertView{Alert: a}
		if p, found := s.Registry.Get(a.Provider); found {
			q, err := p.ParseSearch(provider.SearchInput{Text: a.SearchInput, Domain: a.Domain})
			if err != nil {
				v.SearchError = err.Error()
			} else {
				v.SearchLabel, v.SearchKey, v.SearchURL = q.Label, q.Key(), q.WebURL
				if j, ok := jobs[v.SearchKey]; ok {
					v.SearchID = j.id
					v.SharedWith = j.members
					if a.Enabled {
						v.SharedWith--
					}
				}
			}
		} else {
			v.SearchError = "unknown site"
		}
		views = append(views, v)
	}
	return views, nil
}

func (s *Server) listAlerts(w http.ResponseWriter, r *http.Request) error {
	views, err := s.alertViews(r.Context(), r.URL.Query().Get("provider"))
	if err != nil {
		return err
	}
	return ok(w, views)
}

func (s *Server) getAlert(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	a, err := s.Store.GetAlert(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, a)
}

// readAlert decodes and normalizes an alert from the request body.
func readAlert(r *http.Request) (model.Alert, error) {
	var a model.Alert
	if err := readJSON(r, &a); err != nil {
		return a, err
	}
	a.Name = strings.TrimSpace(a.Name)
	a.SearchInput = strings.TrimSpace(a.SearchInput)
	a.Title = cleanFilter(a.Title)
	a.Description = cleanFilter(a.Description)
	if a.DestinationIDs == nil {
		a.DestinationIDs = []int64{}
	}
	return a, nil
}

func cleanFilter(f model.TextFilter) model.TextFilter {
	clean := func(terms []string) []string {
		out := []string{}
		for _, t := range terms {
			if t = strings.TrimSpace(t); t != "" {
				out = append(out, t)
			}
		}
		return out
	}
	f.Include, f.Exclude = clean(f.Include), clean(f.Exclude)
	if f.IncludeMode != model.MatchAny {
		f.IncludeMode = model.MatchAll
	}
	return f
}

func (s *Server) createAlert(w http.ResponseWriter, r *http.Request) error {
	a, err := readAlert(r)
	if err != nil {
		return err
	}
	if err := s.Engine.Validate(a); err != nil {
		return err
	}
	created, err := s.Store.CreateAlert(r.Context(), a)
	if err != nil {
		return err
	}
	if err := s.Engine.Reload(r.Context()); err != nil {
		return err
	}
	return ok(w, created)
}

func (s *Server) updateAlert(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	a, err := readAlert(r)
	if err != nil {
		return err
	}
	a.ID = id
	if err := s.Engine.Validate(a); err != nil {
		return err
	}
	updated, err := s.Store.UpdateAlert(r.Context(), a)
	if err != nil {
		return err
	}
	if err := s.Engine.Reload(r.Context()); err != nil {
		return err
	}
	return ok(w, updated)
}

func (s *Server) deleteAlert(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.Store.DeleteAlert(r.Context(), id); err != nil {
		return err
	}
	if err := s.Engine.Reload(r.Context()); err != nil {
		return err
	}
	return ok(w, map[string]bool{"deleted": true})
}

func (s *Server) setAlertEnabled(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if err := s.Store.SetAlertEnabled(r.Context(), id, body.Enabled); err != nil {
		return err
	}
	if err := s.Engine.Reload(r.Context()); err != nil {
		return err
	}
	return ok(w, map[string]bool{"enabled": body.Enabled})
}

func (s *Server) previewAlert(w http.ResponseWriter, r *http.Request) error {
	a, err := readAlert(r)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	res, err := s.Engine.Preview(ctx, a)
	if err != nil {
		return err
	}
	return ok(w, res)
}

func (s *Server) describeSearch(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Provider string `json:"provider"`
		Input    string `json:"input"`
		Domain   string `json:"domain"`
		AlertID  int64  `json:"alertId"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	info, err := s.Engine.DescribeSearch(body.Provider, body.Input, body.Domain, body.AlertID)
	if err != nil {
		return badRequest(err.Error())
	}
	return ok(w, info)
}

func (s *Server) listSearches(w http.ResponseWriter, r *http.Request) error {
	return ok(w, s.Engine.Searches(r.URL.Query().Get("provider")))
}

func (s *Server) refreshSearch(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if !s.Engine.RefreshNow(id) {
		return store.ErrNotFound
	}
	return ok(w, map[string]bool{"scheduled": true})
}

func (s *Server) listMatches(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	alertID, _ := strconv.ParseInt(q.Get("alertId"), 10, 64)
	matches, err := s.Store.ListMatches(r.Context(), store.MatchFilter{
		Provider: q.Get("provider"),
		AlertID:  alertID,
		Limit:    queryInt(r, "limit", 100),
	})
	if err != nil {
		return err
	}
	return ok(w, matches)
}

// testMatcher evaluates a keyword filter against a sample text, so the alert
// editor can show the result of the rules while they are typed.
func (s *Server) testMatcher(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Filter model.TextFilter `json:"filter"`
		Text   string           `json:"text"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	f, err := matcher.Compile(cleanFilter(body.Filter))
	if err != nil {
		return badRequest(err.Error())
	}
	return ok(w, map[string]bool{"match": f.Match(body.Text)})
}
