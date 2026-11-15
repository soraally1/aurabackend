package navigation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const routeCacheTTL = 5 * time.Minute

type cachedRoute struct {
	result    *RouteResult
	expiresAt time.Time
}

var (
	routeCacheMu sync.RWMutex
	routeCache   = make(map[string]cachedRoute)
)

// RouteClient handles requests to OSRM routing engines for navigation.
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
	return rc.FetchRouteWithOptions(
		ctx,
		Coordinate{Latitude: originLat, Longitude: originLon},
		Coordinate{Latitude: destLat, Longitude: destLon},
		nil,
		"driving",
	)
}

// FetchRouteWithOptions calls OSRM with optional intermediate waypoints, mode ("driving" or "walking"),
// and caches successful results in-memory for 5 minutes.
func (rc *RouteClient) FetchRouteWithOptions(
	ctx context.Context,
	origin Coordinate,
	dest Coordinate,
	waypoints []Coordinate,
	mode string,
) (*RouteResult, error) {
	if mode == "" {
		mode = "driving"
	}

	coords := make([]string, 0, 2+len(waypoints))
	coords = append(coords, fmt.Sprintf("%.6f,%.6f", origin.Longitude, origin.Latitude))
	for _, wp := range waypoints {
		coords = append(coords, fmt.Sprintf("%.6f,%.6f", wp.Longitude, wp.Latitude))
	}
	coords = append(coords, fmt.Sprintf("%.6f,%.6f", dest.Longitude, dest.Latitude))
	coordPath := strings.Join(coords, ";")

	cacheKey := buildRouteCacheKey(coordPath, mode)
	if cached, ok := getCachedRoute(cacheKey); ok {
		return cached, nil
	}

	endpoints := buildOSRMEndpoints(rc.baseURL, coordPath, mode)

	var lastErr error
	for _, url := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "AuraBackend-Navigation/1.0 (contact: team@aura.app)")

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

		var osrmResp RouteResult
		if err := json.Unmarshal(body, &osrmResp); err != nil {
			lastErr = fmt.Errorf("unmarshal osrm response: %w", err)
			continue
		}

		if osrmResp.Code != "Ok" {
			lastErr = fmt.Errorf("osrm code: %s, message: %s", osrmResp.Code, osrmResp.Message)
			continue
		}

		if len(osrmResp.Routes) == 0 {
			lastErr = fmt.Errorf("osrm returned no routes")
			continue
		}

		setCachedRoute(cacheKey, &osrmResp)
		return &osrmResp, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("route calculation failed: %w", lastErr)
	}
	return nil, errors.New("route calculation failed: no response from routing engines")
}

func buildOSRMEndpoints(drivingBaseURL, coordPath, mode string) []string {
	query := "overview=full&geometries=geojson&steps=true"
	if strings.EqualFold(mode, "walking") {
		return []string{
			fmt.Sprintf("https://routing.openstreetmap.de/routed-foot/route/v1/foot/%s?%s", coordPath, query),
			fmt.Sprintf("https://router.project-osrm.org/route/v1/foot/%s?%s", coordPath, query),
			// Fallback to driving if foot routing server is unavailable
			fmt.Sprintf("%s/%s?%s", drivingBaseURL, coordPath, query),
			fmt.Sprintf("https://router.project-osrm.org/route/v1/driving/%s?%s", coordPath, query),
		}
	}
	return []string{
		fmt.Sprintf("%s/%s?%s", drivingBaseURL, coordPath, query),
		// Fallback to project-osrm router if openstreetmap.de is rate-limited or busy
		fmt.Sprintf("https://router.project-osrm.org/route/v1/driving/%s?%s", coordPath, query),
	}
}

func buildRouteCacheKey(coordPath, mode string) string {
	sum := sha256.Sum256([]byte(mode + "|" + coordPath))
	return hex.EncodeToString(sum[:16])
}

func getCachedRoute(key string) (*RouteResult, bool) {
	routeCacheMu.RLock()
	entry, ok := routeCache[key]
	routeCacheMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.result, true
}

func setCachedRoute(key string, res *RouteResult) {
	routeCacheMu.Lock()
	defer routeCacheMu.Unlock()
	// Prune expired entries occasionally to bound memory usage
	if len(routeCache) > 256 {
		now := time.Now()
		for k, v := range routeCache {
			if now.After(v.expiresAt) {
				delete(routeCache, k)
			}
		}
	}
	routeCache[key] = cachedRoute{
		result:    res,
		expiresAt: time.Now().Add(routeCacheTTL),
	}
}
