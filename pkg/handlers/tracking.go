package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"aurabackend/pkg/firebase"
	"aurabackend/pkg/httpx"
	"aurabackend/pkg/middleware"
	"aurabackend/pkg/tracking"
	"aurabackend/pkg/zones"
)

// StartSession handles POST /api/v1/tracking/start-session.
// Creates a new active tracking session.
func StartSession(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	if r.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	var req tracking.StartSessionRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	// Validate start location
	if err := validateLocation(req.StartLocation); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	// Get user ID from auth token or request body
	userID := resolveUserIDFromAuth(r, c, strings.TrimSpace(req.UserID))

	// Generate session ID: session_{userID}_{unixTimestamp}
	sessionID := fmt.Sprintf("session_%s_%d", userID, time.Now().Unix())

	// Create session object
	session := &tracking.Session{
		ID:            sessionID,
		UserID:        userID,
		SessionName:   strings.TrimSpace(req.SessionName),
		Status:        "active",
		StartedAt:     time.Now().UTC(),
		StartLocation: req.StartLocation,
		Waypoints:     []tracking.Waypoint{},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	// Persist to Firestore
	store := tracking.NewStore(c.Firestore, c.Config.TrackingCollection)
	if err := store.CreateSession(r.Context(), session); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", fmt.Sprintf("Failed to create session: %v", err))
		return
	}

	// Return response
	response := map[string]any{
		"session_id": session.ID,
		"started_at": session.StartedAt.Format(time.RFC3339),
		"status":     "active",
	}

	httpx.OK(w, http.StatusCreated, response)
}

// SyncWaypoints handles POST /api/v1/tracking/sync-waypoints.
// Syncs GPS waypoints during an active tracking session with hybrid smart sync.
func SyncWaypoints(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	if r.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	var req tracking.SyncWaypointsRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	if req.SessionID == "" {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", "session_id is required")
		return
	}

	if len(req.Waypoints) == 0 {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", "waypoints array cannot be empty")
		return
	}

	// Validate all waypoints
	for _, wp := range req.Waypoints {
		if err := validateWaypoint(wp); err != nil {
			httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
			return
		}
	}

	// Get or verify session
	store := tracking.NewStore(c.Firestore, c.Config.TrackingCollection)
	session, err := store.GetSession(r.Context(), req.SessionID)
	if err != nil {
		if err == tracking.ErrNotFound {
			httpx.Error(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session not found or expired")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}

	if session.Status != "active" {
		httpx.Error(w, http.StatusBadRequest, "SESSION_NOT_FOUND", "Session is not active")
		return
	}

	// Process waypoints: classify and enrich with zone data
	zoneStore := zones.NewStore(c.Firestore, c.Config.ZonesCollection)
	analyzer := tracking.NewAnalyzer(zoneStore)
	processedWaypoints := analyzer.ProcessWaypoints(r.Context(), req.Waypoints)

	// Update session with waypoints
	if err := store.UpdateWaypoints(r.Context(), req.SessionID, processedWaypoints); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", fmt.Sprintf("Failed to update waypoints: %v", err))
		return
	}

	// Get updated session to return total waypoint count
	updatedSession, _ := store.GetSession(r.Context(), req.SessionID)
	totalCount := len(updatedSession.Waypoints)

	response := map[string]any{
		"synced_count":    len(processedWaypoints),
		"session_id":      req.SessionID,
		"total_waypoints": totalCount,
		"last_sync_at":    time.Now().UTC().Format(time.RFC3339),
	}

	httpx.OK(w, http.StatusOK, response)
}

// StopSession handles POST /api/v1/tracking/stop-session.
// Ends an active tracking session and returns statistics with AI health analysis.
func StopSession(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	if r.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	var req tracking.StopSessionRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	if req.SessionID == "" {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", "session_id is required")
		return
	}

	if err := validateLocation(req.EndLocation); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	// Get session
	store := tracking.NewStore(c.Firestore, c.Config.TrackingCollection)
	session, err := store.GetSession(r.Context(), req.SessionID)
	if err != nil {
		if err == tracking.ErrNotFound {
			httpx.Error(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}

	if session.Status != "active" {
		httpx.Error(w, http.StatusBadRequest, "SESSION_ALREADY_ENDED", "Session already ended")
		return
	}

	// Calculate stats
	stats := tracking.CalculateStats(session.Waypoints, session.StartedAt, req.EndLocation.Timestamp)

	// Generate health analysis (with Groq if available)
	groqClient := tracking.NewGroqClient(c.Config.GroqAPIKey)
	analyzer := tracking.NewAnalyzerWithGroq(nil, groqClient)
	analysis := analyzer.GenerateHealthAnalysisWithGroq(r.Context(), stats)

	// End session with stats and analysis
	if err := store.EndSession(r.Context(), req.SessionID, req.EndLocation, stats, analysis); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", fmt.Sprintf("Failed to end session: %v", err))
		return
	}

	// Prepare response
	response := map[string]any{
		"session_id":       req.SessionID,
		"started_at":       session.StartedAt.Format(time.RFC3339),
		"ended_at":         req.EndLocation.Timestamp.Format(time.RFC3339),
		"duration_minutes": stats.DurationMinutes,
		"distance_km":      stats.DistanceKm,
		"stats": map[string]any{
			"total_outdoor_minutes": stats.TotalOutdoorMinutes,
			"total_indoor_minutes":  stats.TotalIndoorMinutes,
			"average_outdoor_aqi":   stats.AverageOutdoorAQI,
			"average_outdoor_pm25":  stats.AverageOutdoorPM25,
			"max_aqi":               stats.MaxAQI,
			"worst_area_name":       stats.WorstAreaName,
			"worst_area_aqi":        stats.WorstAreaAQI,
			"waypoints_processed":   stats.WaypointsProcessed,
		},
		"analysis": map[string]any{
			"impact_score":    analysis.ImpactScore,
			"risk_level":      analysis.RiskLevel,
			"summary":         analysis.Summary,
			"recommendations": analysis.Recommendations,
		},
	}

	httpx.OK(w, http.StatusOK, response)
}

// DailyHistory handles GET /api/v1/tracking/daily-history.
// Fetches all journeys and waypoints for a specific date for replay.
func DailyHistory(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	if r.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	q := r.URL.Query()

	// Get user ID from auth token or query param
	userID := resolveUserIDFromAuth(r, c, strings.TrimSpace(q.Get("user_id")))

	dateStr := strings.TrimSpace(q.Get("date"))
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	// Validate date format
	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_DATE_FORMAT", "Invalid date format, use YYYY-MM-DD")
		return
	}

	// Get sessions for the date
	store := tracking.NewStore(c.Firestore, c.Config.TrackingCollection)
	sessions, err := store.GetSessionsByDate(r.Context(), userID, dateStr)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}

	// Aggregate waypoints and stats
	var combinedWaypoints []tracking.Waypoint
	dailyStats := tracking.SessionStats{
		WaypointsProcessed: 0,
	}

	sessionList := make([]map[string]any, 0)

	for _, session := range sessions {
		// Add session to list
		sessionData := map[string]any{
			"session_id":       session.ID,
			"session_name":     session.SessionName,
			"started_at":       session.StartedAt.Format(time.RFC3339),
			"duration_minutes": 0,
			"distance_km":      0,
			"waypoints":        session.Waypoints,
		}

		if session.EndedAt != nil {
			sessionData["ended_at"] = session.EndedAt.Format(time.RFC3339)
			sessionData["duration_minutes"] = session.EndedAt.Sub(session.StartedAt).Minutes()
		}

		sessionList = append(sessionList, sessionData)

		// Combine waypoints
		combinedWaypoints = append(combinedWaypoints, session.Waypoints...)

		// Aggregate stats if session has stats stored
		// Note: You may need to store stats separately in the session model
		// For now, we'll recalculate from waypoints
		if session.EndedAt != nil {
			sessionStats := tracking.CalculateStats(session.Waypoints, session.StartedAt, *session.EndedAt)
			dailyStats.TotalOutdoorMinutes += sessionStats.TotalOutdoorMinutes
			dailyStats.TotalIndoorMinutes += sessionStats.TotalIndoorMinutes
			dailyStats.DistanceKm += sessionStats.DistanceKm
			dailyStats.WaypointsProcessed += sessionStats.WaypointsProcessed

			if sessionStats.MaxAQI > dailyStats.MaxAQI {
				dailyStats.MaxAQI = sessionStats.MaxAQI
				dailyStats.WorstAreaName = sessionStats.WorstAreaName
				dailyStats.WorstAreaAQI = sessionStats.WorstAreaAQI
			}
		}
	}

	// Calculate daily averages if there are outdoor waypoints
	if dailyStats.WaypointsProcessed > 0 {
		var outdoorCount int
		var aqiSum, pm25Sum float64
		for _, wp := range combinedWaypoints {
			if wp.Classification == "outdoor" {
				outdoorCount++
				aqiSum += float64(wp.AQI)
				pm25Sum += wp.PM25
			}
		}
		if outdoorCount > 0 {
			dailyStats.AverageOutdoorAQI = aqiSum / float64(outdoorCount)
			dailyStats.AverageOutdoorPM25 = pm25Sum / float64(outdoorCount)
		}
	}

	response := map[string]any{
		"date":               dateStr,
		"sessions":           sessionList,
		"combined_waypoints": combinedWaypoints,
		"daily_stats": map[string]any{
			"total_outdoor_minutes": dailyStats.TotalOutdoorMinutes,
			"total_indoor_minutes":  dailyStats.TotalIndoorMinutes,
			"average_outdoor_aqi":   dailyStats.AverageOutdoorAQI,
			"average_outdoor_pm25":  dailyStats.AverageOutdoorPM25,
			"max_aqi":               dailyStats.MaxAQI,
			"worst_area_name":       dailyStats.WorstAreaName,
			"worst_area_aqi":        dailyStats.WorstAreaAQI,
			"waypoints_processed":   dailyStats.WaypointsProcessed,
		},
	}

	httpx.OK(w, http.StatusOK, response)
}

// Helper functions

func validateLocation(loc tracking.Location) error {
	if loc.Latitude < -90 || loc.Latitude > 90 {
		return fmt.Errorf("latitude must be between -90 and 90")
	}
	if loc.Longitude < -180 || loc.Longitude > 180 {
		return fmt.Errorf("longitude must be between -180 and 180")
	}
	if loc.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}
	return nil
}

func validateWaypoint(wp tracking.Waypoint) error {
	if wp.Latitude < -90 || wp.Latitude > 90 {
		return fmt.Errorf("waypoint latitude must be between -90 and 90")
	}
	if wp.Longitude < -180 || wp.Longitude > 180 {
		return fmt.Errorf("waypoint longitude must be between -180 and 180")
	}
	if wp.Timestamp.IsZero() {
		return fmt.Errorf("waypoint timestamp is required")
	}
	return nil
}

func resolveUserIDFromAuth(r *http.Request, c *firebase.Clients, fallback string) string {
	if uid := middleware.UID(r.Context()); uid != "" {
		return uid
	}
	if fallback != "" {
		return fallback
	}
	return "guest-user"
}

// calculateSessionStats computes stats from a session (test helper)
func calculateSessionStats(session *tracking.Session) tracking.SessionStats {
	if len(session.Waypoints) == 0 {
		return tracking.SessionStats{WaypointsProcessed: 0}
	}

	endTime := time.Now().UTC()
	if session.EndedAt != nil {
		endTime = *session.EndedAt
	}

	return tracking.CalculateStats(session.Waypoints, session.StartedAt, endTime)
}
