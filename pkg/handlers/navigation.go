package handlers

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"aurabackend/pkg/firebase"
	"aurabackend/pkg/httpx"
	"aurabackend/pkg/navigation"
	"aurabackend/pkg/zones"
)

// CalculateRoute handles POST /api/v1/navigation/calculate-route.
// Calculates routes with turn-by-turn steps, traffic data, health warnings, and map markers.
func CalculateRoute(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	if r.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	var req navigation.CalculateRouteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	// Validate origin and destination
	if err := validateCoordinate(req.Origin.Latitude, req.Origin.Longitude, "origin"); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_COORDINATES", err.Error())
		return
	}

	if err := validateCoordinate(req.Destination.Latitude, req.Destination.Longitude, "destination"); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_COORDINATES", err.Error())
		return
	}

	// Validate waypoints if provided
	for i, wp := range req.Waypoints {
		if err := validateCoordinate(wp.Latitude, wp.Longitude, fmt.Sprintf("waypoint %d", i)); err != nil {
			httpx.Error(w, http.StatusBadRequest, "INVALID_COORDINATES", err.Error())
			return
		}
	}

	// Get route from routing service
	routeClient := navigation.NewRouteClient()
	routeData, err := routeClient.FetchRoute(
		r.Context(),
		req.Origin.Latitude,
		req.Origin.Longitude,
		req.Destination.Latitude,
		req.Destination.Longitude,
	)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "NO_ROUTE", fmt.Sprintf("Failed to compute route: %v", err))
		return
	}

	// Extract coordinates from route
	var coordinates [][]float64
	if routeData.Features != nil && len(routeData.Features) > 0 {
		if routeData.Features[0].Geometry != nil {
			for _, coord := range routeData.Features[0].Geometry.Coordinates {
				if len(coord) >= 2 {
					coordinates = append(coordinates, coord)
				}
			}
		}
	}

	// Generate route ID
	routeID := fmt.Sprintf("route_%s_%s_%d", sanitizeCoordinate(req.Origin), sanitizeCoordinate(req.Destination), int(req.Origin.Latitude*1000000))

	// Extract turn-by-turn steps
	steps := extractSteps(routeData)

	// Get zone information for health warnings and traffic
	zoneStore := zones.NewStore(c.Firestore, c.Config.ZonesCollection)
	allZones, err := zoneStore.ListAll(r.Context())
	if err != nil {
		// Continue without zones but log the error
		fmt.Printf("[Navigation] Warning: could not load zones: %v\n", err)
	}

	// Generate health warnings and traffic segments
	healthWarnings, trafficSegments := generateHealthWarningsAndTraffic(coordinates, allZones)

	// Generate map markers
	mapMarkers := generateMapMarkers(healthWarnings)

	// Calculate distance and duration
	distance := calculateDistanceFromCoordinates(coordinates)
	duration := int(distance / 15 * 60) // Rough estimate: assume 15 m/s average speed

	response := map[string]any{
		"route_id":         routeID,
		"distance_meters":  int(distance * 1000),
		"duration_seconds": duration,
		"polyline":         encodePolyline(coordinates),
		"coordinates":      coordinates,
		"steps":            steps,
		"traffic_segments": trafficSegments,
		"health_warnings":  healthWarnings,
		"map_markers":      mapMarkers,
	}

	httpx.OK(w, http.StatusOK, response)
}

// Helper functions

func validateCoordinate(lat, lon float64, label string) error {
	if lat < -90 || lat > 90 {
		return fmt.Errorf("%s latitude must be between -90 and 90", label)
	}
	if lon < -180 || lon > 180 {
		return fmt.Errorf("%s longitude must be between -180 and 180", label)
	}
	return nil
}

func sanitizeCoordinate(c navigation.Coordinate) string {
	latStr := strings.ReplaceAll(strconv.FormatFloat(c.Latitude, 'f', 2, 64), ".", "_")
	lonStr := strings.ReplaceAll(strconv.FormatFloat(c.Longitude, 'f', 2, 64), ".", "_")
	return fmt.Sprintf("%s_%s", latStr, lonStr)
}

// extractSteps converts route features into turn-by-turn instructions
func extractSteps(routeData any) []map[string]any {
	steps := make([]map[string]any, 0)

	// Default: create synthetic steps for demo/fallback
	// In production, this would parse actual routing service responses (OSRM, Mapbox, etc.)
	steps = append(steps, map[string]any{
		"instruction":      "Mulai dari lokasi Anda",
		"distance_meters":  100,
		"duration_seconds": 20,
		"maneuver":         "straight",
		"start_location": map[string]float64{
			"latitude":  0,
			"longitude": 0,
		},
		"end_location": map[string]float64{
			"latitude":  0.001,
			"longitude": 0.001,
		},
	})

	return steps
}

// generateHealthWarningsAndTraffic creates health warnings and traffic segments from zone data
func generateHealthWarningsAndTraffic(coordinates [][]float64, zones []zones.Zone) ([]map[string]any, []map[string]any) {
	var warnings []map[string]any
	var trafficSegments []map[string]any

	warningID := 0
	warningMap := make(map[string]bool)

	// Scan coordinates for proximity to high-pollution zones
	for i, coord := range coordinates {
		if len(coord) < 2 {
			continue
		}

		lat, lon := coord[1], coord[0]

		// Find nearby zones
		for _, zone := range zones {
			// Simple distance check (not exact haversine)
			latDiff := lat - zone.Latitude
			lonDiff := lon - zone.Longitude
			distance := (latDiff*latDiff + lonDiff*lonDiff) * 111.0 // Very rough approximation

			// Only create one warning per zone
			zoneKey := zone.ID
			if distance < 2.0 && !warningMap[zoneKey] { // Within ~2km
				warningMap[zoneKey] = true
				warningID++

				aqi := 50
				pm25 := 20.0
				severity := "low"
				color := "#10B981"

				if zone.LastSnapshot != nil {
					aqi = zone.LastSnapshot.USAQI
					pm25 = zone.LastSnapshot.PM25

					if aqi > 150 {
						severity = "high"
						color = "#EF4444"
					} else if aqi > 100 {
						severity = "medium"
						color = "#F59E0B"
					}
				}

				warnings = append(warnings, map[string]any{
					"warning_id": fmt.Sprintf("hw_%03d", warningID),
					"type":       determineWarningType(zone.Category),
					"severity":   severity,
					"title":      zone.Name,
					"message":    fmt.Sprintf("Area dengan polusi tinggi: %s. AQI: %d", zone.Description, aqi),
					"location": map[string]float64{
						"latitude":  zone.Latitude,
						"longitude": zone.Longitude,
					},
					"aqi":             aqi,
					"pm2_5":           pm25,
					"health_risks":    getHealthRisksForAQI(aqi),
					"recommendations": getRecommendationsForAQI(aqi),
					"icon":            getIconForWarningType(zone.Category),
					"color":           color,
				})

				// Add traffic segment if it's a traffic corridor
				if zone.Category == "traffic_corridor" || zone.Category == "intersection" {
					trafficSegments = append(trafficSegments, map[string]any{
						"segment_index":        i,
						"severity":             zone.CongestionBaseLevel,
						"delay_seconds":        int(zone.CongestionIndex * 2),
						"affected_coordinates": [][]float64{coord},
					})
				}
			}
		}
	}

	return warnings, trafficSegments
}

// generateMapMarkers creates map markers from warnings
func generateMapMarkers(warnings []map[string]any) []map[string]any {
	var markers []map[string]any

	for i, warning := range warnings {
		markers = append(markers, map[string]any{
			"marker_id": fmt.Sprintf("marker_%03d", i+1),
			"category":  warning["type"],
			"location":  warning["location"],
			"title":     warning["title"],
			"icon":      warning["icon"],
			"color":     warning["color"],
		})
	}

	return markers
}

// Helper functions for health data

func determineWarningType(category string) string {
	switch category {
	case "traffic_corridor", "intersection":
		return "heavy_traffic"
	case "industrial_area":
		return "high_pollution"
	default:
		return "pollution_zone"
	}
}

func getHealthRisksForAQI(aqi int) []string {
	risks := make([]string, 0)

	if aqi <= 50 {
		risks = append(risks, "Tidak ada risiko kesehatan")
	} else if aqi <= 100 {
		risks = append(risks, "Iritasi ringan pada kelompok sensitif")
	} else if aqi <= 150 {
		risks = append(risks, "Iritasi mata dan kulit", "Gangguan pernapasan ringan")
	} else if aqi <= 200 {
		risks = append(risks, "ISPA (Infeksi Saluran Pernapasan Akut)", "Eksaserbasi asma")
	} else {
		risks = append(risks, "Risiko kesehatan serius", "Potensi penyakit paru kronis", "Kematian dini pada kelompok berisiko")
	}

	return risks
}

func getRecommendationsForAQI(aqi int) []string {
	recommendations := make([]string, 0)

	if aqi <= 50 {
		recommendations = append(recommendations, "Pertahankan aktivitas normal")
	} else if aqi <= 100 {
		recommendations = append(recommendations, "Kelompok sensitif sebaiknya membatasi aktivitas outdoor")
	} else if aqi <= 150 {
		recommendations = append(recommendations, "Gunakan masker N95", "Tutup jendela kendaraan", "Aktifkan AC recirculation mode")
	} else if aqi <= 200 {
		recommendations = append(recommendations, "Hindari aktivitas outdoor", "Gunakan air purifier di dalam kendaraan", "Gunakan obat inhaler jika diperlukan")
	} else {
		recommendations = append(recommendations, "Tetap berada di dalam ruangan", "Gunakan masker respirator N95 terpasang rapat", "Konsultasi dengan dokter")
	}

	return recommendations
}

func getIconForWarningType(warningType string) string {
	switch warningType {
	case "traffic_corridor", "intersection":
		return "warning"
	case "industrial_area":
		return "factory"
	default:
		return "cloud"
	}
}

// calculateDistance calculates distance between two points using haversine formula (returns meters)
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000.0 // Earth's radius in meters

	dLat := toRadians(lat2 - lat1)
	dLon := toRadians(lon2 - lon1)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRadians(lat1))*math.Cos(toRadians(lat2))*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Asin(math.Sqrt(a))
	return R * c
}

func toRadians(deg float64) float64 {
	return deg * math.Pi / 180.0
}

// encodePolyline creates a simple encoded polyline string
func encodePolyline(coordinates [][]float64) string {
	if len(coordinates) == 0 {
		return ""
	}

	// Simple encoding: just join coordinate pairs
	var parts []string
	for _, coord := range coordinates {
		if len(coord) >= 2 {
			parts = append(parts, fmt.Sprintf("%.6f,%.6f", coord[1], coord[0]))
		}
	}

	return strings.Join(parts, ";")
}

// calculateDistanceFromCoordinates calculates total distance in kilometers from coordinate array
func calculateDistanceFromCoordinates(coordinates [][]float64) float64 {
	if len(coordinates) < 2 {
		return 0
	}

	var total float64
	for i := 1; i < len(coordinates); i++ {
		prev := coordinates[i-1]
		curr := coordinates[i]

		if len(prev) < 2 || len(curr) < 2 {
			continue
		}

		// Use haversine distance (convert from meters to kilometers)
		dist := calculateDistance(prev[1], prev[0], curr[1], curr[0]) / 1000.0
		total += dist
	}

	return total
}
