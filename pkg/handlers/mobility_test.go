package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProcessMobilityBatchValidation(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		wantStatus int
	}{
		{
			name:       "empty body",
			payload:    "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty waypoints array",
			payload:    `{"waypoints": []}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid json",
			payload:    `{"waypoints": [}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/mobility/batch", bytes.NewBufferString(tc.payload))
			rec := httptest.NewRecorder()

			ProcessMobilityBatch(rec, req, nil)

			if rec.Code != tc.wantStatus {
				t.Errorf("expected status %d, got %d", tc.wantStatus, rec.Code)
			}
		})
	}
}

func TestGetDailyAnalysisMethodValidation(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mobility/daily-analysis", nil)
	rec := httptest.NewRecorder()

	GetDailyAnalysis(rec, req, nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 Method Not Allowed, got %d", rec.Code)
	}
}
