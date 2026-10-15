package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/joho/godotenv"

	"aurabackend/pkg/firebase"
	"aurabackend/pkg/hospitals"
)

func TestLiveRouteAndNearbyHospitals(t *testing.T) {
	_ = godotenv.Load("../../.env")

	// 1. Test Routing with OSRM
	reqRoute := httptest.NewRequest(http.MethodGet, "/api/v1/routes?origin_lat=-7.001692&origin_lon=110.41257&dest_lat=-6.9934&dest_lon=110.4087", nil)
	recRoute := httptest.NewRecorder()

	Routes(recRoute, reqRoute, nil)

	if recRoute.Code != http.StatusOK {
		t.Fatalf("Routes failed with status %d: %s", recRoute.Code, recRoute.Body.String())
	}

	var routeResp struct {
		Success bool                  `json:"success"`
		Data    hospitals.RouteResult `json:"data"`
	}
	if err := json.Unmarshal(recRoute.Body.Bytes(), &routeResp); err != nil {
		t.Fatalf("failed to decode route response: %v", err)
	}

	if !routeResp.Success {
		t.Fatalf("expected routeResp.Success to be true")
	}
	if routeResp.Data.DistanceMeters <= 0 {
		t.Errorf("expected positive distance, got %f", routeResp.Data.DistanceMeters)
	}
	if len(routeResp.Data.Coordinates) == 0 {
		t.Errorf("expected polyline coordinates, got 0")
	}
	if len(routeResp.Data.Steps) == 0 {
		t.Errorf("expected turn steps, got 0")
	}

	// 2. Test Firestore Nearby Hospitals (if Firebase configured)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fbClients, err := firebase.Get(ctx)
	if err != nil {
		t.Skipf("Skipping live Firestore test: %v", err)
		return
	}

	reqNearby := httptest.NewRequest(http.MethodGet, "/api/v1/hospitals/nearby?lat=-6.9934&lon=110.4087&radius_km=15&limit=5", nil)
	recNearby := httptest.NewRecorder()

	NearbyHospitals(recNearby, reqNearby, fbClients)

	if recNearby.Code != http.StatusOK {
		t.Fatalf("NearbyHospitals failed with status %d: %s", recNearby.Code, recNearby.Body.String())
	}

	var nearbyResp struct {
		Success bool                 `json:"success"`
		Data    []hospitals.Hospital `json:"data"`
	}
	if err := json.Unmarshal(recNearby.Body.Bytes(), &nearbyResp); err != nil {
		t.Fatalf("failed to decode nearby response: %v", err)
	}

	if !nearbyResp.Success {
		t.Fatalf("expected nearbyResp.Success to be true")
	}
	if len(nearbyResp.Data) == 0 {
		t.Fatalf("expected nearby hospitals, got 0")
	}

	// Verify that the closest hospital is near coordinates
	closest := nearbyResp.Data[0]
	t.Logf("Closest hospital: %s (ID: %s, Distance: %.2f km)", closest.Name, closest.ID, closest.DistanceKm)
	if closest.DistanceKm < 0 || closest.DistanceKm > 15.0 {
		t.Errorf("expected distance <= 15.0 km, got %f", closest.DistanceKm)
	}
}
