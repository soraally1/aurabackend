package geoutils

import (
	"math"
	"testing"
)

func TestHaversineDistance(t *testing.T) {
	// Jakarta Monas to Bundaran HI (~2.3 km)
	monasLat, monasLon := -6.175392, 106.827153
	hiLat, hiLon := -6.195026, 106.823035

	dist := HaversineDistance(monasLat, monasLon, hiLat, hiLon)
	if dist < 2.0 || dist > 2.6 {
		t.Errorf("expected distance around 2.3 km, got %.2f km", dist)
	}

	// Same point distance must be 0
	sameDist := HaversineDistance(monasLat, monasLon, monasLat, monasLon)
	if math.Abs(sameDist) > 1e-6 {
		t.Errorf("expected distance 0, got %f", sameDist)
	}
}

func TestCalculateBoundingBox(t *testing.T) {
	lat, lon := -6.2000, 106.8166
	radiusKm := 5.0

	bbox := CalculateBoundingBox(lat, lon, radiusKm)
	if bbox.MinLat >= bbox.MaxLat || bbox.MinLon >= bbox.MaxLon {
		t.Errorf("invalid bounding box: %+v", bbox)
	}

	if bbox.MinLat > lat || bbox.MaxLat < lat {
		t.Errorf("bounding box does not contain center latitude")
	}
}

func TestEncodeGeohash(t *testing.T) {
	// Coordinates for Jakarta center
	lat, lon := -6.2088, 106.8229
	hash := EncodeGeohash(lat, lon, 6)

	if len(hash) != 6 {
		t.Fatalf("expected length 6, got %d (%s)", len(hash), hash)
	}

	// Range query validation
	qRange := GetGeohashQueryRange("qqgu")
	if qRange.Lower != "qqgu" || qRange.Upper != "qqgu~" {
		t.Errorf("unexpected range: %+v", qRange)
	}
}
