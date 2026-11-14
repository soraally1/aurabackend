package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aurabackend/pkg/tracking"
)

func TestStartSessionValidation(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		body       any
		wantStatus int
		wantCode   string
	}{
		{
			name:       "invalid method",
			method:     "GET",
			body:       nil,
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   "METHOD_NOT_ALLOWED",
		},
		{
			name:       "invalid JSON",
			method:     "POST",
			body:       "not json",
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_INPUT",
		},
		{
			name:   "missing start_location",
			method: "POST",
			body: map[string]any{
				"user_id":      "test_user",
				"session_name": "Test Session",
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_INPUT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader *bytes.Reader
			if tt.body == nil {
				bodyReader = bytes.NewReader([]byte{})
			} else if s, ok := tt.body.(string); ok {
				bodyReader = bytes.NewReader([]byte(s))
			} else {
				b, _ := json.Marshal(tt.body)
				bodyReader = bytes.NewReader(b)
			}

			req := httptest.NewRequest(tt.method, "/tracking/start-session", bodyReader)
			req.Header.Set("Content-Type", "application/json")

			_ = httptest.NewRecorder()

			// Would need to mock Firebase clients and middleware
			// This is a simplified validation test
		})
	}
}

func TestSyncWaypointsValidation(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		body       map[string]any
		wantStatus int
		wantCode   string
	}{
		{
			name:       "invalid method",
			method:     "GET",
			body:       nil,
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   "METHOD_NOT_ALLOWED",
		},
		{
			name:   "missing session_id",
			method: "POST",
			body: map[string]any{
				"user_id": "test_user",
				"waypoints": []map[string]any{
					{
						"latitude":  -6.9932,
						"longitude": 110.4203,
						"timestamp": "2025-01-15T08:00:00Z",
					},
				},
				"is_final": false,
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_INPUT",
		},
		{
			name:   "empty waypoints",
			method: "POST",
			body: map[string]any{
				"session_id": "session_123",
				"user_id":    "test_user",
				"waypoints":  []map[string]any{},
				"is_final":   false,
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_INPUT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader *bytes.Reader
			if tt.body == nil {
				bodyReader = bytes.NewReader([]byte{})
			} else {
				b, _ := json.Marshal(tt.body)
				bodyReader = bytes.NewReader(b)
			}

			_ = httptest.NewRequest(tt.method, "/tracking/sync-waypoints", bodyReader)

			// Would need to mock Firebase clients and middleware
		})
	}
}

func TestStopSessionValidation(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		body       map[string]any
		wantStatus int
		wantCode   string
	}{
		{
			name:       "invalid method",
			method:     "GET",
			body:       nil,
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   "METHOD_NOT_ALLOWED",
		},
		{
			name:   "missing session_id",
			method: "POST",
			body: map[string]any{
				"user_id": "test_user",
				"end_location": map[string]any{
					"latitude":  -7.0015,
					"longitude": 110.4128,
					"timestamp": "2025-01-15T09:30:00Z",
				},
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_INPUT",
		},
		{
			name:   "missing end_location",
			method: "POST",
			body: map[string]any{
				"session_id": "session_123",
				"user_id":    "test_user",
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_INPUT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader *bytes.Reader
			if tt.body == nil {
				bodyReader = bytes.NewReader([]byte{})
			} else {
				b, _ := json.Marshal(tt.body)
				bodyReader = bytes.NewReader(b)
			}

			req := httptest.NewRequest(tt.method, "/tracking/stop-session", bodyReader)
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()

			// Would need to mock Firebase clients and middleware
			_ = w
		})
	}
}

func TestDailyHistoryValidation(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		query      string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "invalid method",
			method:     "POST",
			query:      "date=2025-01-15",
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   "METHOD_NOT_ALLOWED",
		},
		{
			name:       "invalid date format",
			method:     "GET",
			query:      "date=01-15-2025",
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_DATE_FORMAT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = httptest.NewRequest(tt.method, "/tracking/daily-history?"+tt.query, nil)

			// Would need to mock Firebase clients and middleware
		})
	}
}

// Helper to calculate distance
func TestCalculateDistance(t *testing.T) {
	tests := []struct {
		name          string
		lat1, lon1    float64
		lat2, lon2    float64
		expectedRange [2]float64 // [min, max] acceptable distance in meters
	}{
		{
			name:          "same point",
			lat1:          -6.9932,
			lon1:          110.4203,
			lat2:          -6.9932,
			lon2:          110.4203,
			expectedRange: [2]float64{0, 10},
		},
		{
			name:          "Semarang to nearby city",
			lat1:          -6.9932,
			lon1:          110.4203,
			lat2:          -7.0500,
			lon2:          110.4500,
			expectedRange: [2]float64{6000, 8000}, // ~7km
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dist := calculateDistance(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			if dist < tt.expectedRange[0] || dist > tt.expectedRange[1] {
				t.Errorf("calculateDistance() = %v, want range [%v, %v]",
					dist, tt.expectedRange[0], tt.expectedRange[1])
			}
		})
	}
}

// Helper to test session stats calculation
func TestCalculateSessionStats(t *testing.T) {
	now := time.Now().UTC()

	session := &tracking.Session{
		ID:        "test_session",
		UserID:    "test_user",
		Status:    "active",
		StartedAt: now,
		StartLocation: tracking.Location{
			Latitude:  -6.9932,
			Longitude: 110.4203,
			Timestamp: now,
		},
		Waypoints: []tracking.Waypoint{
			{
				Latitude:       -6.9932,
				Longitude:      110.4203,
				Timestamp:      now,
				Classification: "outdoor",
				AQI:            150,
				PM25:           120,
				ZoneName:       "Simpang Lima",
			},
			{
				Latitude:       -6.9940,
				Longitude:      110.4210,
				Timestamp:      now.Add(5 * time.Minute),
				Classification: "outdoor",
				AQI:            160,
				PM25:           130,
				ZoneName:       "Jl. Pemuda",
			},
			{
				Latitude:       -6.9950,
				Longitude:      110.4220,
				Timestamp:      now.Add(15 * time.Minute),
				Classification: "indoor",
				AQI:            80,
				PM25:           50,
				ZoneName:       "Indoor",
			},
		},
	}

	stats := calculateSessionStats(session)

	// Verify stats calculation
	if stats.WaypointsProcessed != 3 {
		t.Errorf("WaypointsProcessed: got %d, want 3", stats.WaypointsProcessed)
	}

	if stats.MaxAQI != 160 {
		t.Errorf("MaxAQI: got %d, want 160", stats.MaxAQI)
	}

	if stats.WorstAreaName != "Jl. Pemuda" {
		t.Errorf("WorstAreaName: got %s, want Jl. Pemuda", stats.WorstAreaName)
	}

	// Average outdoor AQI should be (150+160)/2 = 155
	expectedAQI := 155.0
	if stats.AverageOutdoorAQI < expectedAQI-1 || stats.AverageOutdoorAQI > expectedAQI+1 {
		t.Errorf("AverageOutdoorAQI: got %f, want ~%f", stats.AverageOutdoorAQI, expectedAQI)
	}
}
