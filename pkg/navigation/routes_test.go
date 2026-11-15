package navigation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchRouteWithOptionsAndCache(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": "Ok",
			"routes": [{
				"distance": 1450.5,
				"duration": 185.2,
				"geometry": {
					"type": "LineString",
					"coordinates": [[110.4203, -6.9932], [110.4150, -6.9970], [110.4128, -7.0015]]
				},
				"legs": [{
					"distance": 1450.5,
					"duration": 185.2,
					"steps": [
						{
							"name": "Jl. Pandanaran",
							"distance": 900.0,
							"duration": 120.0,
							"maneuver": {"location": [110.4203, -6.9932], "type": "depart", "modifier": "straight"}
						},
						{
							"name": "Jl. Dr. Sutomo",
							"distance": 550.5,
							"duration": 65.2,
							"maneuver": {"location": [110.4150, -6.9970], "type": "turn", "modifier": "left"}
						},
						{
							"name": "",
							"distance": 0,
							"duration": 0,
							"maneuver": {"location": [110.4128, -7.0015], "type": "arrive"}
						}
					]
				}]
			}]
		}`))
	}))
	defer srv.Close()

	// Clear cache before test
	routeCacheMu.Lock()
	routeCache = make(map[string]cachedRoute)
	routeCacheMu.Unlock()

	client := &RouteClient{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    srv.URL + "/route/v1/driving",
	}

	origin := Coordinate{Latitude: -6.9932, Longitude: 110.4203}
	dest := Coordinate{Latitude: -7.0015, Longitude: 110.4128}

	res1, err := client.FetchRouteWithOptions(context.Background(), origin, dest, nil, "driving")
	if err != nil {
		t.Fatalf("FetchRouteWithOptions failed: %v", err)
	}
	if len(res1.Routes) != 1 || len(res1.Routes[0].Geometry.Coordinates) != 3 {
		t.Fatalf("unexpected route result: %+v", res1)
	}
	if len(res1.Routes[0].Legs) != 1 || len(res1.Routes[0].Legs[0].Steps) != 3 {
		t.Fatalf("expected 3 steps, got %+v", res1.Routes[0].Legs)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("expected 1 HTTP call before cache, got %d", got)
	}

	// Second call with same inputs should hit 5-minute cache
	res2, err := client.FetchRouteWithOptions(context.Background(), origin, dest, nil, "driving")
	if err != nil {
		t.Fatalf("cached FetchRouteWithOptions failed: %v", err)
	}
	if res2.Routes[0].Distance != 1450.5 {
		t.Fatalf("unexpected cached distance: %v", res2.Routes[0].Distance)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("expected cached response without new HTTP call, got %d hits", got)
	}
}
