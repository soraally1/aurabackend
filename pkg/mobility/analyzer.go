package mobility

import (
	"context"
	"math"
	"time"

	"aurabackend/pkg/mapscraper"
	"aurabackend/pkg/openmeteo"
	"aurabackend/pkg/zones"
)

// Analyzer processes mobility waypoints, classifies stay vs travel, and performs dynamic map scraping & spatial matching.
type Analyzer struct {
	zoneStore       *zones.Store
	openMeteoClient *openmeteo.Client
	mapScraper      *mapscraper.Client
	groqClient      *GroqClient
}

// NewAnalyzer creates a new Analyzer instance backed by dynamic Google Maps scraping and live Open-Meteo telemetry.
func NewAnalyzer(zoneStore *zones.Store, groqClient *GroqClient) *Analyzer {
	return &Analyzer{
		zoneStore:       zoneStore,
		openMeteoClient: openmeteo.NewClient(),
		mapScraper:      mapscraper.NewClient(),
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
	worstAreaName := ""
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

		// Step 2: Dynamic Spatial Matching via Firestore Zones + Real-Time Google Maps Scraping & Open-Meteo
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
				dur = 5.0
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
	avgOutdoorAQI := 72.0
	avgOutdoorPM25 := 23.5
	if outdoorDurationSum > 0 {
		avgOutdoorAQI = math.Round((outdoorAQISum/outdoorDurationSum)*10) / 10
		avgOutdoorPM25 = math.Round((outdoorPM25Sum/outdoorDurationSum)*10) / 10
	} else if len(classifiedWaypoints) > 0 {
		avgOutdoorAQI = 50.0
		avgOutdoorPM25 = 15.0
		worstAreaName = classifiedWaypoints[0].ZoneName
		if worstAreaName == "" {
			worstAreaName = "Area Dalam Ruangan"
		}
		worstAQI = 50
	}

	if worstAreaName == "" && len(classifiedWaypoints) > 0 {
		worstAreaName = classifiedWaypoints[0].ZoneName
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

// matchPollutionIndex dynamically resolves zone name and pollution index from Firestore or live Google Maps scraping.
func (a *Analyzer) matchPollutionIndex(ctx context.Context, lat, lon float64) (string, int, float64) {
	// 1. Try finding nearby Firestore zones if zoneStore is provided
	if a.zoneStore != nil {
		nearby, err := a.zoneStore.FindNearby(ctx, lat, lon, 1.8)
		if err == nil && len(nearby) > 0 {
			z := nearby[0].Zone
			aqi := int(z.CongestionIndex*1.3) + 40
			if z.LastSnapshot != nil && z.LastSnapshot.USAQI > 0 {
				aqi = z.LastSnapshot.USAQI
			}
			pm25 := (float64(aqi) / 150.0) * 55.0
			if z.LastSnapshot != nil && z.LastSnapshot.PM25 > 0 {
				pm25 = z.LastSnapshot.PM25
			}
			return z.Name, aqi, math.Round(pm25*10) / 10
		}
	}

	// 2. Dynamic real-time Google Maps scraping + Open-Meteo Air Quality (Zero hardcoded hotspots)
	scraper := a.mapScraper
	if scraper == nil {
		scraper = mapscraper.NewClient()
	}
	scraped := scraper.ScrapeLocation(ctx, lat, lon, a.openMeteoClient)
	return scraped.ZoneName, scraped.AQI, scraped.PM25
}
