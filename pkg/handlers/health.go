// Package handlers implements the HTTP endpoints.
package handlers

import (
	"net/http"
	"time"

	"aurabackend/pkg/httpx"
)

// Health is a liveness probe that does not touch Firebase.
//
// GET /api/health
func Health(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC(),
	})
}
