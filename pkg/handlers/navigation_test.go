package handlers

import (
	"testing"

	"aurabackend/pkg/navigation"
	"aurabackend/pkg/zones"
)

func TestExtractStepsFromRoute(t *testing.T) {
	origin := navigation.Coordinate{Latitude: -6.9932, Longitude: 110.4203}
	dest := navigation.Coordinate{Latitude: -7.0015, Longitude: 110.4128}

	route := &navigation.Route{
		Distance: 1200,
		Duration: 180,
		Legs: []navigation.RouteLeg{
			{
				Steps: []navigation.RouteLegStep{
					{
						Name:     "Jl. Pandanaran",
						Distance: 700,
						Duration: 100,
						Maneuver: navigation.RouteManeuver{
							Type:     "depart",
							Modifier: "straight",
							Location: []float64{110.4203, -6.9932},
						},
					},
					{
						Name:     "Jl. Pemuda",
						Distance: 500,
						Duration: 80,
						Maneuver: navigation.RouteManeuver{
							Type:     "turn",
							Modifier: "right",
							Location: []float64{110.4160, -6.9960},
						},
					},
					{
						Name:     "",
						Distance: 0,
						Duration: 0,
						Maneuver: navigation.RouteManeuver{
							Type:     "arrive",
							Location: []float64{110.4128, -7.0015},
						},
					},
				},
			},
		},
	}

	steps := extractStepsFromRoute(route, origin, dest, 1200, 180)
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}

	if steps[0]["maneuver"] != "straight" {
		t.Errorf("step 0 maneuver: got %v, want straight", steps[0]["maneuver"])
	}
	if steps[1]["maneuver"] != "turn-right" {
		t.Errorf("step 1 maneuver: got %v, want turn-right", steps[1]["maneuver"])
	}
	if steps[2]["maneuver"] != "arrive" {
		t.Errorf("step 2 maneuver: got %v, want arrive", steps[2]["maneuver"])
	}

	start0 := steps[0]["start_location"].(map[string]float64)
	end0 := steps[0]["end_location"].(map[string]float64)
	if start0["latitude"] != -6.9932 || start0["longitude"] != 110.4203 {
		t.Errorf("unexpected step 0 start_location: %+v", start0)
	}
	if end0["latitude"] != -6.9960 || end0["longitude"] != 110.4160 {
		t.Errorf("unexpected step 0 end_location: %+v", end0)
	}

	// Fallback when route is nil must NOT use (0,0)
	fallback := extractStepsFromRoute(nil, origin, dest, 1200, 180)
	if len(fallback) != 1 {
		t.Fatalf("expected 1 fallback step, got %d", len(fallback))
	}
	fbStart := fallback[0]["start_location"].(map[string]float64)
	if fbStart["latitude"] == 0 && fbStart["longitude"] == 0 {
		t.Errorf("fallback start_location must not be 0,0")
	}
}

func TestEncodePolyline(t *testing.T) {
	// Standard Google Polyline example: (-120.2, 38.5), (-120.95, 40.7), (-126.453, 43.252) in [lon, lat]
	coords := [][]float64{
		{-120.2, 38.5},
		{-120.95, 40.7},
		{-126.453, 43.252},
	}
	got := encodePolyline(coords)
	want := "_p~iF~ps|U_ulLnnqC_mqNvxq`@"
	if got != want {
		t.Errorf("encodePolyline = %q, want %q", got, want)
	}
}

func TestGenerateHealthWarningsAndTraffic(t *testing.T) {
	// Route passing right through Simpang Lima (-6.9893, 110.4229)
	coords := [][]float64{
		{110.4203, -6.9932},
		{110.4229, -6.9893},
		{110.4281, -6.9682},
	}

	allZones := mergeZonesWithSemarangHotspots(nil)
	warnings, traffic := generateHealthWarningsAndTraffic(coords, allZones)
	markers := generateMapMarkers(warnings)

	if len(warnings) == 0 {
		t.Fatalf("expected health warning for Simpang Lima (AQI 132 > 100)")
	}
	if len(traffic) == 0 {
		t.Fatalf("expected traffic segment for Simpang Lima intersection")
	}
	if len(markers) != len(warnings) {
		t.Fatalf("expected %d markers, got %d", len(warnings), len(markers))
	}

	wType := warnings[0]["type"].(string)
	if wType != "heavy_traffic" && wType != "moderate_traffic" && wType != "high_pollution" && wType != "disease_hotspot" {
		t.Errorf("unexpected warning type: %s", wType)
	}

	tSev := traffic[0]["severity"].(string)
	if tSev != "light" && tSev != "moderate" && tSev != "heavy" {
		t.Errorf("unexpected traffic severity: %s", tSev)
	}

	// Clean zone with AQI <= 100 should return non-nil empty slices
	cleanZones := []zones.Zone{
		{
			ID:           "clean_zone",
			Name:         "Taman Kota",
			Category:     "park",
			Latitude:     -6.9893,
			Longitude:    110.4229,
			RadiusMeters: 300,
			LastSnapshot: &zones.ZoneAirSnapshot{USAQI: 42, PM25: 10.5},
		},
	}
	cleanWarnings, cleanTraffic := generateHealthWarningsAndTraffic(coords, cleanZones)
	if cleanWarnings == nil || len(cleanWarnings) != 0 {
		t.Errorf("expected empty non-nil warnings slice, got %#v", cleanWarnings)
	}
	if cleanTraffic == nil || len(cleanTraffic) != 0 {
		t.Errorf("expected empty non-nil traffic slice, got %#v", cleanTraffic)
	}
}
