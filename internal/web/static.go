package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// dist holds the built single-page application (web/ → internal/web/dist).
// It only contains a placeholder until the frontend is built.
//
//go:embed all:dist
var dist embed.FS

const notBuiltPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>ZVintedWatch</title>
<style>body{font-family:system-ui,sans-serif;max-width:40rem;margin:4rem auto;padding:0 1rem;line-height:1.5}</style>
</head><body><h1>ZVintedWatch is running</h1>
<p>The web interface was not included in this build. Build it with:</p>
<pre>cd web && npm ci && npm run build</pre><p>then rebuild the Go binary. Release downloads already include it.</p>
</body></html>`

func staticHandler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(notBuiltPage))
		})
	}
	index, _ := fs.ReadFile(sub, "index.html")
	files := http.FileServerFS(sub)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" && path != "index.html" {
			if info, err := fs.Stat(sub, path); err == nil && !info.IsDir() {
				if strings.HasPrefix(path, "assets/") {
					// Vite adds a content hash to asset names.
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		// Single-page application: every other path renders index.html.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}
