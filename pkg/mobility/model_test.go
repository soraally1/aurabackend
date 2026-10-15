package mobility

import (
	"encoding/json"
	"testing"
)

func TestWaypointFlexibleUnmarshal(t *testing.T) {
	jsonPayload := `{
		"lat": -6.9934,
		"lon": 110.4087,
		"timestamp": "2026-10-15T08:30:00Z",
		"duration_minutes": 25,
		"is_stay": true
	}`

	var wp Waypoint
	if err := json.Unmarshal([]byte(jsonPayload), &wp); err != nil {
		t.Fatalf("failed to unmarshal waypoint: %v", err)
	}

	if wp.Latitude != -6.9934 {
		t.Errorf("expected latitude -6.9934, got %f", wp.Latitude)
	}
	if wp.Longitude != 110.4087 {
		t.Errorf("expected longitude 110.4087, got %f", wp.Longitude)
	}
	if wp.DurationMinutes != 25 {
		t.Errorf("expected duration 25, got %f", wp.DurationMinutes)
	}
	if !wp.IsStay {
		t.Errorf("expected is_stay to be true")
	}
}

func TestWaypointAlternativeFieldNames(t *testing.T) {
	jsonPayload := `{
		"latitude": -7.0016,
		"lng": 110.4125,
		"duration": 15
	}`

	var wp Waypoint
	if err := json.Unmarshal([]byte(jsonPayload), &wp); err != nil {
		t.Fatalf("failed to unmarshal waypoint: %v", err)
	}

	if wp.Latitude != -7.0016 {
		t.Errorf("expected latitude -7.0016, got %f", wp.Latitude)
	}
	if wp.Longitude != 110.4125 {
		t.Errorf("expected longitude 110.4125, got %f", wp.Longitude)
	}
	if wp.DurationMinutes != 15 {
		t.Errorf("expected duration 15, got %f", wp.DurationMinutes)
	}
}
