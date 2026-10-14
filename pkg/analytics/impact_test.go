package analytics

import (
	"testing"
	"time"

	"aurabackend/pkg/openmeteo"
	"aurabackend/pkg/zones"
)

func TestAnalyze7DaysForecast(t *testing.T) {
	lat, lon := -6.2088, 106.8229
	zone := &zones.Zone{
		ID:                  "test_zone",
		Name:                "Test Zone Sudirman",
		CongestionIndex:     80.0,
		CongestionBaseLevel: "Padat",
		Category:            "traffic_corridor",
		HistoricalTrafficFactor: map[string]float64{
			"Monday": 1.2,
			"Friday": 1.3,
		},
	}

	snapshot := &openmeteo.EnvironmentalSnapshot{
		FetchedAt: time.Now().UTC(),
		AirQuality: openmeteo.CurrentAirQuality{
			PM25:            45.0,
			PM10:            65.0,
			CarbonMonoxide:  800.0,
			NitrogenDioxide: 30.0,
			USAQI:           125,
		},
		Weather: openmeteo.WeatherForecastResponse{
			Latitude:  lat,
			Longitude: lon,
			Daily: openmeteo.DailyWeatherForecast{
				Time:                 []string{"2026-10-15", "2026-10-16", "2026-10-17", "2026-10-18", "2026-10-19", "2026-10-20", "2026-10-21"},
				RelativeHumidityMean: []float64{80, 85, 75, 70, 65, 88, 72},
				WindSpeedMax:         []float64{12, 5, 18, 10, 8, 4, 15},
				PrecipitationSum:     []float64{0, 0, 8.5, 0, 1.2, 0, 0},
			},
		},
	}

	analysis := Analyze7DaysForecast("Test Sudirman", lat, lon, zone, snapshot)
	if analysis == nil {
		t.Fatal("expected analysis result, got nil")
	}

	if len(analysis.DailyForecast) != 7 {
		t.Fatalf("expected 7 days forecast, got %d", len(analysis.DailyForecast))
	}

	if analysis.AverageRiskScore <= 0 || analysis.AverageRiskScore > 100 {
		t.Errorf("invalid average risk score: %f", analysis.AverageRiskScore)
	}

	validStatuses := map[string]bool{"Aman": true, "Waspada": true, "Berbahaya": true}
	if !validStatuses[analysis.OverallWeeklyStatus] {
		t.Errorf("unexpected overall status: %s", analysis.OverallWeeklyStatus)
	}

	for _, day := range analysis.DailyForecast {
		if !validStatuses[day.ImpactLevel] {
			t.Errorf("unexpected daily status: %s", day.ImpactLevel)
		}
		if day.TrafficCongestionIdx < 0 || day.TrafficCongestionIdx > 100 {
			t.Errorf("invalid congestion index: %f", day.TrafficCongestionIdx)
		}
		if len(day.Recommendations) == 0 {
			t.Errorf("missing recommendations for %s", day.Date)
		}
	}
}
