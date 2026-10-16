package mobility

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

func TestLiveGroqLlama33(t *testing.T) {
	_ = godotenv.Load("../../.env")

	groqKey := strings.TrimSpace(os.Getenv("GROQ_AURA2"))
	if groqKey == "" {
		groqKey = strings.TrimSpace(os.Getenv("GROQ_API_KEY"))
	}

	if groqKey == "" {
		t.Skip("GROQ_AURA2 or GROQ_API_KEY not found in environment, skipping live test")
		return
	}

	client := NewGroqClient(groqKey)

	stats := ExposureStats{
		TotalIndoorMinutes:  240,
		TotalOutdoorMinutes: 50,
		AverageOutdoorAQI:   135,
		AverageOutdoorPM25:  49.5,
		MaxAQI:              155,
		WorstAreaName:       "Simpang Lima Semarang",
		WorstAreaAQI:        155,
		WorstAreaTimestamp:  "2026-10-15T08:30:00Z",
		WaypointsProcessed:  10,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	diagnosis, err := client.AnalyzeExposureWithGroq(ctx, stats)
	if err != nil {
		t.Fatalf("AnalyzeExposureWithGroq error: %v", err)
	}

	t.Logf("Groq Impact Score     : %.1f", diagnosis.ImpactScore)
	t.Logf("Groq Risk Level       : %s", diagnosis.RiskLevel)
	t.Logf("Groq Summary          : %s", diagnosis.Summary)
	t.Logf("Groq Doctor Diagnosis : %s", diagnosis.DoctorDiagnosis)
	t.Logf("Groq Recommendations  : %d items", len(diagnosis.Recommendations))
	for i, r := range diagnosis.Recommendations {
		t.Logf("  [%d] %s", i+1, r)
	}

	if diagnosis.ImpactScore <= 0 {
		t.Errorf("expected positive impact score")
	}
	if diagnosis.RiskLevel == "" {
		t.Errorf("expected non-empty risk level")
	}
	if diagnosis.DoctorDiagnosis == "" {
		t.Errorf("expected non-empty doctor diagnosis")
	}
	if len(diagnosis.Recommendations) == 0 {
		t.Errorf("expected recommendations")
	}
}
