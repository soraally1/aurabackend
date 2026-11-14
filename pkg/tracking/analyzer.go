package tracking

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"aurabackend/pkg/zones"
)

// Analyzer processes waypoints and enriches them with zone and pollution data.
type Analyzer struct {
	zoneStore  *zones.Store
	groqClient *GroqClient
}

// NewAnalyzer creates a new waypoint analyzer without Groq support.
func NewAnalyzer(zoneStore *zones.Store) *Analyzer {
	return &Analyzer{
		zoneStore: zoneStore,
	}
}

// NewAnalyzerWithGroq creates a waypoint analyzer with Groq AI support.
func NewAnalyzerWithGroq(zoneStore *zones.Store, groqClient *GroqClient) *Analyzer {
	return &Analyzer{
		zoneStore:  zoneStore,
		groqClient: groqClient,
	}
}

// ProcessWaypoints enriches waypoints with zone and AQI data, and classifies as indoor/outdoor.
func (a *Analyzer) ProcessWaypoints(ctx context.Context, waypoints []Waypoint) []Waypoint {
	if len(waypoints) == 0 {
		return waypoints
	}

	// Classify indoor/outdoor and calculate durations
	a.classifyWaypoints(waypoints)

	// Match waypoints with pollution zones
	a.enrichWithZoneData(ctx, waypoints)

	return waypoints
}

// classifyWaypoints determines if each waypoint is indoor or outdoor based on movement patterns.
func (a *Analyzer) classifyWaypoints(waypoints []Waypoint) {
	for i := range waypoints {
		if i == 0 {
			waypoints[i].Classification = "outdoor"
			continue
		}

		prevWP := waypoints[i-1]
		currWP := waypoints[i]

		// Calculate distance from previous waypoint
		distance := haversineDistance(prevWP.Latitude, prevWP.Longitude, currWP.Latitude, currWP.Longitude)

		// Calculate duration since previous waypoint
		duration := currWP.Timestamp.Sub(prevWP.Timestamp).Minutes()

		// If distance < 60m and stayed for >= 20 minutes, classify as indoor
		if distance < 0.06 && duration >= 20 { // 0.06 km = 60 m
			waypoints[i].Classification = "indoor"
			waypoints[i].IsStay = true
		} else {
			waypoints[i].Classification = "outdoor"
			waypoints[i].IsStay = false
		}
	}
}

// enrichWithZoneData matches each waypoint with nearby pollution zones.
func (a *Analyzer) enrichWithZoneData(ctx context.Context, waypoints []Waypoint) {
	for i := range waypoints {
		// Search for zones within 1 km
		zones, err := a.zoneStore.FindNearby(ctx, waypoints[i].Latitude, waypoints[i].Longitude, 1.0)
		if err != nil || len(zones) == 0 {
			// No zone found, use default values
			waypoints[i].ZoneName = "Umum"
			waypoints[i].AQI = 50
			waypoints[i].PM25 = 20
			continue
		}

		// Use the closest zone
		closest := zones[0]
		waypoints[i].ZoneName = closest.Zone.Name
		waypoints[i].AQI = closest.AirQuality.USAQI
		waypoints[i].PM25 = closest.AirQuality.PM25
	}
}

// CalculateStats aggregates exposure statistics from waypoints.
func CalculateStats(waypoints []Waypoint, startedAt, endedAt time.Time) SessionStats {
	stats := SessionStats{
		WaypointsProcessed: len(waypoints),
	}

	if len(waypoints) == 0 {
		return stats
	}

	// Calculate duration
	stats.DurationMinutes = endedAt.Sub(startedAt).Minutes()

	// Calculate distance (haversine between all consecutive waypoints)
	var totalDistanceKm float64
	for i := 1; i < len(waypoints); i++ {
		dist := haversineDistance(
			waypoints[i-1].Latitude,
			waypoints[i-1].Longitude,
			waypoints[i].Latitude,
			waypoints[i].Longitude,
		)
		totalDistanceKm += dist
	}
	stats.DistanceKm = totalDistanceKm

	// Aggregate indoor/outdoor time and pollution metrics
	var outdoorCount, indoorCount int
	var aqiSum, pm25Sum float64
	maxAQI := 0
	worstIdx := 0

	for i, wp := range waypoints {
		if wp.Classification == "outdoor" {
			outdoorCount++
			aqiSum += float64(wp.AQI)
			pm25Sum += wp.PM25
			if wp.AQI > maxAQI {
				maxAQI = wp.AQI
				worstIdx = i
			}
		} else {
			indoorCount++
		}
	}

	// Calculate averages
	if outdoorCount > 0 {
		stats.AverageOutdoorAQI = aqiSum / float64(outdoorCount)
		stats.AverageOutdoorPM25 = pm25Sum / float64(outdoorCount)
	}
	stats.MaxAQI = maxAQI

	// Find worst area
	if worstIdx < len(waypoints) {
		stats.WorstAreaName = waypoints[worstIdx].ZoneName
		stats.WorstAreaAQI = waypoints[worstIdx].AQI
	} else {
		stats.WorstAreaName = "-"
	}

	// Calculate indoor/outdoor minutes (distribute duration by waypoint count)
	minutesPerWaypoint := 0.0
	if len(waypoints) > 0 {
		minutesPerWaypoint = stats.DurationMinutes / float64(len(waypoints))
	}
	stats.TotalOutdoorMinutes = float64(outdoorCount) * minutesPerWaypoint
	stats.TotalIndoorMinutes = float64(indoorCount) * minutesPerWaypoint

	return stats
}

// GenerateHealthAnalysis creates health recommendations based on exposure stats.
func GenerateHealthAnalysis(stats SessionStats) HealthAnalysis {
	return generateHealthAnalysisRuleBased(stats)
}

// GenerateHealthAnalysisWithGroq uses Groq AI if available, falls back to rule-based analysis.
func (a *Analyzer) GenerateHealthAnalysisWithGroq(ctx context.Context, stats SessionStats) HealthAnalysis {
	if a.groqClient == nil {
		return generateHealthAnalysisRuleBased(stats)
	}

	// Try Groq first, fall back to rule-based if it fails
	analysis, err := a.groqClient.AnalyzeExposureWithGroq(ctx, stats)
	if err != nil {
		log.Printf("[Tracking] Groq analysis error, using rule-based fallback: %v", err)
		return generateHealthAnalysisRuleBased(stats)
	}

	return analysis
}

// generateHealthAnalysisRuleBased creates health recommendations based on exposure stats using rule-based logic.
func generateHealthAnalysisRuleBased(stats SessionStats) HealthAnalysis {
	analysis := HealthAnalysis{
		Recommendations: make([]string, 0),
	}

	// Calculate impact score (0-100)
	timeFactor := math.Min(40.0, (stats.TotalOutdoorMinutes/120.0)*40.0)
	aqiFactor := math.Min(50.0, (stats.AverageOutdoorAQI/200.0)*50.0)
	peakFactor := 0.0
	if stats.MaxAQI > 100 {
		peakFactor = math.Min(10.0, float64(stats.MaxAQI-100)*0.1)
	}

	rawScore := math.Min(100.0, math.Max(10.0, timeFactor+aqiFactor+peakFactor))
	analysis.ImpactScore = math.Round(rawScore*10) / 10

	worstArea := stats.WorstAreaName
	if worstArea == "" || worstArea == "-" {
		worstArea = "jalur lalu lintas utama"
	}

	if analysis.ImpactScore < 38.0 {
		analysis.RiskLevel = "Rendah"
		analysis.Summary = fmt.Sprintf("Paparan polusi Anda hari ini tergolong rendah (durasi luar ruangan %.0f menit). Saluran pernapasan Anda dalam kondisi aman.", stats.TotalOutdoorMinutes)
		analysis.Recommendations = append(analysis.Recommendations,
			"Pertahankan hidrasi tubuh dengan minum air putih minimal 2 liter per hari.",
			"Lakukan ventilasi ruangan saat pagi hari ketika kualitas udara luar masih segar.",
			"Gunakan masker kain atau medis ringan jika berkendara sepeda motor santai.",
		)
	} else if analysis.ImpactScore < 68.0 {
		analysis.RiskLevel = "Sedang"
		analysis.Summary = fmt.Sprintf("Paparan polusi Anda hari ini berada pada tingkat sedang setelah beraktivitas luar ruangan selama %.0f menit di sekitar %s.", stats.TotalOutdoorMinutes, worstArea)
		analysis.Recommendations = append(analysis.Recommendations,
			"Gunakan masker standar medis atau KF94 saat bepergian melintasi jalan padat kendaraan.",
			"Bilas wajah dan bersihkan rongga hidung dengan air bersih atau larutan saline setelah sampai di rumah.",
			"Konsumsi buah bervitamin C tinggi (jeruk, jambu biji) untuk menangkal efek stres oksidatif partikulat polusi.",
			"Nyalakan ventilasi mekanis atau air purifier di kamar tidur.",
		)
	} else {
		analysis.RiskLevel = "Tinggi"
		analysis.Summary = fmt.Sprintf("Perhatian: Paparan polusi harian Anda tinggi dengan skor %.1f akibat durasi outdoor signifikan di area padat %s (AQI puncak %d).", analysis.ImpactScore, worstArea, stats.MaxAQI)
		analysis.Recommendations = append(analysis.Recommendations,
			"Wajib kenakan masker respirator N95/KF94 terpasang rapat (fit test) bila harus berada di luar ruangan.",
			"Gunakan obat semprot hidung saline (NaCl fisiologis) untuk membilas partikulat debu yang menempel di silia hidung.",
			"Konsumsi air hangat dan madu guna meredakan inflamasi dinding faring.",
			"Tutup rapat jendela dan pintu ruangan serta pasang pemurni udara (air purifier) berefisiensi HEPA.",
		)
	}

	return analysis
}

// haversineDistance calculates the great-circle distance between two points in kilometers.
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0 // Earth's radius in kilometers

	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Asin(math.Sqrt(a))
	return R * c
}

func toRad(deg float64) float64 {
	return deg * math.Pi / 180.0
}
