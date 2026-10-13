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

// resolvePath restores the original request path if it was rewritten by Vercel.
func resolvePath(r *http.Request) {
	// 1. Try Vercel's x-matched-path header (set automatically by Vercel edge rewrite)
	if p := r.Header.Get("x-matched-path"); p != "" {
		if u, err := url.Parse(p); err == nil && u.Path != "" {
			r.URL.Path = u.Path
			return
		}
		r.URL.Path = p
		return
	}

	// 2. Try __path query parameter if passed in vercel.json rewrite
	if p := r.URL.Query().Get("__path"); p != "" {
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		r.URL.Path = p

		// Clean up __path so business logic doesn't see it
		q := r.URL.Query()
		q.Del("__path")
		r.URL.RawQuery = q.Encode()
		return
	}

	// 3. Fallback to common proxy headers if present
	if p := r.Header.Get("x-forwarded-uri"); p != "" {
		if u, err := url.Parse(p); err == nil && u.Path != "" {
			r.URL.Path = u.Path
			return
		}
	}
}
