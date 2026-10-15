package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/joho/godotenv"

	"aurabackend/pkg/firebase"
	"aurabackend/pkg/mobility"
)

func TestLiveMobilityBatchAndDailyAnalysis(t *testing.T) {
	_ = godotenv.Load("../../.env")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fbClients, err := firebase.Get(ctx)
	if err != nil {
		t.Skipf("Skipping live Firebase test: %v", err)
		return
	}

	payload := `{
		"user_id": "test-aura-mobile-user",
		"date": "2026-10-15",
		"waypoints": [
			{
				"lat": -6.9934,
				"lon": 110.4087,
				"timestamp": "2026-10-15T08:00:00Z",
				"duration_minutes": 180,
				"is_stay": true
			},
			{
				"lat": -6.9893,
				"lon": 110.4229,
				"timestamp": "2026-10-15T11:15:00Z",
				"duration_minutes": 15,
				"is_stay": false
			},
			{
				"lat": -6.9587,
				"lon": 110.4578,
				"timestamp": "2026-10-15T11:35:00Z",
				"duration_minutes": 12,
				"is_stay": false
			}
		]
	}`

	// 1. Test POST /api/v1/mobility/batch
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/mobility/batch", bytes.NewBufferString(payload))
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()

	ProcessMobilityBatch(postRec, postReq, fbClients)

	if postRec.Code != http.StatusOK {
		t.Fatalf("ProcessMobilityBatch failed with status %d: %s", postRec.Code, postRec.Body.String())
	}

	var batchResp struct {
		Success bool                   `json:"success"`
		Data    mobility.DailyAnalysis `json:"data"`
	}
	if err := json.Unmarshal(postRec.Body.Bytes(), &batchResp); err != nil {
		t.Fatalf("failed to parse batch response: %v", err)
	}

	if !batchResp.Success {
		t.Fatalf("expected batchResp.Success to be true")
	}
	if batchResp.Data.Stats.TotalIndoorMinutes != 180.0 {
		t.Errorf("expected 180 indoor minutes, got %f", batchResp.Data.Stats.TotalIndoorMinutes)
	}
	if batchResp.Data.Stats.TotalOutdoorMinutes != 27.0 {
		t.Errorf("expected 27 outdoor minutes, got %f", batchResp.Data.Stats.TotalOutdoorMinutes)
	}
	if batchResp.Data.Analysis.DoctorDiagnosis == "" {
		t.Errorf("expected doctor diagnosis")
	}

	// 2. Test GET /api/v1/mobility/daily-analysis
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/mobility/daily-analysis?user_id=test-aura-mobile-user&date=2026-10-15", nil)
	getRec := httptest.NewRecorder()

	GetDailyAnalysis(getRec, getReq, fbClients)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GetDailyAnalysis failed with status %d: %s", getRec.Code, getRec.Body.String())
	}

	var getResp struct {
		Success bool                   `json:"success"`
		Data    mobility.DailyAnalysis `json:"data"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed to parse get response: %v", err)
	}

	if !getResp.Success {
		t.Fatalf("expected getResp.Success to be true")
	}
	if getResp.Data.UserID != "test-aura-mobile-user" {
		t.Errorf("expected user_id test-aura-mobile-user, got %s", getResp.Data.UserID)
	}
}
