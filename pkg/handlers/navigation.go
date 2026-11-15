package handlers

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"aurabackend/pkg/firebase"
	"aurabackend/pkg/geoutils"
	"aurabackend/pkg/httpx"
	"aurabackend/pkg/navigation"
	"aurabackend/pkg/zones"
)

var routeNavClient = navigation.NewRouteClient()

// semarangFallbackHotspots provides reference pollution/traffic corridors in Semarang
// when Firestore environmental_zones has not yet been seeded with Semarang zones.
var semarangFallbackHotspots = []zones.Zone{
	{
		ID:                  "hw_kaligawe_01",
		Name:                "Zona Polusi Tinggi - Kaligawe",
		Description:         "Kawasan Industri & Jalur Pantura Kaligawe dengan emisi kendaraan berat tinggi",
		Category:            "industrial_area",
		Latitude:            -6.9587,
		Longitude:           110.4578,
		RadiusMeters:        600,
		CongestionBaseLevel: "Sangat Padat",
		CongestionIndex:     88,
		LastSnapshot: &zones.ZoneAirSnapshot{
			USAQI: 156,
			PM25:  62.5,
		},
	},
	{
		ID:                  "hw_simpang_lima_01",
		Name:                "Koridor Padat - Simpang Lima",
		Description:         "Pusat kepadatan lalu lintas Kota Semarang",
		Category:            "intersection",
		Latitude:            -6.9893,
		Longitude:           110.4229,
		RadiusMeters:        450,
		CongestionBaseLevel: "Padat",
		CongestionIndex:     76,
		LastSnapshot: &zones.ZoneAirSnapshot{
			USAQI: 132,
			PM25:  48.5,
		},
	},
	{
		ID:                  "hw_tugu_muda_01",
		Name:                "Simpang Tugu Muda - Jl. Pemuda",
		Description:         "Persimpangan padat kendaraan komuter Jl. Pemuda - Pandanaran",
		Category:            "traffic_corridor",
		Latitude:            -6.9840,
		Longitude:           110.4093,
		RadiusMeters:        400,
		CongestionBaseLevel: "Padat",
		CongestionIndex:     72,
		LastSnapshot: &zones.ZoneAirSnapshot{
			USAQI: 125,
			PM25:  45.0,
		},
	},
	{
		ID:                  "hw_peterongan_01",
		Name:                "Simpang Peterongan - MT Haryono",
		Description:         "Simpang arteri padat lalu lintas Semarang Selatan",
		Category:            "intersection",
		Latitude:            -6.9996,
		Longitude:           110.4320,
		RadiusMeters:        400,
		CongestionBaseLevel: "Padat",
		CongestionIndex:     68,
		LastSnapshot: &zones.ZoneAirSnapshot{
			USAQI: 118,
			PM25:  42.5,
		},
	},
	{
		ID:                  "hw_krapyak_01",
		Name:                "Gerbang Tol Krapyak",
		Description:         "Koridor angkutan berat dan akses tol barat Semarang",
		Category:            "traffic_corridor",
		Latitude:            -6.9850,
		Longitude:           110.3600,
		RadiusMeters:        500,
		CongestionBaseLevel: "Sangat Padat",
		CongestionIndex:     82,
		LastSnapshot: &zones.ZoneAirSnapshot{
			USAQI: 142,
			PM25:  53.0,
		},
	},
}

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
	if req.Origin.Latitude == 0 && req.Origin.Longitude == 0 {
		httpx.Error(w, http.StatusBadRequest, "INVALID_COORDINATES", "origin coordinates cannot be 0,0")
		return
	}

	if err := validateCoordinate(req.Destination.Latitude, req.Destination.Longitude, "destination"); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_COORDINATES", err.Error())
		return
	}
	if req.Destination.Latitude == 0 && req.Destination.Longitude == 0 {
		httpx.Error(w, http.StatusBadRequest, "INVALID_COORDINATES", "destination coordinates cannot be 0,0")
		return
	}

	// Validate waypoints if provided
	for i, wp := range req.Waypoints {
		if err := validateCoordinate(wp.Latitude, wp.Longitude, fmt.Sprintf("waypoint %d", i)); err != nil {
			httpx.Error(w, http.StatusBadRequest, "INVALID_COORDINATES", err.Error())
			return
		}
	}

	// Fetch real road route from OSRM routing engine (cached 5 min)
	routeData, err := routeNavClient.FetchRouteWithOptions(
		r.Context(),
		req.Origin,
		req.Destination,
		req.Waypoints,
		req.Mode,
	)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "NO_ROUTE", fmt.Sprintf("Failed to compute route: %v", err))
		return
	}

	// Extract coordinates, distance, duration, and turn-by-turn steps from OSRM Routes
	coordinates := make([][]float64, 0)
	var distanceMeters int
	var durationSeconds int
	var primaryRoute *navigation.Route

	if len(routeData.Routes) > 0 {
		primaryRoute = &routeData.Routes[0]
		for _, coord := range primaryRoute.Geometry.Coordinates {
			if len(coord) >= 2 && !(coord[0] == 0 && coord[1] == 0) {
				coordinates = append(coordinates, []float64{coord[0], coord[1]})
			}
		}
		distanceMeters = int(math.Round(primaryRoute.Distance))
		durationSeconds = int(math.Round(primaryRoute.Duration))
	} else if len(routeData.Features) > 0 && routeData.Features[0].Geometry != nil {
		for _, coord := range routeData.Features[0].Geometry.Coordinates {
			if len(coord) >= 2 && !(coord[0] == 0 && coord[1] == 0) {
				coordinates = append(coordinates, []float64{coord[0], coord[1]})
			}
		}
	}

	if len(coordinates) < 2 {
		httpx.Error(w, http.StatusBadGateway, "NO_ROUTE", "Routing engine returned insufficient coordinates")
		return
	}

	if distanceMeters <= 0 {
		distanceMeters = int(math.Round(calculateDistanceFromCoordinates(coordinates) * 1000))
	}
	if durationSeconds <= 0 {
		speedMps := 10.0
		if strings.EqualFold(req.Mode, "walking") {
			speedMps = 1.4
		}
		durationSeconds = int(math.Round(float64(distanceMeters) / speedMps))
	}

	routeID := fmt.Sprintf("route_%s_%s_%d", sanitizeCoordinate(req.Origin), sanitizeCoordinate(req.Destination), int(math.Abs(req.Origin.Latitude*1000000)))
	steps := extractStepsFromRoute(primaryRoute, req.Origin, req.Destination, distanceMeters, durationSeconds)

	// Load Firestore zones (non-fatal if empty/unavailable)
	var allZones []zones.Zone
	if c != nil && c.Firestore != nil {
		zoneStore := zones.NewStore(c.Firestore, c.Config.ZonesCollection)
		loaded, zErr := zoneStore.ListAll(r.Context())
		if zErr != nil {
			fmt.Printf("[Navigation] Warning: could not load zones: %v\n", zErr)
		} else {
			allZones = loaded
		}
	}

	allZones = mergeZonesWithSemarangHotspots(allZones)

	// Generate health warnings and traffic segments along the polyline
	healthWarnings, trafficSegments := generateHealthWarningsAndTraffic(coordinates, allZones)
	mapMarkers := generateMapMarkers(healthWarnings)

	response := map[string]any{
		"route_id":         routeID,
		"distance_meters":  distanceMeters,
		"duration_seconds": durationSeconds,
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
	if math.IsNaN(lat) || math.IsNaN(lon) {
		return fmt.Errorf("%s coordinates must be valid numbers", label)
	}
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

// extractStepsFromRoute converts OSRM route leg steps into frontend-compatible RouteStep objects.
func extractStepsFromRoute(
	route *navigation.Route,
	origin navigation.Coordinate,
	destination navigation.Coordinate,
	totalDistMeters int,
	totalDurSeconds int,
) []map[string]any {
	var rawSteps []navigation.RouteLegStep
	if route != nil {
		for _, leg := range route.Legs {
			rawSteps = append(rawSteps, leg.Steps...)
		}
	}

	steps := make([]map[string]any, 0, len(rawSteps))
	if len(rawSteps) == 0 {
		steps = append(steps, map[string]any{
			"instruction":      "Menuju tujuan",
			"distance_meters":  totalDistMeters,
			"duration_seconds": totalDurSeconds,
			"maneuver":         "straight",
			"start_location": map[string]float64{
				"latitude":  origin.Latitude,
				"longitude": origin.Longitude,
			},
			"end_location": map[string]float64{
				"latitude":  destination.Latitude,
				"longitude": destination.Longitude,
			},
		})
		return steps
	}

	for i, s := range rawSteps {
		startLat, startLon := stepStartLocation(s, origin)
		endLat, endLon := destination.Latitude, destination.Longitude
		if i+1 < len(rawSteps) {
			endLat, endLon = stepStartLocation(rawSteps[i+1], destination)
		} else if s.Geometry != nil && len(s.Geometry.Coordinates) > 0 {
			last := s.Geometry.Coordinates[len(s.Geometry.Coordinates)-1]
			if len(last) >= 2 && !(last[0] == 0 && last[1] == 0) {
				endLon, endLat = last[0], last[1]
			}
		}

		maneuver := mapOSRMManeuver(s.Maneuver.Type, s.Maneuver.Modifier)
		instruction := formatStepInstruction(s.Maneuver.Type, s.Maneuver.Modifier, s.Name)

		steps = append(steps, map[string]any{
			"instruction":      instruction,
			"distance_meters":  int(math.Round(s.Distance)),
			"duration_seconds": int(math.Round(s.Duration)),
			"maneuver":         maneuver,
			"start_location": map[string]float64{
				"latitude":  startLat,
				"longitude": startLon,
			},
			"end_location": map[string]float64{
				"latitude":  endLat,
				"longitude": endLon,
			},
		})
	}

	return steps
}

func stepStartLocation(s navigation.RouteLegStep, fallback navigation.Coordinate) (float64, float64) {
	if len(s.Maneuver.Location) >= 2 && !(s.Maneuver.Location[0] == 0 && s.Maneuver.Location[1] == 0) {
		return s.Maneuver.Location[1], s.Maneuver.Location[0]
	}
	if len(s.Intersections) > 0 && len(s.Intersections[0].Location) >= 2 {
		loc := s.Intersections[0].Location
		if !(loc[0] == 0 && loc[1] == 0) {
			return loc[1], loc[0]
		}
	}
	if s.Geometry != nil && len(s.Geometry.Coordinates) > 0 {
		first := s.Geometry.Coordinates[0]
		if len(first) >= 2 && !(first[0] == 0 && first[1] == 0) {
			return first[1], first[0]
		}
	}
	return fallback.Latitude, fallback.Longitude
}

func mapOSRMManeuver(maneuverType, modifier string) string {
	t := strings.ToLower(strings.TrimSpace(maneuverType))
	m := strings.ToLower(strings.TrimSpace(modifier))

	switch {
	case t == "arrive":
		return "arrive"
	case t == "roundabout" || t == "rotary" || t == "exit roundabout" || t == "exit rotary":
		return "roundabout"
	case strings.Contains(m, "uturn") || strings.Contains(m, "u-turn"):
		return "uturn"
	case strings.Contains(m, "left"):
		return "turn-left"
	case strings.Contains(m, "right"):
		return "turn-right"
	default:
		return "straight"
	}
}

func formatStepInstruction(maneuverType, modifier, roadName string) string {
	t := strings.ToLower(strings.TrimSpace(maneuverType))
	m := strings.ToLower(strings.TrimSpace(modifier))
	name := strings.TrimSpace(roadName)

	switch {
	case t == "arrive":
		if name != "" {
			return fmt.Sprintf("Sampai di tujuan (%s)", name)
		}
		return "Sampai di tujuan"
	case t == "depart":
		if name != "" {
			return fmt.Sprintf("Mulai perjalanan di %s", name)
		}
		return "Mulai dari lokasi Anda"
	case t == "roundabout" || t == "rotary":
		if name != "" {
			return fmt.Sprintf("Masuk bundaran menuju %s", name)
		}
		return "Masuk bundaran"
	case strings.Contains(m, "uturn") || strings.Contains(m, "u-turn"):
		if name != "" {
			return fmt.Sprintf("Putar balik ke %s", name)
		}
		return "Putar balik"
	case strings.Contains(m, "left"):
		if name != "" {
			return fmt.Sprintf("Belok kiri ke %s", name)
		}
		return "Belok kiri"
	case strings.Contains(m, "right"):
		if name != "" {
			return fmt.Sprintf("Belok kanan ke %s", name)
		}
		return "Belok kanan"
	default:
		if name != "" {
			return fmt.Sprintf("Ikuti %s", name)
		}
		return "Lurus terus"
	}
}

func mergeZonesWithSemarangHotspots(existing []zones.Zone) []zones.Zone {
	merged := make([]zones.Zone, 0, len(existing)+len(semarangFallbackHotspots))
	merged = append(merged, existing...)

	for _, fallback := range semarangFallbackHotspots {
		duplicate := false
		for _, z := range existing {
			if geoutils.HaversineDistance(z.Latitude, z.Longitude, fallback.Latitude, fallback.Longitude) < 0.4 {
				duplicate = true
				break
			}
		}
		if !duplicate {
			merged = append(merged, fallback)
		}
	}
	return merged
}

// generateHealthWarningsAndTraffic creates health warnings and traffic segments from zone data
// using Haversine proximity from the route polyline segments.
func generateHealthWarningsAndTraffic(coordinates [][]float64, allZones []zones.Zone) ([]map[string]any, []map[string]any) {
	warnings := make([]map[string]any, 0)
	trafficSegments := make([]map[string]any, 0)

	if len(coordinates) == 0 || len(allZones) == 0 {
		return warnings, trafficSegments
	}

	warningID := 0
	for _, zone := range allZones {
		thresholdKm := 0.35
		if zone.RadiusMeters > 0 {
			thresholdKm = math.Max(0.15, math.Min(zone.RadiusMeters/1000.0, 0.8))
		}

		closestSegIdx, minDistKm, affected := closestRouteProximity(coordinates, zone.Latitude, zone.Longitude, thresholdKm)
		if closestSegIdx < 0 || minDistKm > thresholdKm {
			continue
		}

		aqi, pm25 := resolveZoneAirQuality(zone)

		// Include health warnings when route passes a zone with AQI > 100
		if aqi > 100 {
			warningID++
			severity := "medium"
			color := "#F59E0B"
			if aqi > 150 {
				severity = "high"
				color = "#9333EA"
			}

			msg := fmt.Sprintf("AQI %d (Tidak Sehat). Potensi paparan partikel PM2.5 %.0f µg/m³.", aqi, pm25)
			if zone.Description != "" {
				msg = fmt.Sprintf("AQI %d. %s", aqi, zone.Description)
			}

			warnings = append(warnings, map[string]any{
				"warning_id": fmt.Sprintf("hw_%s_%02d", sanitizeZoneID(zone.ID), warningID),
				"type":       determineWarningType(zone.Category, aqi),
				"severity":   severity,
				"title":      zone.Name,
				"message":    msg,
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
		}

		// Add traffic segment if it's a congested corridor or intersection
		if zone.Category == "traffic_corridor" || zone.Category == "intersection" || zone.CongestionIndex >= 60 {
			if len(affected) == 0 {
				affected = [][]float64{coordinates[closestSegIdx]}
			}
			trafficSegments = append(trafficSegments, map[string]any{
				"segment_index":        closestSegIdx,
				"severity":             mapTrafficSeverity(zone.CongestionBaseLevel, zone.CongestionIndex),
				"delay_seconds":        int(math.Max(30, zone.CongestionIndex*2)),
				"affected_coordinates": affected,
			})
		}
	}

	return warnings, trafficSegments
}

func closestRouteProximity(coordinates [][]float64, zoneLat, zoneLon, thresholdKm float64) (int, float64, [][]float64) {
	bestIdx := -1
	minDist := math.MaxFloat64
	affected := make([][]float64, 0)

	for i, coord := range coordinates {
		if len(coord) < 2 {
			continue
		}
		d := geoutils.HaversineDistance(coord[1], coord[0], zoneLat, zoneLon)
		if d < minDist {
			minDist = d
			bestIdx = i
		}
		if d <= thresholdKm {
			affected = append(affected, coord)
		}

		// Check distance to segment between coord and next coord for sparse straight stretches
		if i+1 < len(coordinates) && len(coordinates[i+1]) >= 2 {
			segDist := pointToSegmentDistanceKm(zoneLat, zoneLon, coord[1], coord[0], coordinates[i+1][1], coordinates[i+1][0])
			if segDist < minDist {
				minDist = segDist
				bestIdx = i
			}
			if segDist <= thresholdKm && len(affected) == 0 {
				affected = append(affected, coord, coordinates[i+1])
			}
		}
	}

	return bestIdx, minDist, affected
}

func pointToSegmentDistanceKm(pLat, pLon, aLat, aLon, bLat, bLon float64) float64 {
	dx := bLon - aLon
	dy := bLat - aLat
	if dx == 0 && dy == 0 {
		return geoutils.HaversineDistance(pLat, pLon, aLat, aLon)
	}
	t := ((pLon-aLon)*dx + (pLat-aLat)*dy) / (dx*dx + dy*dy)
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	projLon := aLon + t*dx
	projLat := aLat + t*dy
	return geoutils.HaversineDistance(pLat, pLon, projLat, projLon)
}

func resolveZoneAirQuality(zone zones.Zone) (int, float64) {
	if zone.LastSnapshot != nil && zone.LastSnapshot.USAQI > 0 {
		pm25 := zone.LastSnapshot.PM25
		if pm25 <= 0 {
			pm25 = math.Round((float64(zone.LastSnapshot.USAQI)/150.0)*55.0*10) / 10
		}
		return zone.LastSnapshot.USAQI, pm25
	}
	if zone.CongestionIndex > 0 {
		aqi := int(zone.CongestionIndex*1.3) + 40
		pm25 := math.Round((float64(aqi)/150.0)*55.0*10) / 10
		return aqi, pm25
	}
	return 65, 22.0
}

func sanitizeZoneID(id string) string {
	cleaned := strings.ReplaceAll(strings.TrimSpace(id), " ", "_")
	if cleaned == "" {
		return "zone"
	}
	return cleaned
}

// generateMapMarkers creates map markers from warnings
func generateMapMarkers(warnings []map[string]any) []map[string]any {
	markers := make([]map[string]any, 0, len(warnings))

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

func determineWarningType(category string, aqi int) string {
	switch category {
	case "traffic_corridor", "intersection":
		if aqi > 150 {
			return "heavy_traffic"
		}
		return "moderate_traffic"
	case "industrial_area":
		return "high_pollution"
	default:
		return "high_pollution"
	}
}

func mapTrafficSeverity(baseLevel string, congestionIndex float64) string {
	lower := strings.ToLower(strings.TrimSpace(baseLevel))
	switch {
	case lower == "sangat padat" || lower == "padat" || lower == "heavy" || congestionIndex >= 75:
		return "heavy"
	case lower == "sedang" || lower == "moderate" || congestionIndex >= 45:
		return "moderate"
	default:
		return "light"
	}
}

func getHealthRisksForAQI(aqi int) []string {
	risks := make([]string, 0)

	if aqi <= 50 {
		risks = append(risks, "Tidak ada risiko kesehatan")
	} else if aqi <= 100 {
		risks = append(risks, "Iritasi ringan pada kelompok sensitif")
	} else if aqi <= 150 {
		risks = append(risks, "Iritasi saluran pernapasan", "Iritasi mata dan tenggorokan")
	} else if aqi <= 200 {
		risks = append(risks, "Iritasi saluran pernapasan", "Pemicu asma dan ISPA")
	} else {
		risks = append(risks, "Risiko kesehatan serius", "Gangguan paru akut pada kelompok berisiko")
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
		recommendations = append(recommendations, "Gunakan masker N95", "Tutup jendela kendaraan", "Hindari aktivitas outdoor berkepanjangan")
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
		return "cloud"
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

// encodePolyline encodes [lon, lat] coordinates into Google Encoded Polyline Algorithm Format.
func encodePolyline(coordinates [][]float64) string {
	if len(coordinates) == 0 {
		return ""
	}

	var b strings.Builder
	var prevLat, prevLon int64

	for _, coord := range coordinates {
		if len(coord) < 2 {
			continue
		}
		lat := int64(math.Round(coord[1] * 1e5))
		lon := int64(math.Round(coord[0] * 1e5))

		encodeSignedValue(&b, lat-prevLat)
		encodeSignedValue(&b, lon-prevLon)

		prevLat = lat
		prevLon = lon
	}

	return b.String()
}

func encodeSignedValue(b *strings.Builder, v int64) {
	if v < 0 {
		v = ^(v << 1)
	} else {
		v <<= 1
	}
	for v >= 0x20 {
		b.WriteByte(byte((0x20 | (v & 0x1f)) + 63))
		v >>= 5
	}
	b.WriteByte(byte(v + 63))
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

		dist := calculateDistance(prev[1], prev[0], curr[1], curr[0]) / 1000.0
		total += dist
	}

	return total
}
