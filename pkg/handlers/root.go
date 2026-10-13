package handlers

import (
	"net/http"
	"time"

	"aurabackend/pkg/httpx"
)

// Root is the landing handler for GET / and GET /api.
func Root(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, http.StatusOK, map[string]any{
		"name":      "Aura Backend API",
		"status":    "online",
		"timestamp": time.Now().UTC(),
		"endpoints": map[string]string{
			"health":   "GET /api/health",
			"register": "POST /api/auth/register",
			"login":    "POST /api/auth/login",
			"refresh":  "POST /api/auth/refresh",
			"me":       "GET /api/auth/me",
			"logout":   "POST /api/auth/logout",
		},
	})
}
