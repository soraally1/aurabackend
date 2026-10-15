package mobility

import (
	"context"
	"testing"
	"time"
)

func TestAnalyzerStayVsTravelAndExposure(t *testing.T) {
	groqClient := NewGroqClient("") // Empty key uses clinical fallback
	analyzer := NewAnalyzer(nil, groqClient)

	now := time.Now().UTC()
	waypoints := []Waypoint{
		{
			// Stay point at office (duration >= 20 mins -> Indoor)
			Latitude:        -6.9934,
			Longitude:       110.4087,
			Timestamp:       now,
			DurationMinutes: 120.0,
			IsStay:          false, // Should be classified as indoor due to duration >= 20
		},
		{
			// Travel passing through Simpang Lima (duration 15 mins -> Outdoor)
			Latitude:        -6.9893,
			Longitude:       110.4229,
			Timestamp:       now.Add(125 * time.Minute),
			DurationMinutes: 15.0,
			IsStay:          false,
		},
		{
			// Travel passing through Kaligawe (duration 10 mins -> Outdoor)
			Latitude:        -6.9587,
			Longitude:       110.4578,
			Timestamp:       now.Add(145 * time.Minute),
			DurationMinutes: 10.0,
			IsStay:          false,
		},
		{
			// Stay point at home (duration 45 mins -> Indoor)
			Latitude:        -7.0650,
			Longitude:       110.4180,
			Timestamp:       now.Add(160 * time.Minute),
			DurationMinutes: 45.0,
			IsStay:          true,
		},
	}

	analysis, err := analyzer.Analyze(context.Background(), "test-user-1", "2026-10-15", waypoints)
	if err != nil {
		t.Fatalf("unexpected analyzer error: %v", err)
	}

	// Verify stay classification
	if analysis.Waypoints[0].Classification != "indoor" {
		t.Errorf("expected waypoint 0 to be classified as indoor, got %s", analysis.Waypoints[0].Classification)
	}
	if analysis.Waypoints[1].Classification != "outdoor" {
		t.Errorf("expected waypoint 1 to be classified as outdoor, got %s", analysis.Waypoints[1].Classification)
	}
	if analysis.Waypoints[2].Classification != "outdoor" {
		t.Errorf("expected waypoint 2 to be classified as outdoor, got %s", analysis.Waypoints[2].Classification)
	}
	if analysis.Waypoints[3].Classification != "indoor" {
		t.Errorf("expected waypoint 3 to be classified as indoor, got %s", analysis.Waypoints[3].Classification)
	}

	// Verify stats
	expectedIndoorMin := 165.0  // 120 + 45
	expectedOutdoorMin := 25.0  // 15 + 10

	if analysis.Stats.TotalIndoorMinutes != expectedIndoorMin {
		t.Errorf("expected indoor minutes %f, got %f", expectedIndoorMin, analysis.Stats.TotalIndoorMinutes)
	}
	if analysis.Stats.TotalOutdoorMinutes != expectedOutdoorMin {
		t.Errorf("expected outdoor minutes %f, got %f", expectedOutdoorMin, analysis.Stats.TotalOutdoorMinutes)
	}

	if analysis.Stats.WorstAreaName == "" {
		t.Errorf("expected worst area name to be identified")
	}
	if analysis.Stats.MaxAQI <= 0 {
		t.Errorf("expected positive max AQI, got %d", analysis.Stats.MaxAQI)
	}

	// Verify AI Diagnosis fields
	if analysis.Analysis.ImpactScore <= 0 || analysis.Analysis.ImpactScore > 100 {
		t.Errorf("expected impact score between 0 and 100, got %f", analysis.Analysis.ImpactScore)
	}
	if analysis.Analysis.RiskLevel == "" {
		t.Errorf("expected risk level")
	}
	if analysis.Analysis.DoctorDiagnosis == "" {
		t.Errorf("expected doctor diagnosis text")
	}
	if len(analysis.Analysis.Recommendations) == 0 {
		t.Errorf("expected at least 1 recommendation")
	}
}
