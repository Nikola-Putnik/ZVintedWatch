package web

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/httpx"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
)

// --- Authentication -------------------------------------------------------

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) error {
	return ok(w, map[string]bool{
		"required":      s.Auth.Required(),
		"authenticated": s.Auth.Authenticated(r),
		"loopback":      s.Loopback,
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get(csrfHeader) != "1" {
		return &httpError{http.StatusForbidden, "missing " + csrfHeader + " header"}
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if !s.Auth.Required() {
		return ok(w, map[string]bool{"authenticated": true})
	}
	if !s.Auth.Check(body.Password) {
		time.Sleep(700 * time.Millisecond) // slow down guessing
		return &httpError{http.StatusUnauthorized, "wrong password"}
	}
	s.Auth.IssueSession(w, r)
	return ok(w, map[string]bool{"authenticated": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) error {
	s.Auth.ClearSession(w)
	return ok(w, map[string]bool{"authenticated": false})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if s.Auth.Required() && !s.Auth.Check(body.Current) {
		return badRequest("the current password is wrong")
	}
	if err := s.Auth.SetPassword(r.Context(), body.New); err != nil {
		return badRequest(err.Error())
	}
	if s.Auth.Required() {
		s.Auth.IssueSession(w, r)
	}
	return ok(w, map[string]bool{"required": s.Auth.Required()})
}

// --- Telegram destinations ------------------------------------------------

var chatIDPattern = regexp.MustCompile(`^(-?\d{1,20}|@[A-Za-z][A-Za-z0-9_]{3,63})$`)

func readDestination(r *http.Request) (model.Destination, error) {
	var d model.Destination
	if err := readJSON(r, &d); err != nil {
		return d, err
	}
	d.Name = strings.TrimSpace(d.Name)
	d.ChatID = strings.TrimSpace(d.ChatID)
	if d.Name == "" {
		return d, badRequest("the name is required")
	}
	if !chatIDPattern.MatchString(d.ChatID) {
		return d, badRequest("the chat id must be a number (e.g. 123456789 or -1001234567890) or a public channel name such as @mychannel")
	}
	return d, nil
}

func (s *Server) listDestinations(w http.ResponseWriter, r *http.Request) error {
	list, err := s.Store.ListDestinations(r.Context())
	if err != nil {
		return err
	}
	return ok(w, list)
}

func (s *Server) createDestination(w http.ResponseWriter, r *http.Request) error {
	d, err := readDestination(r)
	if err != nil {
		return err
	}
	created, err := s.Store.CreateDestination(r.Context(), d)
	if err != nil {
		return err
	}
	return ok(w, created)
}

func (s *Server) updateDestination(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	d, err := readDestination(r)
	if err != nil {
		return err
	}
	d.ID = id
	if err := s.Store.UpdateDestination(r.Context(), d); err != nil {
		return err
	}
	return ok(w, d)
}

// deleteDestination removes a destination and detaches it from every alert.
func (s *Server) deleteDestination(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	if err := s.Store.DeleteDestination(ctx, id); err != nil {
		return err
	}
	alerts, err := s.Store.ListAlerts(ctx, "")
	if err != nil {
		return err
	}
	for _, a := range alerts {
		kept := []int64{}
		for _, d := range a.DestinationIDs {
			if d != id {
				kept = append(kept, d)
			}
		}
		if len(kept) != len(a.DestinationIDs) {
			a.DestinationIDs = kept
			if _, err := s.Store.UpdateAlert(ctx, a); err != nil {
				return err
			}
		}
	}
	if s.Telegram.Settings().SystemDestinationID == id {
		_ = s.Telegram.SetSystemDestination(ctx, 0)
	}
	if err := s.Engine.Reload(ctx); err != nil {
		return err
	}
	return ok(w, map[string]bool{"deleted": true})
}

// --- Telegram bot -----------------------------------------------------------

func (s *Server) telegramView() map[string]any {
	cfg := s.Telegram.Settings()
	hint := ""
	if cfg.Token != "" {
		// Show only the bot id part and the last characters of the secret.
		id, secret, _ := strings.Cut(cfg.Token, ":")
		if len(secret) > 4 {
			secret = secret[len(secret)-4:]
		}
		hint = id + ":…" + secret
	}
	return map[string]any{
		"configured":          cfg.Token != "",
		"botUsername":         cfg.BotUsername,
		"tokenHint":           hint,
		"systemDestinationId": cfg.SystemDestinationID,
	}
}

func (s *Server) getTelegram(w http.ResponseWriter, r *http.Request) error {
	return ok(w, s.telegramView())
}

func (s *Server) updateTelegram(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Token               *string `json:"token"`
		SystemDestinationID *int64  `json:"systemDestinationId"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if body.Token != nil {
		if _, err := s.Telegram.SetToken(ctx, *body.Token); err != nil {
			return badRequest(err.Error())
		}
	}
	if body.SystemDestinationID != nil {
		if err := s.Telegram.SetSystemDestination(ctx, *body.SystemDestinationID); err != nil {
			return err
		}
	}
	return ok(w, s.telegramView())
}

func (s *Server) discoverChats(w http.ResponseWriter, r *http.Request) error {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	chats, err := s.Telegram.DiscoverChats(ctx)
	if err != nil {
		return err
	}
	return ok(w, chats)
}

func (s *Server) testTelegram(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		ChatID string `json:"chatId"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.Telegram.SendTest(ctx, strings.TrimSpace(body.ChatID)); err != nil {
		return err
	}
	return ok(w, map[string]bool{"sent": true})
}

// --- Network settings -------------------------------------------------------

// NetworkSettingsKey returns the settings key of a provider's network settings.
func NetworkSettingsKey(providerID string) string { return "network." + providerID }

func (s *Server) getNetwork(w http.ResponseWriter, r *http.Request) error {
	out := map[string]httpx.Settings{}
	for _, p := range s.Registry.All() {
		out[p.Info().ID] = s.Fetcher.Settings(p.Info().ID)
	}
	return ok(w, out)
}

func (s *Server) updateNetwork(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("provider")
	if _, found := s.Registry.Get(id); !found {
		return badRequest("unknown site")
	}
	var settings httpx.Settings
	if err := readJSON(r, &settings); err != nil {
		return err
	}
	settings = settings.Normalized()
	if settings.MinGapMs < 1000 {
		return badRequest("the minimum gap between requests must be at least 1000 ms")
	}
	if err := s.Fetcher.Configure(id, settings); err != nil {
		return badRequest(err.Error())
	}
	if err := s.Store.SetSetting(r.Context(), NetworkSettingsKey(id), settings); err != nil {
		return err
	}
	return ok(w, settings)
}

func (s *Server) resetCooldown(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("provider")
	if _, found := s.Registry.Get(id); !found {
		return badRequest("unknown site")
	}
	s.Fetcher.ResetCooldown(id)
	return ok(w, s.Fetcher.Health(id))
}
