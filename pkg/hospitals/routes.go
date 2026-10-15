package hospitals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RouteStep represents a turn-by-turn navigation step.
type RouteStep struct {
	Name     string  `json:"name"`
	Distance float64 `json:"distance"`
	Duration float64 `json:"duration"`
	Modifier string  `json:"modifier"`
	Type     string  `json:"type"`
}

// RouteResult holds the parsed navigation response matching the mobile app spec.
type RouteResult struct {
	DistanceMeters  float64     `json:"distance_meters"`
	DurationSeconds float64     `json:"duration_seconds"`
	Coordinates     [][]float64 `json:"coordinates"`
	Steps           []RouteStep `json:"steps"`
}

// osrmResponse models the raw JSON from the OSRM routing engine.
type osrmResponse struct {
	Code   string `json:"code"`
	Routes []struct {
		Distance float64 `json:"distance"`
		Duration float64 `json:"duration"`
		Geometry struct {
			Coordinates [][]float64 `json:"coordinates"`
			Type        string      `json:"type"`
		} `json:"geometry"`
		Legs []struct {
			Steps []struct {
				Name     string  `json:"name"`
				Distance float64 `json:"distance"`
				Duration float64 `json:"duration"`
				Maneuver struct {
					Modifier string `json:"modifier"`
					Type     string `json:"type"`
				} `json:"maneuver"`
			} `json:"steps"`
		} `json:"legs"`
	} `json:"routes"`
}

// RouteClient handles requests to OSRM routing engines.
type RouteClient struct {
	httpClient *http.Client
	baseURL    string
}

// NewRouteClient initializes a new RouteClient.
func NewRouteClient() *RouteClient {
	return &RouteClient{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		baseURL: "https://routing.openstreetmap.de/routed-car/route/v1/driving",
	}
}

// FetchRoute calls OSRM driving engine with origin and destination coordinates.
// Format required by OSRM: {lon1},{lat1};{lon2},{lat2}
func (rc *RouteClient) FetchRoute(ctx context.Context, originLat, originLon, destLat, destLon float64) (*RouteResult, error) {
	endpoints := []string{
		fmt.Sprintf("%s/%f,%f;%f,%f?overview=full&geometries=geojson&steps=true",
			rc.baseURL, originLon, originLat, destLon, destLat),
		// Fallback to project-osrm router if openstreetmap.de is rate-limited or busy
		fmt.Sprintf("https://router.project-osrm.org/route/v1/driving/%f,%f;%f,%f?overview=full&geometries=geojson&steps=true",
			originLon, originLat, destLon, destLat),
	}

	var lastErr error
	for _, url := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "AuraBackend-Routing/1.0 (contact: team@aura.app)")

		resp, err := rc.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("osrm returned status %d: %s", resp.StatusCode, string(body))
			continue
		}

		var osrmResp osrmResponse
		if err := json.Unmarshal(body, &osrmResp); err != nil {
			lastErr = fmt.Errorf("unmarshal osrm response: %w", err)
			continue
		}

		if osrmResp.Code != "Ok" || len(osrmResp.Routes) == 0 {
			lastErr = fmt.Errorf("osrm code: %s, routes found: %d", osrmResp.Code, len(osrmResp.Routes))
			continue
		}

		route := osrmResp.Routes[0]
		res := &RouteResult{
			DistanceMeters:  route.Distance,
			DurationSeconds: route.Duration,
			Coordinates:     route.Geometry.Coordinates,
			Steps:           make([]RouteStep, 0),
		}

		for _, leg := range route.Legs {
			for _, step := range leg.Steps {
				res.Steps = append(res.Steps, RouteStep{
					Name:     step.Name,
					Distance: step.Distance,
					Duration: step.Duration,
					Modifier: step.Maneuver.Modifier,
					Type:     step.Maneuver.Type,
				})
			}
		}

		return res, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("route calculation failed: %w", lastErr)
	}
	return nil, errors.New("route calculation failed: no response from routing engines")
}
