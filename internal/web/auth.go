package web

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/store"
)

const (
	authSettingsKey = "auth"
	sessionCookie   = "zvw_session"
	sessionLifetime = 30 * 24 * time.Hour
	minPasswordLen  = 8
)

// ErrPasswordRequired is returned when removing the password while the web
// interface is reachable from the network.
var ErrPasswordRequired = errors.New("a password is required while the web interface is reachable from other devices")

type authSettings struct {
	PasswordHash string `json:"passwordHash"`
	// Secret signs session cookies. It is rotated when the password changes,
	// which logs out every existing session.
	Secret string `json:"secret"`
}

// Auth protects the web interface with an optional password. Sessions are
// stateless HMAC-signed cookies, so they survive restarts.
type Auth struct {
	store    *store.Store
	loopback bool

	mu  sync.RWMutex
	cfg authSettings
}

// NewAuth loads the authentication settings. When the interface listens on a
// non-loopback address and no password exists, a random password is created
// and returned so it can be shown to the operator.
func NewAuth(ctx context.Context, st *store.Store, loopback bool) (*Auth, string, error) {
	a := &Auth{store: st, loopback: loopback}
	if _, err := st.GetSetting(ctx, authSettingsKey, &a.cfg); err != nil {
		return nil, "", err
	}
	if a.cfg.Secret == "" {
		a.cfg.Secret = randomToken(32)
		if err := a.save(ctx); err != nil {
			return nil, "", err
		}
	}
	generated := ""
	if !loopback && a.cfg.PasswordHash == "" {
		generated = randomPassword()
		if err := a.SetPassword(ctx, generated); err != nil {
			return nil, "", err
		}
	}
	return a, generated, nil
}

// ResetPassword replaces the password with a random one and returns it.
// It is used by the "reset-password" command.
func ResetPassword(ctx context.Context, st *store.Store) (string, error) {
	a := &Auth{store: st, loopback: true}
	if _, err := st.GetSetting(ctx, authSettingsKey, &a.cfg); err != nil {
		return "", err
	}
	pw := randomPassword()
	return pw, a.SetPassword(ctx, pw)
}

// Required reports whether a password protects the interface.
func (a *Auth) Required() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg.PasswordHash != ""
}

// Check verifies a password.
func (a *Auth) Check(password string) bool {
	a.mu.RLock()
	hash := a.cfg.PasswordHash
	a.mu.RUnlock()
	return hash != "" && bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// SetPassword sets a new password (empty removes it) and invalidates every
// existing session.
func (a *Auth) SetPassword(ctx context.Context, password string) error {
	hash := ""
	if password != "" {
		if len(password) < minPasswordLen {
			return errors.New("the password must contain at least 8 characters")
		}
		h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		hash = string(h)
	} else if !a.loopback {
		return ErrPasswordRequired
	}
	a.mu.Lock()
	a.cfg.PasswordHash = hash
	a.cfg.Secret = randomToken(32)
	a.mu.Unlock()
	return a.save(ctx)
}

func (a *Auth) save(ctx context.Context) error {
	a.mu.RLock()
	cfg := a.cfg
	a.mu.RUnlock()
	return a.store.SetSetting(ctx, authSettingsKey, cfg)
}

// Authenticated reports whether the request carries a valid session, or no
// password is configured.
func (a *Auth) Authenticated(r *http.Request) bool {
	if !a.Required() {
		return true
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	expiry, sig, ok := strings.Cut(c.Value, ".")
	if !ok {
		return false
	}
	exp, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(a.sign(expiry)))
}

// IssueSession sets a session cookie on the response.
func (a *Auth) IssueSession(w http.ResponseWriter, r *http.Request) {
	exp := time.Now().Add(sessionLifetime)
	expiry := strconv.FormatInt(exp.Unix(), 10)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    expiry + "." + a.sign(expiry),
		Path:     "/",
		Expires:  exp,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
}

// ClearSession removes the session cookie.
func (a *Auth) ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		SameSite: http.SameSiteStrictMode})
}

func (a *Auth) sign(expiry string) string {
	a.mu.RLock()
	secret := a.cfg.Secret
	a.mu.RUnlock()
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(expiry))
	return hex.EncodeToString(m.Sum(nil))
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// randomPassword returns a readable 16-character password.
func randomPassword() string {
	const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}
