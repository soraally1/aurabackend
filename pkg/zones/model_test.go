package zones

import (
	"testing"

	"aurabackend/pkg/geoutils"
)

func TestZoneModelAndGeohash(t *testing.T) {
	z := Zone{
		ID:                  "zone_test_1",
		Name:                "Test Intersection",
		Latitude:            -6.2088,
		Longitude:           106.8229,
		CongestionIndex:     85.0,
		CongestionBaseLevel: "Padat",
	}

	z.Geohash = geoutils.EncodeGeohash(z.Latitude, z.Longitude, 6)
	if len(z.Geohash) != 6 {
		t.Fatalf("expected geohash length 6, got %s", z.Geohash)
	}

	dist := geoutils.HaversineDistance(-6.2088, 106.8229, -6.2100, 106.8250)
	if dist <= 0 || dist > 1.0 {
		t.Errorf("expected close distance, got %f km", dist)
	}
}
