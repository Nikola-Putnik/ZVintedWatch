package httpx

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// Browser identity used by the production transport. The TLS profile, the
// User-Agent and the client hints must describe the same browser version.
const (
	// UserAgent is the User-Agent header matching the TLS profile.
	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
	// SecChUa is the sec-ch-ua client hint matching UserAgent.
	SecChUa = `"Chromium";v="146", "Not-A.Brand";v="24", "Google Chrome";v="146"`
	// SecChUaPlatform is the sec-ch-ua-platform client hint matching UserAgent.
	SecChUaPlatform = `"Windows"`
	// AcceptEncoding is the Accept-Encoding header sent by the browser.
	AcceptEncoding = "gzip, deflate, br, zstd"
)

const defaultMaxBody = 8 << 20

// browserTransport impersonates Chrome using tls-client. Each instance owns a
// cookie jar, so cookies set by a site persist across requests of the lane.
type browserTransport struct {
	client tls_client.HttpClient
}

// NewBrowserTransport creates a Chrome-impersonating transport. It is the
// default TransportFactory.
func NewBrowserTransport(proxy string, timeout time.Duration) (Transport, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	opts := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(int(timeout.Seconds())),
		tls_client.WithClientProfile(profiles.Chrome_146),
		tls_client.WithCookieJar(tls_client.NewCookieJar()),
	}
	if proxy != "" {
		opts = append(opts, tls_client.WithProxyUrl(proxy))
	}
	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), opts...)
	if err != nil {
		return nil, err
	}
	return &browserTransport{client: client}, nil
}

func (t *browserTransport) Do(ctx context.Context, req *Request) (*Response, error) {
	method := req.Method
	if method == "" {
		method = fhttp.MethodGet
	}
	hreq, err := fhttp.NewRequestWithContext(ctx, method, req.URL, nil)
	if err != nil {
		return nil, err
	}

	header := fhttp.Header{}
	order := make([]string, 0, len(req.Headers))
	for _, h := range req.Headers {
		name := strings.ToLower(h.Name)
		header[name] = append(header[name], h.Value)
		order = append(order, name)
	}
	header[fhttp.HeaderOrderKey] = order
	hreq.Header = header

	start := time.Now()
	hresp, err := t.client.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer hresp.Body.Close()

	body := fhttp.DecompressBody(hresp)
	defer body.Close()

	resp := &Response{Status: hresp.StatusCode, Header: http.Header(hresp.Header)}
	counted := &countingReader{r: body}
	if req.Consume != nil {
		err = req.Consume(counted)
	} else {
		limit := req.MaxBodyBytes
		if limit <= 0 {
			limit = defaultMaxBody
		}
		resp.Body, err = io.ReadAll(io.LimitReader(counted, limit))
	}
	resp.Bytes = counted.n
	resp.Duration = time.Since(start)
	if err != nil {
		return resp, fmt.Errorf("read body: %w", err)
	}
	return resp, nil
}

func (t *browserTransport) Cookies(rawURL string) []Cookie {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	var out []Cookie
	for _, c := range t.client.GetCookies(u) {
		out = append(out, Cookie{Name: c.Name, Value: c.Value})
	}
	return out
}

func (t *browserTransport) Close() { t.client.CloseIdleConnections() }

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
