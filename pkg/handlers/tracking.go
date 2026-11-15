package handlers

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"aurabackend/pkg/firebase"
	"aurabackend/pkg/geoutils"
	"aurabackend/pkg/httpx"
	"aurabackend/pkg/middleware"
	"aurabackend/pkg/tracking"
	"aurabackend/pkg/users"
	"aurabackend/pkg/zones"
)

var wibZone = time.FixedZone("WIB", 7*3600)

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

	// Verify user ownership (IDOR protection)
	reqUserID := strings.TrimSpace(req.UserID)
	if authUID := middleware.UID(r.Context()); authUID != "" && reqUserID != "" && reqUserID != authUID {
		httpx.Error(w, http.StatusForbidden, "FORBIDDEN", "user_id does not match authenticated user")
		return
	}
	userID := resolveUserIDFromAuth(r, c, reqUserID)

	now := time.Now().UTC()
	sessionID := fmt.Sprintf("session_%s_%d", userID, now.UnixMilli())

	session := &tracking.Session{
		ID:            sessionID,
		UserID:        userID,
		SessionName:   strings.TrimSpace(req.SessionName),
		Status:        "active",
		StartedAt:     now,
		StartLocation: req.StartLocation,
		Waypoints:     []tracking.Waypoint{},
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	store := tracking.NewStore(c.Firestore, c.Config.TrackingCollection)
	if err := store.CreateSession(r.Context(), session); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", fmt.Sprintf("Failed to create session: %v", err))
		return
	}

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

	if len(req.Waypoints) == 0 && !req.IsFinal {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", "waypoints array cannot be empty")
		return
	}

	for _, wp := range req.Waypoints {
		if err := validateWaypoint(wp); err != nil {
			httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
			return
		}
	}

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

	if err := verifySessionOwnership(r, session.UserID, req.UserID); err != nil {
		httpx.Error(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	if session.Status != "active" {
		httpx.Error(w, http.StatusBadRequest, "SESSION_NOT_FOUND", "Session is not active")
		return
	}

	// Allow final empty sync ping gracefully
	if len(req.Waypoints) == 0 {
		response := map[string]any{
			"success":         true,
			"synced_count":    0,
			"session_id":      req.SessionID,
			"total_waypoints": len(session.Waypoints),
			"last_sync_at":    time.Now().UTC().Format(time.RFC3339),
		}
		httpx.OK(w, http.StatusOK, response)
		return
	}

	// Use last stored waypoint as anchor context across incremental batches
	var prevAnchor *tracking.Waypoint
	if len(session.Waypoints) > 0 {
		last := session.Waypoints[len(session.Waypoints)-1]
		prevAnchor = &last
	}

	zoneStore := zones.NewStore(c.Firestore, c.Config.ZonesCollection)
	analyzer := tracking.NewAnalyzer(zoneStore)
	processedWaypoints := analyzer.ProcessWaypointsWithContext(r.Context(), req.Waypoints, prevAnchor)

	totalCount, err := store.AppendWaypoints(r.Context(), req.SessionID, processedWaypoints)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", fmt.Sprintf("Failed to update waypoints: %v", err))
		return
	}

	response := map[string]any{
		"success":         true,
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

	if err := verifySessionOwnership(r, session.UserID, req.UserID); err != nil {
		httpx.Error(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	if session.Status != "active" {
		httpx.Error(w, http.StatusBadRequest, "SESSION_ALREADY_ENDED", "Session already ended")
		return
	}

	stats := tracking.CalculateStats(session.Waypoints, session.StartedAt, req.EndLocation.Timestamp)
	if len(session.Waypoints) == 0 {
		distKm := geoutils.HaversineDistance(
			session.StartLocation.Latitude,
			session.StartLocation.Longitude,
			req.EndLocation.Latitude,
			req.EndLocation.Longitude,
		)
		if distKm <= 50.0 {
			stats.DistanceKm = math.Round(distKm*100) / 100
		}
	}

	// Load optional user profile sensitivity context
	var userCtx *tracking.UserProfileContext
	if c != nil && c.Firestore != nil && session.UserID != "" {
		userStore := users.NewStore(c.Firestore, c.Config.UsersCollection)
		if profile, uErr := userStore.Get(r.Context(), session.UserID); uErr == nil && profile != nil {
			userCtx = &tracking.UserProfileContext{
				Name:         profile.Name,
				Usia:         profile.Usia,
				WaktuDiJalan: profile.WaktuDiJalan,
			}
		}
	}

	groqClient := tracking.NewGroqClient(c.Config.GroqAPIKey)
	analyzer := tracking.NewAnalyzerWithGroq(nil, groqClient)
	analysis := analyzer.GenerateHealthAnalysisWithUser(r.Context(), stats, userCtx)

	if err := store.EndSession(r.Context(), req.SessionID, req.EndLocation, stats, analysis); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", fmt.Sprintf("Failed to end session: %v", err))
		return
	}

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

	// Verify ownership (IDOR protection)
	qUserID := strings.TrimSpace(q.Get("user_id"))
	if authUID := middleware.UID(r.Context()); authUID != "" && qUserID != "" && qUserID != authUID {
		httpx.Error(w, http.StatusForbidden, "FORBIDDEN", "user_id does not match authenticated user")
		return
	}
	userID := resolveUserIDFromAuth(r, c, qUserID)

	dateStr := strings.TrimSpace(q.Get("date"))
	if dateStr == "" {
		dateStr = time.Now().In(wibZone).Format("2006-01-02")
	}

	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_DATE_FORMAT", "Invalid date format, use YYYY-MM-DD")
		return
	}

	store := tracking.NewStore(c.Firestore, c.Config.TrackingCollection)
	sessions, err := store.GetSessionsByDate(r.Context(), userID, dateStr)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}

	combinedWaypoints := make([]tracking.Waypoint, 0)
	dailyStats := tracking.SessionStats{
		WorstAreaName: "-",
	}

	sessionList := make([]map[string]any, 0, len(sessions))
	now := time.Now().UTC()

	for _, session := range sessions {
		waypoints := session.Waypoints
		if waypoints == nil {
			waypoints = make([]tracking.Waypoint, 0)
		}

		var sessionStats tracking.SessionStats
		if session.Stats != nil && session.Stats.WaypointsProcessed == len(waypoints) {
			sessionStats = *session.Stats
		} else {
			endTime := now
			if session.EndedAt != nil {
				endTime = *session.EndedAt
			} else if len(waypoints) > 0 {
				endTime = waypoints[len(waypoints)-1].Timestamp
			}
			sessionStats = tracking.CalculateStats(waypoints, session.StartedAt, endTime)
		}

		sessionData := map[string]any{
			"session_id":       session.ID,
			"user_id":          session.UserID,
			"session_name":     session.SessionName,
			"status":           session.Status,
			"started_at":       session.StartedAt.Format(time.RFC3339),
			"duration_minutes": sessionStats.DurationMinutes,
			"distance_km":      sessionStats.DistanceKm,
			"waypoints":        waypoints,
			"stats": map[string]any{
				"total_outdoor_minutes": sessionStats.TotalOutdoorMinutes,
				"total_indoor_minutes":  sessionStats.TotalIndoorMinutes,
				"average_outdoor_aqi":   sessionStats.AverageOutdoorAQI,
				"average_outdoor_pm25":  sessionStats.AverageOutdoorPM25,
				"max_aqi":               sessionStats.MaxAQI,
				"worst_area_name":       sessionStats.WorstAreaName,
				"worst_area_aqi":        sessionStats.WorstAreaAQI,
				"waypoints_processed":   sessionStats.WaypointsProcessed,
			},
		}

		if session.EndedAt != nil {
			sessionData["ended_at"] = session.EndedAt.Format(time.RFC3339)
		}
		if session.Analysis != nil {
			sessionData["analysis"] = session.Analysis
		}

		sessionList = append(sessionList, sessionData)
		combinedWaypoints = append(combinedWaypoints, waypoints...)

		dailyStats.TotalOutdoorMinutes += sessionStats.TotalOutdoorMinutes
		dailyStats.TotalIndoorMinutes += sessionStats.TotalIndoorMinutes
		dailyStats.DistanceKm += sessionStats.DistanceKm
		dailyStats.WaypointsProcessed += sessionStats.WaypointsProcessed

		if sessionStats.MaxAQI > dailyStats.MaxAQI {
			dailyStats.MaxAQI = sessionStats.MaxAQI
			if sessionStats.WorstAreaName != "" {
				dailyStats.WorstAreaName = sessionStats.WorstAreaName
			}
			dailyStats.WorstAreaAQI = sessionStats.WorstAreaAQI
		}
	}

	// Sort combined waypoints chronologically by timestamp ASC
	sort.Slice(combinedWaypoints, func(i, j int) bool {
		return combinedWaypoints[i].Timestamp.Before(combinedWaypoints[j].Timestamp)
	})

	// Calculate daily outdoor averages from combined waypoints
	if len(combinedWaypoints) > 0 {
		var outdoorCount int
		var aqiSum, pm25Sum float64
		for _, wp := range combinedWaypoints {
			if wp.Classification != "indoor" {
				outdoorCount++
				aqiSum += float64(wp.AQI)
				pm25Sum += wp.PM25
			}
		}
		if outdoorCount > 0 {
			dailyStats.AverageOutdoorAQI = math.Round((aqiSum/float64(outdoorCount))*10) / 10
			dailyStats.AverageOutdoorPM25 = math.Round((pm25Sum/float64(outdoorCount))*10) / 10
		}
	}

	if dailyStats.WorstAreaName == "" {
		dailyStats.WorstAreaName = "-"
	}

	response := map[string]any{
		"date":               dateStr,
		"sessions":           sessionList,
		"combined_waypoints": combinedWaypoints,
		"daily_stats": map[string]any{
			"total_outdoor_minutes": math.Round(dailyStats.TotalOutdoorMinutes*10) / 10,
			"total_indoor_minutes":  math.Round(dailyStats.TotalIndoorMinutes*10) / 10,
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

func verifySessionOwnership(r *http.Request, sessionUserID, reqUserID string) error {
	authUID := middleware.UID(r.Context())
	if authUID == "" {
		return nil
	}
	if sessionUserID != "" && sessionUserID != authUID {
		return fmt.Errorf("session does not belong to authenticated user")
	}
	if trimmed := strings.TrimSpace(reqUserID); trimmed != "" && trimmed != authUID {
		return fmt.Errorf("user_id does not match authenticated user")
	}
	return nil
}

func validateLocation(loc tracking.Location) error {
	if math.IsNaN(loc.Latitude) || math.IsNaN(loc.Longitude) {
		return fmt.Errorf("coordinates must be valid numbers")
	}
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
	if math.IsNaN(wp.Latitude) || math.IsNaN(wp.Longitude) {
		return fmt.Errorf("waypoint coordinates must be valid numbers")
	}
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
		return tracking.SessionStats{WaypointsProcessed: 0, WorstAreaName: "-"}
	}

	endTime := time.Now().UTC()
	if session.EndedAt != nil {
		endTime = *session.EndedAt
	}

	return tracking.CalculateStats(session.Waypoints, session.StartedAt, endTime)
}
