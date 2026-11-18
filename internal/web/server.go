// Package web serves the ZVintedWatch web interface: a JSON API under /api,
// a Server-Sent Events stream for the live console and the single-page
// application embedded in the binary.
package web

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/activity"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/engine"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/httpx"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/notify/telegram"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/store"
)

// csrfHeader must be present on state-changing API requests. Browsers do not
// let other sites set custom headers on cross-origin requests without CORS,
// which this server never grants.
const csrfHeader = "X-ZVW"

// Deps are the components the web server exposes.
type Deps struct {
	Store    *store.Store
	Bus      *activity.Bus
	Fetcher  *httpx.Fetcher
	Registry *provider.Registry
	Engine   *engine.Engine
	Telegram *telegram.Service
	Auth     *Auth
	Logger   *slog.Logger
	// Loopback is true when the interface only listens on localhost.
	Loopback bool
}

// Server is the HTTP handler of the web interface.
type Server struct {
	Deps
	mux *http.ServeMux
}

// New builds the server and its routes.
func New(d Deps) *Server {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	s := &Server{Deps: d, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	m := s.mux
	// Authentication
	m.HandleFunc("GET /api/auth/status", s.api(s.authStatus))
	m.HandleFunc("POST /api/auth/login", s.api(s.login))
	m.HandleFunc("POST /api/auth/logout", s.api(s.logout))
	m.HandleFunc("PUT /api/auth/password", s.api(s.changePassword))

	// Overview
	m.HandleFunc("GET /api/meta", s.api(s.meta))
	m.HandleFunc("GET /api/status", s.api(s.status))

	// Alerts and searches
	m.HandleFunc("GET /api/alerts", s.api(s.listAlerts))
	m.HandleFunc("POST /api/alerts", s.api(s.createAlert))
	m.HandleFunc("GET /api/alerts/{id}", s.api(s.getAlert))
	m.HandleFunc("PUT /api/alerts/{id}", s.api(s.updateAlert))
	m.HandleFunc("DELETE /api/alerts/{id}", s.api(s.deleteAlert))
	m.HandleFunc("POST /api/alerts/{id}/enabled", s.api(s.setAlertEnabled))
	m.HandleFunc("POST /api/alerts/preview", s.api(s.previewAlert))
	m.HandleFunc("POST /api/searches/describe", s.api(s.describeSearch))
	m.HandleFunc("GET /api/searches", s.api(s.listSearches))
	m.HandleFunc("POST /api/searches/{id}/refresh", s.api(s.refreshSearch))
	m.HandleFunc("GET /api/matches", s.api(s.listMatches))
	m.HandleFunc("POST /api/matcher/test", s.api(s.testMatcher))

	// Telegram
	m.HandleFunc("GET /api/destinations", s.api(s.listDestinations))
	m.HandleFunc("POST /api/destinations", s.api(s.createDestination))
	m.HandleFunc("PUT /api/destinations/{id}", s.api(s.updateDestination))
	m.HandleFunc("DELETE /api/destinations/{id}", s.api(s.deleteDestination))
	m.HandleFunc("GET /api/telegram", s.api(s.getTelegram))
	m.HandleFunc("PUT /api/telegram", s.api(s.updateTelegram))
	m.HandleFunc("GET /api/telegram/chats", s.api(s.discoverChats))
	m.HandleFunc("POST /api/telegram/test", s.api(s.testTelegram))

	// Network
	m.HandleFunc("GET /api/settings/network", s.api(s.getNetwork))
	m.HandleFunc("PUT /api/settings/network/{provider}", s.api(s.updateNetwork))
	m.HandleFunc("POST /api/providers/{provider}/reset-cooldown", s.api(s.resetCooldown))

	// Live console
	m.HandleFunc("GET /api/activity", s.api(s.recentActivity))
	m.HandleFunc("GET /api/activity/stream", s.activityStream)

	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, errorBody{"unknown API endpoint"})
	})
	m.Handle("/", staticHandler())
}

// ServeHTTP applies security headers, authentication and CSRF protection.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https: data:; "+
		"style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'")

	if strings.HasPrefix(r.URL.Path, "/api/") {
		public := r.URL.Path == "/api/auth/status" || r.URL.Path == "/api/auth/login"
		if !public && !s.Auth.Authenticated(r) {
			writeJSON(w, http.StatusUnauthorized, errorBody{"authentication required"})
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get(csrfHeader) != "1" {
			writeJSON(w, http.StatusForbidden, errorBody{"missing " + csrfHeader + " header"})
			return
		}
		h.Set("Cache-Control", "no-store")
	}
	s.mux.ServeHTTP(w, r)
}

// apiFunc is an API handler that reports failures by returning an error.
type apiFunc func(w http.ResponseWriter, r *http.Request) error

type errorBody struct {
	Error string `json:"error"`
}

// httpError carries an explicit status code.
type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string { return e.msg }

func badRequest(msg string) error { return &httpError{http.StatusBadRequest, msg} }

func (s *Server) api(h apiFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			s.writeError(w, r, err)
		}
	}
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var he *httpError
	var cool *httpx.CooldownError
	var apiErr *telegram.APIError
	status := http.StatusInternalServerError
	switch {
	case errors.As(err, &he):
		status = he.status
	case errors.Is(err, store.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, engine.ErrValidation), errors.Is(err, provider.ErrInvalidSearch),
		errors.Is(err, telegram.ErrNotConfigured), errors.Is(err, ErrPasswordRequired):
		status = http.StatusBadRequest
	case errors.As(err, &cool):
		status = http.StatusServiceUnavailable
	case errors.As(err, &apiErr):
		status = http.StatusBadGateway
	default:
		s.Logger.Error("API error", "path", r.URL.Path, "err", err)
	}
	writeJSON(w, status, errorBody{err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, v any) error {
	writeJSON(w, http.StatusOK, v)
	return nil
}

func readJSON(r *http.Request, v any) error {
	body := io.LimitReader(r.Body, 1<<20)
	if err := json.NewDecoder(body).Decode(v); err != nil {
		return badRequest("invalid JSON body: " + err.Error())
	}
	return nil
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, badRequest("invalid id")
	}
	return id, nil
}

func queryInt(r *http.Request, key string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(key)); err == nil {
		return v
	}
	return def
}

// startOfDay returns local midnight of t.
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
