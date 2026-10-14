// Package app builds the HTTP router shared by the Vercel function and the
// local development server.
package app

import (
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"aurabackend/pkg/firebase"
	"aurabackend/pkg/handlers"
	"aurabackend/pkg/httpx"
	"aurabackend/pkg/middleware"
)

type clientHandler func(http.ResponseWriter, *http.Request, *firebase.Clients)

var (
	handlerOnce sync.Once
	handler     http.Handler
)

// Handler returns the application's root http.Handler (built once per process).
func Handler() http.Handler {
	handlerOnce.Do(func() { handler = newRouter() })
	return handler
}

func newRouter() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", handlers.Root)
	mux.HandleFunc("GET /api", handlers.Root)
	mux.HandleFunc("GET /api/index", handlers.Root)
	mux.HandleFunc("GET /api/health", handlers.Health)
	mux.HandleFunc("GET /health", handlers.Health)

	mux.Handle("POST /api/auth/register", public(handlers.Register))
	mux.Handle("POST /auth/register", public(handlers.Register))

	mux.Handle("POST /api/auth/login", public(handlers.Login))
	mux.Handle("POST /auth/login", public(handlers.Login))

	mux.Handle("POST /api/auth/refresh", public(handlers.Refresh))
	mux.Handle("POST /auth/refresh", public(handlers.Refresh))

	mux.Handle("GET /api/auth/me", protected(handlers.Me))
	mux.Handle("GET /auth/me", protected(handlers.Me))

	mux.Handle("PUT /api/auth/profile", protected(handlers.UpdateProfile))
	mux.Handle("PUT /auth/profile", protected(handlers.UpdateProfile))
	mux.Handle("PATCH /api/auth/profile", protected(handlers.UpdateProfile))
	mux.Handle("PATCH /auth/profile", protected(handlers.UpdateProfile))

	mux.Handle("POST /api/auth/logout", protected(handlers.Logout))
	mux.Handle("POST /auth/logout", protected(handlers.Logout))

	// Environmental Zones & 1-Week Impact Analysis
	mux.Handle("GET /api/zones/nearby", public(handlers.NearbyZones))
	mux.Handle("GET /zones/nearby", public(handlers.NearbyZones))

	mux.Handle("GET /api/zones/analysis", public(handlers.ZoneImpactAnalysis))
	mux.Handle("GET /zones/analysis", public(handlers.ZoneImpactAnalysis))
	mux.Handle("POST /api/zones/analysis", public(handlers.ZoneImpactAnalysis))
	mux.Handle("POST /zones/analysis", public(handlers.ZoneImpactAnalysis))

	mux.Handle("POST /api/zones/seed", public(handlers.SeedZones))
	mux.Handle("POST /zones/seed", public(handlers.SeedZones))

	mux.Handle("POST /api/zones", protected(handlers.CreateZone))
	mux.Handle("POST /zones", protected(handlers.CreateZone))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Route not found")
	})

	return middleware.Recover(middleware.CORS(allowedOrigins(), middleware.RewritePath(mux)))
}

// public injects Firebase clients into a handler.
func public(h clientHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := clients(w, r)
		if !ok {
			return
		}
		h(w, r, c)
	})
}

// protected injects Firebase clients and requires a valid Firebase ID token.
func protected(h clientHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := clients(w, r)
		if !ok {
			return
		}
		middleware.RequireAuth(c.Auth, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h(w, r, c)
		})).ServeHTTP(w, r)
	})
}

func clients(w http.ResponseWriter, r *http.Request) (*firebase.Clients, bool) {
	c, err := firebase.Get(r.Context())
	if err != nil {
		log.Printf("firebase init: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "SERVER_MISCONFIGURED", "Server is not configured correctly")
		return nil, false
	}
	return c, true
}

// allowedOrigins is read directly from the environment so CORS (including
// preflight) works even if Firebase configuration is broken.
func allowedOrigins() []string {
	raw := os.Getenv("ALLOWED_ORIGINS")
	if strings.TrimSpace(raw) == "" {
		return []string{"*"}
	}
	var out []string
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}
