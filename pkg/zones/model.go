// Package zones defines models and Firestore persistence for environmental
// and traffic hotspot zones.
package zones

import (
	"time"

	"aurabackend/pkg/openmeteo"
)

// Zone represents a mapped area prone to traffic congestion and air pollution.
type Zone struct {
	ID                      string                    `firestore:"id" json:"id"`
	Name                    string                    `firestore:"name" json:"name"`
	Description             string                    `firestore:"description" json:"description"`
	Category                string                    `firestore:"category" json:"category"` // traffic_corridor, industrial_area, intersection, transit_hub
	Latitude                float64                   `firestore:"latitude" json:"latitude"`
	Longitude               float64                   `firestore:"longitude" json:"longitude"`
	Geohash                 string                    `firestore:"geohash" json:"geohash"`
	RadiusMeters            float64                   `firestore:"radius_meters" json:"radius_meters"`
	CongestionBaseLevel     string                    `firestore:"congestion_base_level" json:"congestion_base_level"` // Rendah, Sedang, Padat, Sangat Padat
	CongestionIndex         float64                   `firestore:"congestion_index" json:"congestion_index"`           // 0.0 - 100.0
	PeakHours               []string                  `firestore:"peak_hours" json:"peak_hours"`
	PollutionSources        []string                  `firestore:"pollution_sources" json:"pollution_sources"`
	HistoricalTrafficFactor map[string]float64        `firestore:"historical_traffic_factor" json:"historical_traffic_factor"`
	LastSnapshot            *ZoneAirSnapshot          `firestore:"last_snapshot,omitempty" json:"last_snapshot,omitempty"`
	CreatedAt               time.Time                 `firestore:"createdAt" json:"createdAt"`
	UpdatedAt               time.Time                 `firestore:"updatedAt" json:"updatedAt"`
}

// ZoneAirSnapshot holds the latest cached air quality reading for this zone.
type ZoneAirSnapshot struct {
	PM25      float64   `firestore:"pm25" json:"pm25"`
	PM10      float64   `firestore:"pm10" json:"pm10"`
	CO        float64   `firestore:"co" json:"co"`
	USAQI     int       `firestore:"us_aqi" json:"us_aqi"`
	UpdatedAt time.Time `firestore:"updatedAt" json:"updatedAt"`
}

// NearbyZoneResult wraps a matched zone with computed distance and live environmental metrics.
type NearbyZoneResult struct {
	Zone             Zone                         `json:"zone"`
	DistanceKm       float64                      `json:"distance_km"`
	AirQuality       openmeteo.CurrentAirQuality  `json:"air_quality"`
	CongestionStatus string                       `json:"congestion_status"`
	ImpactScore      float64                      `json:"impact_score"`
	ImpactLevel      string                       `json:"impact_level"` // Aman, Waspada, Berbahaya
}
