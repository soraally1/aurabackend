// Package handler is the Vercel Go serverless entrypoint. vercel.json rewrites
// requests to this function, and the router dispatches them.
package handler

import (
	"net/http"
	"net/url"
	"strings"

	"aurabackend/pkg/app"
)

// Handler is invoked by the Vercel Go runtime.
func Handler(w http.ResponseWriter, r *http.Request) {
	resolvePath(r)
	app.Handler().ServeHTTP(w, r)
}

func resolvePath(r *http.Request) {
	// 1. Utamakan query parameter __path dari rewrite vercel.json: /(.*) -> /api/index?__path=/$1
	if p := r.URL.Query().Get("__path"); p != "" {
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		r.URL.Path = p

		// Bersihkan query __path agar tidak terlihat oleh logic handler
		q := r.URL.Query()
		q.Del("__path")
		r.URL.RawQuery = q.Encode()
		return
	}

	// 2. Fallback ke header x-matched-path HANYA jika bukan entrypoint fungsi (/api/index atau /api)
	if p := r.Header.Get("x-matched-path"); p != "" && p != "/api/index" && p != "/api" {
		if u, err := url.Parse(p); err == nil && u.Path != "" {
			r.URL.Path = u.Path
			return
		}
		r.URL.Path = p
		return
	}

	// 3. Fallback ke proxy headers lainnya
	if p := r.Header.Get("x-forwarded-uri"); p != "" {
		if u, err := url.Parse(p); err == nil && u.Path != "" {
			r.URL.Path = u.Path
			return
		}
	}
}
