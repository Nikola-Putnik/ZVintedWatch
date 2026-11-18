package vinted

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/httpx"
)

// session holds the anonymous credentials Vinted expects on API calls.
type session struct {
	token  string // value of the access_token_web cookie (a JWT)
	anonID string // value of the anon_id cookie / X-Anon-Id header
}

// ensureSession returns the session stored in the lane's cookie jar for the
// domain, creating or refreshing it with a lightweight HEAD request on the
// homepage when it is missing, expired or force is set.
func (p *Provider) ensureSession(ctx context.Context, lease *httpx.Lease, domain string, force bool) (session, error) {
	home := "https://www." + domain + "/"
	if !force {
		if s := sessionFromCookies(lease.Cookies(home)); s.token != "" && !tokenExpired(s.token, time.Now()) {
			return s, nil
		}
	}

	resp, err := lease.Do(ctx, &httpx.Request{
		Method:  "HEAD",
		URL:     home,
		Headers: documentHeaders(domain, "none", ""),
		Purpose: "session",
	})
	if err != nil {
		return session{}, err
	}
	if resp.Status == 403 {
		lease.Block("session refused (HTTP 403)", 0)
		return session{}, fmt.Errorf("vinted refused the session request (HTTP 403)")
	}
	if resp.Status >= 400 {
		return session{}, fmt.Errorf("session request failed (HTTP %d)", resp.Status)
	}

	s := sessionFromCookies(lease.Cookies(home))
	if s.anonID == "" {
		s.anonID = resp.Header.Get("X-Anon-Id")
	}
	if s.token == "" {
		return session{}, fmt.Errorf("vinted did not return a session token")
	}
	return s, nil
}

func sessionFromCookies(cookies []httpx.Cookie) session {
	var s session
	for _, c := range cookies {
		switch c.Name {
		case "access_token_web":
			s.token = c.Value
		case "anon_id":
			s.anonID = c.Value
		}
	}
	return s
}

// tokenExpired reports whether a JWT expires within the next minute. Tokens
// that cannot be decoded are considered valid; the API will reject them if not.
func tokenExpired(token string, now time.Time) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Exp == 0 {
		return false
	}
	return now.Add(time.Minute).Unix() >= claims.Exp
}

// documentHeaders are the headers of a top-level page navigation.
func documentHeaders(domain, fetchSite, referer string) []httpx.Header {
	h := []httpx.Header{
		{Name: "sec-ch-ua", Value: httpx.SecChUa},
		{Name: "sec-ch-ua-mobile", Value: "?0"},
		{Name: "sec-ch-ua-platform", Value: httpx.SecChUaPlatform},
		{Name: "upgrade-insecure-requests", Value: "1"},
		{Name: "user-agent", Value: httpx.UserAgent},
		{Name: "accept", Value: "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"},
		{Name: "sec-fetch-site", Value: fetchSite},
		{Name: "sec-fetch-mode", Value: "navigate"},
		{Name: "sec-fetch-user", Value: "?1"},
		{Name: "sec-fetch-dest", Value: "document"},
	}
	if referer != "" {
		h = append(h, httpx.Header{Name: "referer", Value: referer})
	}
	return append(h,
		httpx.Header{Name: "accept-encoding", Value: httpx.AcceptEncoding},
		httpx.Header{Name: "accept-language", Value: acceptLanguage(domain)},
	)
}

// apiHeaders are the headers of an XHR call from the website to the API.
func apiHeaders(domain string, s session) []httpx.Header {
	return []httpx.Header{
		{Name: "sec-ch-ua-platform", Value: httpx.SecChUaPlatform},
		{Name: "authorization", Value: "Bearer " + s.token},
		{Name: "sec-ch-ua", Value: httpx.SecChUa},
		{Name: "sec-ch-ua-mobile", Value: "?0"},
		{Name: "x-anon-id", Value: s.anonID},
		{Name: "user-agent", Value: httpx.UserAgent},
		{Name: "accept", Value: "application/json, text/plain, */*"},
		{Name: "origin", Value: "https://www." + domain},
		{Name: "sec-fetch-site", Value: "same-site"},
		{Name: "sec-fetch-mode", Value: "cors"},
		{Name: "sec-fetch-dest", Value: "empty"},
		{Name: "referer", Value: "https://www." + domain + "/"},
		{Name: "accept-encoding", Value: httpx.AcceptEncoding},
		{Name: "accept-language", Value: acceptLanguage(domain)},
	}
}
