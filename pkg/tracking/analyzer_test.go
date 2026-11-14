package tracking

import (
	"math"
	"testing"
	"time"
)

func TestHaversineDistance(t *testing.T) {
	// Semarang city center to Kaligawe (approximately 5-6 km)
	lat1, lon1 := -6.9932, 110.4203
	lat2, lon2 := -6.9587, 110.4578

	distance := haversineDistance(lat1, lon1, lat2, lon2)

	// Should be around 5-6 km
	if distance < 4 || distance > 8 {
		t.Errorf("Expected distance ~5-6 km, got %f km", distance)
	}
}

func TestClassifyWaypoints(t *testing.T) {
	now := time.Now().UTC()

	// Create waypoints: first at a location, then move 100m away, then stay for 30 min
	waypoints := []Waypoint{
		{
			Latitude:  -6.9932,
			Longitude: 110.4203,
			Timestamp: now,
		},
		{
			Latitude:  -6.9932,
			Longitude: 110.4203,
			Timestamp: now.Add(30 * time.Minute), // Stayed 30 min at same location
		},
		{
			Latitude:  -6.9940,
			Longitude: 110.4210,
			Timestamp: now.Add(31 * time.Minute), // Moved 1 km away
		},
	}

	analyzer := NewAnalyzer(nil)
	analyzer.classifyWaypoints(waypoints)

	// First waypoint should be outdoor (no previous reference)
	if waypoints[0].Classification != "outdoor" {
		t.Errorf("First waypoint should be outdoor, got %s", waypoints[0].Classification)
	}

	// Second waypoint should be indoor (stayed 30 min at same location)
	if waypoints[1].Classification != "indoor" {
		t.Errorf("Second waypoint should be indoor, got %s", waypoints[1].Classification)
	}

	// Third waypoint should be outdoor (moved to new location)
	if waypoints[2].Classification != "outdoor" {
		t.Errorf("Third waypoint should be outdoor, got %s", waypoints[2].Classification)
	}
}

func TestCalculateStats(t *testing.T) {
	now := time.Now().UTC()

	waypoints := []Waypoint{
		{
			Latitude:       -6.9932,
			Longitude:      110.4203,
			Timestamp:      now,
			Classification: "outdoor",
			AQI:            100,
			PM25:           50,
			ZoneName:       "Zone A",
		},
		{
			Latitude:       -6.9940,
			Longitude:      110.4210,
			Timestamp:      now.Add(10 * time.Minute),
			Classification: "outdoor",
			AQI:            150,
			PM25:           80,
			ZoneName:       "Zone B",
		},
		{
			Latitude:       -6.9950,
			Longitude:      110.4220,
			Timestamp:      now.Add(20 * time.Minute),
			Classification: "indoor",
			AQI:            50,
			PM25:           20,
			ZoneName:       "Indoor",
		},
	}

	startedAt := now
	endedAt := now.Add(30 * time.Minute)

	stats := CalculateStats(waypoints, startedAt, endedAt)

	// Check basic calculations
	if stats.WaypointsProcessed != 3 {
		t.Errorf("Expected 3 waypoints processed, got %d", stats.WaypointsProcessed)
	}

	if stats.DurationMinutes != 30 {
		t.Errorf("Expected 30 minutes duration, got %f", stats.DurationMinutes)
	}

	if stats.MaxAQI != 150 {
		t.Errorf("Expected max AQI 150, got %d", stats.MaxAQI)
	}

	if stats.WorstAreaName != "Zone B" {
		t.Errorf("Expected worst area Zone B, got %s", stats.WorstAreaName)
	}

	// Average AQI should be (100+150)/2 = 125
	if math.Abs(stats.AverageOutdoorAQI-125) > 1 {
		t.Errorf("Expected average AQI ~125, got %f", stats.AverageOutdoorAQI)
	}
}

func TestGenerateHealthAnalysis(t *testing.T) {
	tests := []struct {
		name          string
		stats         SessionStats
		expectedLevel string
		minScore      float64
		maxScore      float64
	}{
		{
			name: "Low exposure",
			stats: SessionStats{
				TotalOutdoorMinutes: 30,
				AverageOutdoorAQI:   50,
				MaxAQI:              60,
				WorstAreaName:       "Clean Area",
				WaypointsProcessed:  10,
			},
			expectedLevel: "Rendah",
			minScore:      10,
			maxScore:      37,
		},
		{
			name: "Medium exposure",
			stats: SessionStats{
				TotalOutdoorMinutes: 90,
				AverageOutdoorAQI:   120,
				MaxAQI:              140,
				WorstAreaName:       "Medium Zone",
				WaypointsProcessed:  20,
			},
			expectedLevel: "Sedang",
			minScore:      38,
			maxScore:      67,
		},
		{
			name: "High exposure",
			stats: SessionStats{
				TotalOutdoorMinutes: 180,
				AverageOutdoorAQI:   180,
				MaxAQI:              200,
				WorstAreaName:       "Industrial Zone",
				WaypointsProcessed:  40,
			},
			expectedLevel: "Tinggi",
			minScore:      68,
			maxScore:      100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := GenerateHealthAnalysis(tt.stats)

			if analysis.RiskLevel != tt.expectedLevel {
				t.Errorf("Expected risk level %s, got %s", tt.expectedLevel, analysis.RiskLevel)
			}

			if analysis.ImpactScore < tt.minScore || analysis.ImpactScore > tt.maxScore {
				t.Errorf("Expected score between %f and %f, got %f", tt.minScore, tt.maxScore, analysis.ImpactScore)
			}

			if len(analysis.Recommendations) == 0 {
				t.Errorf("Expected at least one recommendation")
			}

			if analysis.Summary == "" {
				t.Errorf("Expected non-empty summary")
			}
		})
	}
}
