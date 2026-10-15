package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"aurabackend/pkg/firebase"
	"aurabackend/pkg/httpx"
	"aurabackend/pkg/middleware"
	"aurabackend/pkg/mobility"
	"aurabackend/pkg/zones"
)

// ProcessMobilityBatch handles POST /api/v1/mobility/batch.
// Receives daily waypoints, detects stay vs travel, matches pollution index,
// runs Groq AI (Llama-3.3-70b-versatile) exposure analysis, and persists result.
func ProcessMobilityBatch(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	if r.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	var req mobility.BatchRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	if len(req.Waypoints) == 0 {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", "Waypoints array cannot be empty")
		return
	}

	// Determine User ID (from auth header if valid, otherwise body, fallback to guest)
	userID := resolveUserID(r, c, strings.TrimSpace(req.UserID))

	dateStr := strings.TrimSpace(req.Date)
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	// Prepare analyzer dependencies
	zoneStore := zones.NewStore(c.Firestore, c.Config.ZonesCollection)
	groqClient := mobility.NewGroqClient(c.Config.GroqAPIKey)
	analyzer := mobility.NewAnalyzer(zoneStore, groqClient)

	analysis, err := analyzer.Analyze(r.Context(), userID, dateStr, req.Waypoints)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ANALYSIS_ERROR", fmt.Sprintf("Failed to analyze mobility: %v", err))
		return
	}

	// Persist to Firestore
	mobilityStore := mobility.NewStore(c.Firestore, c.Config.MobilityCollection)
	if err := mobilityStore.Save(r.Context(), &analysis); err != nil {
		// Log warning but return calculated analysis so user doesn't get blocked
		fmt.Printf("[Mobility] Warning saving to Firestore: %v\n", err)
	}

	httpx.OK(w, http.StatusOK, analysis)
}

// GetDailyAnalysis handles GET /api/v1/mobility/daily-analysis?date=YYYY-MM-DD.
// Retrieves the computed daily exposure and AI doctor diagnosis for the user.
func GetDailyAnalysis(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	if r.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	q := r.URL.Query()

	dateStr := strings.TrimSpace(q.Get("date"))
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	userID := resolveUserID(r, c, strings.TrimSpace(q.Get("user_id")))

	mobilityStore := mobility.NewStore(c.Firestore, c.Config.MobilityCollection)
	analysis, err := mobilityStore.Get(r.Context(), userID, dateStr)
	if err != nil {
		if err == mobility.ErrNotFound {
			httpx.Error(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("Belum ada data mobilitas untuk tanggal %s", dateStr))
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}

	httpx.OK(w, http.StatusOK, analysis)
}

func resolveUserID(r *http.Request, c *firebase.Clients, fallback string) string {
	if uid := middleware.UID(r.Context()); uid != "" {
		return uid
	}
	authHeader := r.Header.Get("Authorization")
	scheme, raw, ok := strings.Cut(authHeader, " ")
	if ok && strings.EqualFold(scheme, "Bearer") && strings.TrimSpace(raw) != "" && c != nil && c.Auth != nil {
		if token, err := c.Auth.VerifyIDToken(r.Context(), strings.TrimSpace(raw)); err == nil && token != nil {
			return token.UID
		}
	}
	if fallback != "" {
		return fallback
	}
	return "guest-user"
}
