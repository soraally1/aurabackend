package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoutesValidation(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{
			name:       "missing all params",
			query:      "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing dest_lat",
			query:      "origin_lat=-6.99&origin_lon=110.40&dest_lon=110.42",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid origin_lat format",
			query:      "origin_lat=abc&origin_lon=110.40&dest_lat=-6.98&dest_lon=110.42",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid latitude range",
			query:      "origin_lat=95.0&origin_lon=110.40&dest_lat=-6.98&dest_lon=110.42",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid longitude range",
			query:      "origin_lat=-6.99&origin_lon=210.0&dest_lat=-6.98&dest_lon=110.42",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/routes?"+tc.query, nil)
			rec := httptest.NewRecorder()

			Routes(rec, req, nil)

			if rec.Code != tc.wantStatus {
				t.Errorf("expected status %d, got %d", tc.wantStatus, rec.Code)
			}
		})
	}
}

func TestNearbyHospitalsValidation(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{
			name:       "missing coordinates",
			query:      "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing lon",
			query:      "lat=-6.99",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid lat",
			query:      "lat=invalid&lon=110.40",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/hospitals/nearby?"+tc.query, nil)
			rec := httptest.NewRecorder()

			NearbyHospitals(rec, req, nil)

			if rec.Code != tc.wantStatus {
				t.Errorf("expected status %d, got %d", tc.wantStatus, rec.Code)
			}

			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("expected valid JSON response: %v", err)
			}
			if body["success"] != false {
				t.Errorf("expected success to be false, got %v", body["success"])
			}
		})
	}
}
