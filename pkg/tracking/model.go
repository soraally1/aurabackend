package tracking

import (
	"time"
)

// Session represents an active or completed tracking session.
type Session struct {
	ID            string     `firestore:"id" json:"id"`
	UserID        string     `firestore:"user_id" json:"user_id"`
	SessionName   string     `firestore:"session_name" json:"session_name"`
	Status        string     `firestore:"status" json:"status"` // "active", "completed"
	StartedAt     time.Time  `firestore:"started_at" json:"started_at"`
	EndedAt       *time.Time `firestore:"ended_at,omitempty" json:"ended_at,omitempty"`
	StartLocation Location   `firestore:"start_location" json:"start_location"`
	EndLocation   *Location  `firestore:"end_location,omitempty" json:"end_location,omitempty"`
	Waypoints     []Waypoint `firestore:"waypoints,omitempty" json:"waypoints,omitempty"`
	CreatedAt     time.Time  `firestore:"created_at" json:"created_at"`
	UpdatedAt     time.Time  `firestore:"updated_at" json:"updated_at"`
}

// Location represents a geographic point with timestamp.
type Location struct {
	Latitude  float64   `firestore:"latitude" json:"latitude"`
	Longitude float64   `firestore:"longitude" json:"longitude"`
	Timestamp time.Time `firestore:"timestamp" json:"timestamp"`
}

// Waypoint represents a GPS point collected during tracking.
type Waypoint struct {
	Latitude       float64   `firestore:"latitude" json:"latitude"`
	Longitude      float64   `firestore:"longitude" json:"longitude"`
	Timestamp      time.Time `firestore:"timestamp" json:"timestamp"`
	AccuracyMeters float64   `firestore:"accuracy_meters,omitempty" json:"accuracy_meters,omitempty"`
	SpeedMps       float64   `firestore:"speed_mps,omitempty" json:"speed_mps,omitempty"`
	Heading        float64   `firestore:"heading,omitempty" json:"heading,omitempty"`
	Classification string    `firestore:"classification,omitempty" json:"classification,omitempty"` // "indoor", "outdoor"
	IsStay         bool      `firestore:"is_stay,omitempty" json:"is_stay,omitempty"`
	ZoneName       string    `firestore:"zone_name,omitempty" json:"zone_name,omitempty"`
	AQI            int       `firestore:"aqi,omitempty" json:"aqi,omitempty"`
	PM25           float64   `firestore:"pm2_5,omitempty" json:"pm2_5,omitempty"`
}

// SessionStats holds aggregated exposure statistics for a completed session.
type SessionStats struct {
	TotalOutdoorMinutes float64 `firestore:"total_outdoor_minutes" json:"total_outdoor_minutes"`
	TotalIndoorMinutes  float64 `firestore:"total_indoor_minutes" json:"total_indoor_minutes"`
	AverageOutdoorAQI   float64 `firestore:"average_outdoor_aqi" json:"average_outdoor_aqi"`
	AverageOutdoorPM25  float64 `firestore:"average_outdoor_pm25" json:"average_outdoor_pm25"`
	MaxAQI              int     `firestore:"max_aqi" json:"max_aqi"`
	WorstAreaName       string  `firestore:"worst_area_name" json:"worst_area_name"`
	WorstAreaAQI        int     `firestore:"worst_area_aqi" json:"worst_area_aqi"`
	WaypointsProcessed  int     `firestore:"waypoints_processed" json:"waypoints_processed"`
	DistanceKm          float64 `firestore:"distance_km" json:"distance_km"`
	DurationMinutes     float64 `firestore:"duration_minutes" json:"duration_minutes"`
}

// HealthAnalysis holds AI-generated health recommendations for a session.
type HealthAnalysis struct {
	ImpactScore     float64  `firestore:"impact_score" json:"impact_score"`
	RiskLevel       string   `firestore:"risk_level" json:"risk_level"`
	Summary         string   `firestore:"summary" json:"summary"`
	Recommendations []string `firestore:"recommendations" json:"recommendations"`
}

// StartSessionRequest represents the request for POST /tracking/start-session.
type StartSessionRequest struct {
	UserID        string   `json:"user_id,omitempty"`
	SessionName   string   `json:"session_name,omitempty"`
	StartLocation Location `json:"start_location"`
}

// SyncWaypointsRequest represents the request for POST /tracking/sync-waypoints.
type SyncWaypointsRequest struct {
	SessionID string     `json:"session_id"`
	UserID    string     `json:"user_id,omitempty"`
	Waypoints []Waypoint `json:"waypoints"`
	IsFinal   bool       `json:"is_final"`
}

// StopSessionRequest represents the request for POST /tracking/stop-session.
type StopSessionRequest struct {
	SessionID   string   `json:"session_id"`
	UserID      string   `json:"user_id,omitempty"`
	EndLocation Location `json:"end_location"`
}
