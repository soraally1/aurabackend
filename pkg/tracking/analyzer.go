package tracking

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"aurabackend/pkg/mapscraper"
	"aurabackend/pkg/openmeteo"
	"aurabackend/pkg/zones"
)

// UserProfileContext holds optional sensitivity metadata for personalized AI analysis.
type UserProfileContext struct {
	Name         string
	Usia         int
	WaktuDiJalan float64
}

// Analyzer processes waypoints and enriches them with dynamic Google Maps scraped place data and live pollution telemetry.
type Analyzer struct {
	zoneStore       *zones.Store
	openMeteoClient *openmeteo.Client
	mapScraper      *mapscraper.Client
	groqClient      *GroqClient
}

// NewAnalyzer creates a new waypoint analyzer without Groq support.
func NewAnalyzer(zoneStore *zones.Store) *Analyzer {
	return &Analyzer{
		zoneStore:       zoneStore,
		openMeteoClient: openmeteo.NewClient(),
		mapScraper:      mapscraper.NewClient(),
	}
}

// NewAnalyzerWithGroq creates a waypoint analyzer with Groq AI support.
func NewAnalyzerWithGroq(zoneStore *zones.Store, groqClient *GroqClient) *Analyzer {
	return &Analyzer{
		zoneStore:       zoneStore,
		openMeteoClient: openmeteo.NewClient(),
		mapScraper:      mapscraper.NewClient(),
		groqClient:      groqClient,
	}
}

// ProcessWaypoints enriches waypoints with zone and AQI data, and classifies as indoor/outdoor.
func (a *Analyzer) ProcessWaypoints(ctx context.Context, waypoints []Waypoint) []Waypoint {
	return a.ProcessWaypointsWithContext(ctx, waypoints, nil)
}

// ProcessWaypointsWithContext enriches incremental waypoint batches using an optional previous
// session waypoint as anchor context for stationary indoor stay detection.
func (a *Analyzer) ProcessWaypointsWithContext(ctx context.Context, waypoints []Waypoint, prevAnchor *Waypoint) []Waypoint {
	if len(waypoints) == 0 {
		return waypoints
	}

	a.classifyWaypointsWithAnchor(waypoints, prevAnchor)
	a.enrichWithZoneData(ctx, waypoints)

	return waypoints
}

// classifyWaypoints determines if each waypoint is indoor or outdoor based on movement patterns.
func (a *Analyzer) classifyWaypoints(waypoints []Waypoint) {
	a.classifyWaypointsWithAnchor(waypoints, nil)
}

func (a *Analyzer) classifyWaypointsWithAnchor(waypoints []Waypoint, prevAnchor *Waypoint) {
	if len(waypoints) == 0 {
		return
	}

	var anchor Waypoint
	if prevAnchor != nil {
		anchor = *prevAnchor
	} else {
		anchor = waypoints[0]
		waypoints[0].Classification = "outdoor"
		waypoints[0].IsStay = false
	}

	startIdx := 1
	if prevAnchor != nil {
		startIdx = 0
	}

	for i := startIdx; i < len(waypoints); i++ {
		currWP := waypoints[i]
		distanceKm := haversineDistance(anchor.Latitude, anchor.Longitude, currWP.Latitude, currWP.Longitude)
		stayDurationMin := currWP.Timestamp.Sub(anchor.Timestamp).Minutes()

		// Stayed within 60m of cluster anchor for >= 20 minutes -> indoor stay
		if distanceKm < 0.06 && stayDurationMin >= 20 {
			waypoints[i].Classification = "indoor"
			waypoints[i].IsStay = true
		} else {
			waypoints[i].Classification = "outdoor"
			waypoints[i].IsStay = false
			if distanceKm >= 0.06 {
				anchor = currWP
			}
		}
	}
}

// enrichWithZoneData matches each waypoint with Firestore zones, Semarang hotspots, or live Open-Meteo AQI.
func (a *Analyzer) enrichWithZoneData(ctx context.Context, waypoints []Waypoint) {
	if len(waypoints) == 0 {
		return
	}

	// Load Firestore zones once per batch to avoid N+1 Firestore queries
	var allZones []zones.Zone
	if a.zoneStore != nil {
		loaded, err := a.zoneStore.ListAll(ctx)
		if err == nil {
			allZones = loaded
		}
	}

	// Lazy-fetch live Open-Meteo AQI at most once per batch if needed as fallback
	var liveFetched bool
	var liveAQI int
	var livePM25 float64

	getLiveEnv := func(lat, lon float64) (int, float64) {
		if liveFetched {
			return liveAQI, livePM25
		}
		liveFetched = true
		liveAQI = 65
		livePM25 = 22.0
		if a.openMeteoClient != nil {
			aq, err := a.openMeteoClient.FetchAirQuality(ctx, lat, lon)
			if err == nil && aq != nil && aq.Current.USAQI > 0 {
				liveAQI = aq.Current.USAQI
				livePM25 = math.Round(aq.Current.PM25*10) / 10
			}
		}
		return liveAQI, livePM25
	}

	for i := range waypoints {
		lat, lon := waypoints[i].Latitude, waypoints[i].Longitude

		// 1. Closest Firestore zone within 2.5 km
		var bestZone *zones.Zone
		bestZoneDist := 2.5
		for idx := range allZones {
			d := haversineDistance(lat, lon, allZones[idx].Latitude, allZones[idx].Longitude)
			if d <= bestZoneDist {
				bestZoneDist = d
				bestZone = &allZones[idx]
			}
		}

		if bestZone != nil {
			waypoints[i].ZoneID = bestZone.ID
			waypoints[i].ZoneName = bestZone.Name
			if bestZone.LastSnapshot != nil && bestZone.LastSnapshot.USAQI > 0 {
				waypoints[i].AQI = bestZone.LastSnapshot.USAQI
				waypoints[i].PM25 = math.Round(bestZone.LastSnapshot.PM25*10) / 10
			} else if bestZone.CongestionIndex > 0 {
				aqi := int(bestZone.CongestionIndex*1.3) + 40
				waypoints[i].AQI = aqi
				waypoints[i].PM25 = math.Round(((float64(aqi)/150.0)*55.0)*10) / 10
			} else {
				aqi, pm25 := getLiveEnv(lat, lon)
				waypoints[i].AQI = aqi
				waypoints[i].PM25 = pm25
			}
			continue
		}

		// 2. Dynamic real-time Google Maps scraping + Open-Meteo Air Quality (Zero hardcoded hotspots)
		scraper := a.mapScraper
		if scraper == nil {
			scraper = mapscraper.NewClient()
		}
		scraped := scraper.ScrapeLocation(ctx, lat, lon, a.openMeteoClient)
		waypoints[i].ZoneID = scraped.ZoneID
		waypoints[i].ZoneName = scraped.ZoneName
		waypoints[i].AQI = scraped.AQI
		waypoints[i].PM25 = scraped.PM25
	}
}

// CalculateStats aggregates exposure statistics from waypoints.
func CalculateStats(waypoints []Waypoint, startedAt, endedAt time.Time) SessionStats {
	stats := SessionStats{
		WaypointsProcessed: len(waypoints),
		WorstAreaName:      "-",
	}

	duration := endedAt.Sub(startedAt).Minutes()
	if duration < 0 {
		duration = 0
	}
	stats.DurationMinutes = math.Round(duration*10) / 10

	if len(waypoints) == 0 {
		return stats
	}

	// Calculate distance (haversine between all consecutive waypoints)
	var totalDistanceKm float64
	for i := 1; i < len(waypoints); i++ {
		dist := haversineDistance(
			waypoints[i-1].Latitude,
			waypoints[i-1].Longitude,
			waypoints[i].Latitude,
			waypoints[i].Longitude,
		)
		// Filter unrealistic GPS teleportation jumps (> 5 km between consecutive points)
		if dist <= 5.0 {
			totalDistanceKm += dist
		}
	}
	stats.DistanceKm = math.Round(totalDistanceKm*100) / 100

	// Aggregate indoor/outdoor metrics
	var outdoorCount, indoorCount int
	var aqiSum, pm25Sum float64
	maxAQI := 0
	worstIdx := -1

	for i, wp := range waypoints {
		if wp.Classification != "indoor" {
			outdoorCount++
			aqiSum += float64(wp.AQI)
			pm25Sum += wp.PM25
			if wp.AQI >= maxAQI {
				maxAQI = wp.AQI
				worstIdx = i
			}
		} else {
			indoorCount++
		}
	}

	if outdoorCount > 0 {
		stats.AverageOutdoorAQI = math.Round((aqiSum/float64(outdoorCount))*10) / 10
		stats.AverageOutdoorPM25 = math.Round((pm25Sum/float64(outdoorCount))*10) / 10
	}
	stats.MaxAQI = maxAQI

	if worstIdx >= 0 && worstIdx < len(waypoints) && waypoints[worstIdx].ZoneName != "" {
		stats.WorstAreaName = waypoints[worstIdx].ZoneName
		stats.WorstAreaAQI = waypoints[worstIdx].AQI
	}

	// Calculate indoor/outdoor minutes (distribute total duration across waypoints)
	minutesPerWaypoint := stats.DurationMinutes / float64(len(waypoints))
	stats.TotalOutdoorMinutes = math.Round(float64(outdoorCount)*minutesPerWaypoint*10) / 10
	stats.TotalIndoorMinutes = math.Round(float64(indoorCount)*minutesPerWaypoint*10) / 10

	return stats
}

// GenerateHealthAnalysis creates health recommendations based on exposure stats.
func GenerateHealthAnalysis(stats SessionStats) HealthAnalysis {
	return generateHealthAnalysisRuleBased(stats)
}

// GenerateHealthAnalysisWithGroq uses Groq AI if available, falls back to rule-based analysis.
func (a *Analyzer) GenerateHealthAnalysisWithGroq(ctx context.Context, stats SessionStats) HealthAnalysis {
	return a.GenerateHealthAnalysisWithUser(ctx, stats, nil)
}

// GenerateHealthAnalysisWithUser uses Groq AI with optional user sensitivity context.
func (a *Analyzer) GenerateHealthAnalysisWithUser(ctx context.Context, stats SessionStats, userCtx *UserProfileContext) HealthAnalysis {
	if a.groqClient == nil {
		return generateHealthAnalysisRuleBased(stats)
	}

	analysis, err := a.groqClient.AnalyzeExposureWithUserGroq(ctx, stats, userCtx)
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
