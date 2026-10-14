// Package handlers provides HTTP endpoints for environmental zones and impact forecasts.
package handlers

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"aurabackend/pkg/analytics"
	"aurabackend/pkg/firebase"
	"aurabackend/pkg/httpx"
	"aurabackend/pkg/openmeteo"
	"aurabackend/pkg/zones"
)

var openMeteoClient = openmeteo.NewClient()

// NearbyZones handles GET /api/zones/nearby.
// Query params:
//   - lat: user latitude (required)
//   - lon: user longitude (required)
//   - radius: search radius in km (optional, default: 5.0, max: 50.0)
func NearbyZones(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	if r.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	lat, lon, radius, err := parseNearbyQuery(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	ctx := r.Context()
	store := zones.NewStore(c.Firestore, c.Config.ZonesCollection)

	// Retrieve zones within radius
	matched, err := store.FindNearby(ctx, lat, lon, radius)
	if err != nil {
		// Log and continue with empty matches if collection empty or query failed
		matched = []zones.NearbyZoneResult{}
	}

	// Fetch current real-time environmental data for the user coordinate
	envSnapshot, _ := openMeteoClient.FetchCombinedEnvironment(ctx, lat, lon)

	// Enrich matched zones with live air metrics and impact calculations
	for i := range matched {
		z := matched[i].Zone
		matched[i].AirQuality = envSnapshot.AirQuality

		// Calculate quick single-day impact score for this hotspot
		trafficWeight := z.CongestionIndex * 0.35
		pm25Weight := (envSnapshot.AirQuality.PM25 / 75.0) * 45.0
		if pm25Weight > 45.0 {
			pm25Weight = 45.0
		}
		rawScore := math.Min(100.0, trafficWeight+pm25Weight+10.0)

		matched[i].ImpactScore = math.Round(rawScore*10) / 10
		if rawScore < 38.0 {
			matched[i].ImpactLevel = "Aman"
		} else if rawScore < 68.0 {
			matched[i].ImpactLevel = "Waspada"
		} else {
			matched[i].ImpactLevel = "Berbahaya"
		}
	}

	resp := map[string]any{
		"user_coordinates": map[string]float64{
			"latitude":  lat,
			"longitude": lon,
		},
		"search_radius_km":   radius,
		"zones_found_count":  len(matched),
		"nearby_zones":       matched,
		"local_environment":  envSnapshot,
		"summary_advice":     generateNearbySummaryAdvice(len(matched), envSnapshot),
	}

	httpx.OK(w, http.StatusOK, resp)
}

// ZoneImpactAnalysis handles GET /api/zones/analysis.
// Query params:
//   - zone_id: specific zone ID (optional if lat & lon provided)
//   - lat: latitude (optional if zone_id provided)
//   - lon: longitude (optional if zone_id provided)
//   - name: friendly location name (optional)
func ZoneImpactAnalysis(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	q := r.URL.Query()
	zoneID := strings.TrimSpace(q.Get("zone_id"))
	latStr := strings.TrimSpace(q.Get("lat"))
	lonStr := strings.TrimSpace(q.Get("lon"))
	name := strings.TrimSpace(q.Get("name"))

	ctx := r.Context()
	store := zones.NewStore(c.Firestore, c.Config.ZonesCollection)

	var targetZone *zones.Zone
	var lat, lon float64
	var err error

	if zoneID != "" {
		targetZone, err = store.Get(ctx, zoneID)
		if err != nil {
			httpx.Error(w, http.StatusNotFound, "ZONE_NOT_FOUND", fmt.Sprintf("Zone with ID '%s' not found", zoneID))
			return
		}
		lat = targetZone.Latitude
		lon = targetZone.Longitude
		if name == "" {
			name = targetZone.Name
		}
	} else {
		if latStr == "" || lonStr == "" {
			httpx.Error(w, http.StatusBadRequest, "MISSING_COORDINATES", "Either zone_id or lat and lon parameters must be provided")
			return
		}

		lat, err = strconv.ParseFloat(latStr, 64)
		if err != nil || lat < -90 || lat > 90 {
			httpx.Error(w, http.StatusBadRequest, "INVALID_LATITUDE", "Latitude must be between -90 and 90")
			return
		}

		lon, err = strconv.ParseFloat(lonStr, 64)
		if err != nil || lon < -180 || lon > 180 {
			httpx.Error(w, http.StatusBadRequest, "INVALID_LONGITUDE", "Longitude must be between -180 and 180")
			return
		}

		if name == "" {
			name = fmt.Sprintf("Koordinat (%.4f, %.4f)", lat, lon)
		}
	}

	// Fetch 7-day environmental forecast
	envSnapshot, err := openMeteoClient.FetchCombinedEnvironment(ctx, lat, lon)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "OPENMETEO_ERROR", "Failed to retrieve environmental data")
		return
	}

	// Run 1-week predictive impact analysis
	weeklyAnalysis := analytics.Analyze7DaysForecast(name, lat, lon, targetZone, envSnapshot)

	httpx.OK(w, http.StatusOK, weeklyAnalysis)
}

// SeedZones handles POST /api/zones/seed to populate standard Indonesian hotspots.
func SeedZones(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	if r.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	store := zones.NewStore(c.Firestore, c.Config.ZonesCollection)
	if err := store.SeedDefaultZones(r.Context()); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "SEED_FAILED", fmt.Sprintf("Failed to seed zones: %v", err))
		return
	}

	httpx.OK(w, http.StatusOK, map[string]string{
		"message": "Default environmental and traffic hotspot zones seeded successfully",
	})
}

// CreateZone handles POST /api/zones to register custom hotspot zones.
func CreateZone(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	if r.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	var req zones.Zone
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_PAYLOAD", err.Error())
		return
	}

	if strings.TrimSpace(req.ID) == "" {
		httpx.Error(w, http.StatusBadRequest, "VALIDATION_ERROR", "Field 'id' is required")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		httpx.Error(w, http.StatusBadRequest, "VALIDATION_ERROR", "Field 'name' is required")
		return
	}
	if req.Latitude < -90 || req.Latitude > 90 || req.Longitude < -180 || req.Longitude > 180 {
		httpx.Error(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid latitude or longitude")
		return
	}

	store := zones.NewStore(c.Firestore, c.Config.ZonesCollection)
	if err := store.Create(r.Context(), &req); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "CREATE_FAILED", fmt.Sprintf("Failed to create zone: %v", err))
		return
	}

	httpx.OK(w, http.StatusCreated, req)
}

func parseNearbyQuery(r *http.Request) (float64, float64, float64, error) {
	q := r.URL.Query()
	latStr := strings.TrimSpace(q.Get("lat"))
	lonStr := strings.TrimSpace(q.Get("lon"))
	radStr := strings.TrimSpace(q.Get("radius"))

	if latStr == "" || lonStr == "" {
		return 0, 0, 0, fmt.Errorf("lat and lon query parameters are required")
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil || lat < -90 || lat > 90 {
		return 0, 0, 0, fmt.Errorf("lat must be a valid latitude between -90 and 90")
	}

	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil || lon < -180 || lon > 180 {
		return 0, 0, 0, fmt.Errorf("lon must be a valid longitude between -180 and 180")
	}

	radius := 5.0 // default 5 km
	if radStr != "" {
		parsedRadius, err := strconv.ParseFloat(radStr, 64)
		if err != nil || parsedRadius <= 0 || parsedRadius > 50 {
			return 0, 0, 0, fmt.Errorf("radius must be a positive number up to 50 km")
		}
		radius = parsedRadius
	}

	return lat, lon, radius, nil
}

func generateNearbySummaryAdvice(zonesCount int, env *openmeteo.EnvironmentalSnapshot) string {
	if zonesCount > 0 {
		return fmt.Sprintf(
			"Terdeteksi %d zona hotspot kemacetan/polusi dalam radius pencarian. AQI setempat saat ini: %d (PM2.5: %.1f ug/m3). Disarankan memantau jam sibuk sebelum melintasi koridor.",
			zonesCount, env.AirQuality.USAQI, env.AirQuality.PM25,
		)
	}
	return fmt.Sprintf(
		"Tidak ada zona titik kemacetan kritis dalam radius langsung. Kualitas udara setempat berada pada AQI: %d (PM2.5: %.1f ug/m3).",
		env.AirQuality.USAQI, env.AirQuality.PM25,
	)
}
