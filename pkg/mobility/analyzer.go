package mobility

import (
	"context"
	"math"
	"time"

	"aurabackend/pkg/geoutils"
	"aurabackend/pkg/openmeteo"
	"aurabackend/pkg/zones"
)

// KnownSemarangHotspots provides reference pollution baselines for major traffic nodes.
type Hotspot struct {
	Name    string
	Lat     float64
	Lon     float64
	BaseAQI int
	PM25    float64
}

var semarangHotspots = []Hotspot{
	{Name: "Kawasan Industri Kaligawe", Lat: -6.9587, Lon: 110.4578, BaseAQI: 154, PM25: 61.2},
	{Name: "Simpang Lima Semarang", Lat: -6.9893, Lon: 110.4229, BaseAQI: 132, PM25: 48.5},
	{Name: "Bundaran Tugu Muda", Lat: -6.9840, Lon: 110.4093, BaseAQI: 125, PM25: 45.0},
	{Name: "Simpang Peterongan", Lat: -6.9996, Lon: 110.4320, BaseAQI: 118, PM25: 42.5},
	{Name: "Gerbang Tol Krapyak", Lat: -6.9850, Lon: 110.3600, BaseAQI: 142, PM25: 53.0},
	{Name: "Kawasan Banyumanik - Setiabudi", Lat: -7.0650, Lon: 110.4180, BaseAQI: 95, PM25: 33.0},
	{Name: "Kawasan RSUP Dr. Kariadi - Sutomo", Lat: -6.9934, Lon: 110.4087, BaseAQI: 88, PM25: 29.5},
}

// Analyzer processes mobility waypoints, classifies stay vs travel, and performs spatial matching.
type Analyzer struct {
	zoneStore       *zones.Store
	openMeteoClient *openmeteo.Client
	groqClient      *GroqClient
}

// NewAnalyzer creates a new Analyzer instance.
func NewAnalyzer(zoneStore *zones.Store, groqClient *GroqClient) *Analyzer {
	return &Analyzer{
		zoneStore:       zoneStore,
		openMeteoClient: openmeteo.NewClient(),
		groqClient:      groqClient,
	}
}

// Analyze processes a batch of waypoints into a DailyAnalysis report.
func (a *Analyzer) Analyze(ctx context.Context, userID, dateStr string, waypoints []Waypoint) (DailyAnalysis, error) {
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}
	if userID == "" {
		userID = "guest-user"
	}

	classifiedWaypoints := make([]Waypoint, len(waypoints))
	var totalIndoorMin float64
	var totalOutdoorMin float64

	var outdoorAQISum float64
	var outdoorPM25Sum float64
	var outdoorDurationSum float64

	worstAQI := 0
	worstAreaName := "Kota Semarang"
	worstAreaTimestamp := ""

	for i, wp := range waypoints {
		w := wp

		// Step 1: Deteksi Stay vs Perjalanan
		// Jika duration di 1 area >= 20 menit atau flag is_stay bernilai true, klasifikasikan sebagai indoor.
		if w.DurationMinutes >= 20.0 || w.IsStay {
			w.Classification = "indoor"
			w.IsStay = true
			totalIndoorMin += w.DurationMinutes
		} else {
			w.Classification = "outdoor"
			w.IsStay = false
			totalOutdoorMin += w.DurationMinutes
		}

		// Step 2: Spatial Matching dengan Indeks Polusi Zona Semarang
		matchedZone, aqi, pm25 := a.matchPollutionIndex(ctx, w.Latitude, w.Longitude)
		if w.ZoneName == "" {
			w.ZoneName = matchedZone
		}
		if w.AQI <= 0 {
			w.AQI = aqi
		}
		if w.PM25 <= 0 {
			w.PM25 = pm25
		}

		// Track outdoor exposure
		if w.Classification == "outdoor" {
			dur := w.DurationMinutes
			if dur <= 0 {
				dur = 5.0 // default sample duration if missing
			}
			outdoorAQISum += float64(w.AQI) * dur
			outdoorPM25Sum += w.PM25 * dur
			outdoorDurationSum += dur

			if w.AQI > worstAQI {
				worstAQI = w.AQI
				worstAreaName = w.ZoneName
				worstAreaTimestamp = w.Timestamp.Format(time.RFC3339)
			}
		}

		classifiedWaypoints[i] = w
	}

	// Compute averages
	avgOutdoorAQI := 75.0
	avgOutdoorPM25 := 25.0
	if outdoorDurationSum > 0 {
		avgOutdoorAQI = math.Round((outdoorAQISum/outdoorDurationSum)*10) / 10
		avgOutdoorPM25 = math.Round((outdoorPM25Sum/outdoorDurationSum)*10) / 10
	} else if len(classifiedWaypoints) > 0 {
		// All indoor
		avgOutdoorAQI = 50.0
		avgOutdoorPM25 = 15.0
		worstAreaName = "Area Dalam Ruangan"
		worstAQI = 50
	}

	stats := ExposureStats{
		TotalIndoorMinutes:  totalIndoorMin,
		TotalOutdoorMinutes: totalOutdoorMin,
		AverageOutdoorAQI:   avgOutdoorAQI,
		AverageOutdoorPM25:  avgOutdoorPM25,
		MaxAQI:              worstAQI,
		WorstAreaName:       worstAreaName,
		WorstAreaAQI:        worstAQI,
		WorstAreaTimestamp:  worstAreaTimestamp,
		WaypointsProcessed:  len(classifiedWaypoints),
	}

	// Step 3: Integrasi Groq API (Llama-3.3-70b-versatile)
	diagnosis, _ := a.groqClient.AnalyzeExposureWithGroq(ctx, stats)

	now := time.Now().UTC()
	return DailyAnalysis{
		ID:        userID + "_" + dateStr,
		UserID:    userID,
		Date:      dateStr,
		Stats:     stats,
		Analysis:  diagnosis,
		Waypoints: classifiedWaypoints,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// matchPollutionIndex matches coordinate to Firestore zones or reference hotspots.
func (a *Analyzer) matchPollutionIndex(ctx context.Context, lat, lon float64) (string, int, float64) {
	// 1. Try finding nearby Firestore zones if zoneStore is provided
	if a.zoneStore != nil {
		nearby, err := a.zoneStore.FindNearby(ctx, lat, lon, 2.5)
		if err == nil && len(nearby) > 0 {
			z := nearby[0].Zone
			aqi := int(z.CongestionIndex*1.3) + 40
			pm25 := (float64(aqi) / 150.0) * 55.0
			return z.Name, aqi, math.Round(pm25*10) / 10
		}
	}

	// 2. Spatial match with known reference hotspots in Semarang
	closestName := "Pusat Kota Semarang"
	minDist := math.MaxFloat64
	bestAQI := 78
	bestPM25 := 26.5

	for _, h := range semarangHotspots {
		dist := geoutils.HaversineDistance(lat, lon, h.Lat, h.Lon)
		if dist < minDist {
			minDist = dist
			closestName = h.Name
			bestAQI = h.BaseAQI
			bestPM25 = h.PM25
		}
	}

	if minDist <= 4.0 {
		return closestName, bestAQI, bestPM25
	}

	// Default baseline for general Semarang suburban area
	return "Wilayah Semarang", 75, 25.0
}
