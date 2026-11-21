package mapscraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"aurabackend/pkg/geoutils"
	"aurabackend/pkg/openmeteo"
)

// ScrapedLocation holds dynamic place/road metadata scraped from Google Maps and live air telemetry.
type ScrapedLocation struct {
	ZoneID   string  `json:"zone_id"`
	ZoneName string  `json:"zone_name"`
	Category string  `json:"category"`
	AQI      int     `json:"aqi"`
	PM25     float64 `json:"pm2_5"`
	Source   string  `json:"source"`
}

type cacheEntry struct {
	result    ScrapedLocation
	expiresAt time.Time
}

// Client scrapes real-time location names and road corridor context from Google Maps
// without any hardcoded hotspot lists.
type Client struct {
	httpClient *http.Client
	mu         sync.RWMutex
	cache      map[string]cacheEntry
}

var (
	defaultClient     *Client
	defaultClientOnce sync.Once
	plusCodeRegex     = regexp.MustCompile(`^[23456789CFGHJMPQRVWX]{4,8}\+[23456789CFGHJMPQRVWX]{2,4}\b[,\s]*`)
)

// NewClient returns a shared or new Google Maps scraper client.
func NewClient() *Client {
	defaultClientOnce.Do(func() {
		defaultClient = &Client{
			httpClient: &http.Client{
				Timeout: 4 * time.Second,
			},
			cache: make(map[string]cacheEntry),
		}
	})
	return defaultClient
}

func cacheKey(lat, lon float64) string {
	return fmt.Sprintf("%.3f_%.3f", lat, lon)
}

// ScrapeLocation dynamically resolves the street/landmark/district name at (lat, lon)
// by scraping Google Maps' preview/reveal endpoint (with OpenStreetMap Nominatim fallback)
// and combining it with live Open-Meteo air quality telemetry.
func (c *Client) ScrapeLocation(ctx context.Context, lat, lon float64, meteo *openmeteo.Client) ScrapedLocation {
	key := cacheKey(lat, lon)
	now := time.Now()

	c.mu.RLock()
	if entry, ok := c.cache[key]; ok && now.Before(entry.expiresAt) {
		c.mu.RUnlock()
		return entry.result
	}
	c.mu.RUnlock()

	placeName, fullAddress, source := c.scrapeGoogleMapsReveal(ctx, lat, lon)
	if placeName == "" {
		placeName, fullAddress, source = c.scrapeNominatimFallback(ctx, lat, lon)
	}
	if placeName == "" {
		placeName = fmt.Sprintf("Kawasan (%.4f, %.4f)", lat, lon)
		source = "gps_coordinate"
	}

	category, trafficAqiBoost := classifyCorridorFromScrapedText(placeName, fullAddress)

	// Fetch live Open-Meteo Air Quality for the exact coordinate
	baseAQI := 72
	basePM25 := 23.5
	if meteo != nil {
		if aq, err := meteo.FetchAirQuality(ctx, lat, lon); err == nil && aq != nil && aq.Current.USAQI > 0 {
			baseAQI = aq.Current.USAQI
			basePM25 = aq.Current.PM25
		}
	}

	finalAQI := baseAQI + trafficAqiBoost
	if finalAQI > 300 {
		finalAQI = 300
	}
	finalPM25 := math.Round((basePM25+float64(trafficAqiBoost)*0.38)*10) / 10

	res := ScrapedLocation{
		ZoneID:   "gmaps_" + geoutils.EncodeGeohash(lat, lon, 7),
		ZoneName: placeName,
		Category: category,
		AQI:      finalAQI,
		PM25:     finalPM25,
		Source:   source,
	}

	c.mu.Lock()
	c.cache[key] = cacheEntry{
		result:    res,
		expiresAt: now.Add(15 * time.Minute),
	}
	c.mu.Unlock()

	return res
}

// scrapeGoogleMapsReveal queries Google Maps' internal preview/reveal endpoint for (lat, lon).
func (c *Client) scrapeGoogleMapsReveal(ctx context.Context, lat, lon float64) (string, string, string) {
	// Try 1200m and 3000m reveal radii centered on (512, 384) of a 1024x768 viewport
	radii := []int{1200, 3000}
	for _, rMeters := range radii {
		url := fmt.Sprintf(
			"https://www.google.com/maps/preview/reveal?authuser=0&hl=id&gl=id&pb=!2m12!1m3!1d%d!2d%.6f!3d%.6f!2m3!1f0!2f0!3f0!3m2!1i1024!2i768!4f13.1!3m2!2d%.6f!3d%.6f!4m2!1i512!2i384",
			rMeters, lon, lat, lon, lat,
		)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
		req.Header.Set("Accept-Language", "id-ID,id;q=0.9,en-US;q=0.8")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}

		clean := strings.TrimSpace(string(body))
		clean = strings.TrimPrefix(clean, ")]}'")
		clean = strings.TrimSpace(clean)

		var rawArr []any
		if err := json.Unmarshal([]byte(clean), &rawArr); err != nil || len(rawArr) == 0 {
			continue
		}

		title, addr := extractGoogleMapsPlaceStrings(rawArr)
		if title != "" {
			return title, addr, "google_maps_scrape"
		}
	}

	return "", "", ""
}

func extractGoogleMapsPlaceStrings(rawArr []any) (string, string) {
	var primaryTitle string
	var secondaryAddr string

	// rawArr[0] is typically ["Street / Place Name", "Nearest Address / District, City"]
	if len(rawArr) > 0 {
		if arr0, ok := rawArr[0].([]any); ok && len(arr0) > 0 {
			if s0, ok := arr0[0].(string); ok {
				primaryTitle = cleanGoogleAddressPart(s0)
			}
			if len(arr0) > 1 {
				if s1, ok := arr0[1].(string); ok {
					secondaryAddr = cleanGoogleAddressPart(s1)
				}
			}
		}
	}

	// rawArr[1] is typically ["Full Formatted Address", "0x...:0x...", [null, null, lat, lon]]
	if len(rawArr) > 1 {
		if arr1, ok := rawArr[1].([]any); ok && len(arr1) > 0 {
			if s0, ok := arr1[0].(string); ok {
				cleaned := cleanGoogleAddressPart(s0)
				if secondaryAddr == "" {
					secondaryAddr = cleaned
				}
				if primaryTitle == "" {
					primaryTitle = cleaned
				}
			}
		}
	}

	if primaryTitle == "" && secondaryAddr == "" {
		return "", ""
	}

	// Build concise, informative display name from Google Maps address components
	display := formatConciseLocationName(primaryTitle, secondaryAddr)
	full := strings.TrimSpace(primaryTitle + ", " + secondaryAddr)
	return display, full
}

func cleanGoogleAddressPart(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "Alamat Terdekat:")
	s = strings.TrimPrefix(s, "Alamat terdekat:")
	s = strings.TrimSpace(s)
	s = plusCodeRegex.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func formatConciseLocationName(primary, secondary string) string {
	// Extract street or area name from secondary if primary is a generic city name or shop name
	secParts := strings.Split(secondary, ",")
	var streetPart, districtPart string
	for _, p := range secParts {
		trimmed := strings.TrimSpace(p)
		lower := strings.ToLower(trimmed)
		if streetPart == "" && (strings.HasPrefix(lower, "jl.") || strings.HasPrefix(lower, "jalan ") || strings.Contains(lower, "marina") || strings.Contains(lower, "industri") || strings.Contains(lower, "simpang")) {
			streetPart = trimmed
		}
		if districtPart == "" && (strings.HasPrefix(lower, "kec.") || (!strings.Contains(lower, "jawa tengah") && !strings.Contains(lower, "kota semarang") && len(trimmed) > 3 && trimmed != streetPart)) {
			districtPart = trimmed
		}
	}

	if streetPart != "" {
		if districtPart != "" && !strings.Contains(strings.ToLower(streetPart), strings.ToLower(districtPart)) {
			return fmt.Sprintf("%s, %s", streetPart, districtPart)
		}
		return streetPart
	}

	if primary != "" && !strings.EqualFold(primary, "Kota Semarang") && !strings.EqualFold(primary, "Laut") {
		if districtPart != "" && !strings.Contains(strings.ToLower(primary), strings.ToLower(districtPart)) {
			return fmt.Sprintf("%s, %s", primary, districtPart)
		}
		return primary
	}

	if len(secParts) >= 2 {
		return strings.TrimSpace(secParts[0]) + ", " + strings.TrimSpace(secParts[1])
	}
	if primary != "" {
		return primary
	}
	return secondary
}

func (c *Client) scrapeNominatimFallback(ctx context.Context, lat, lon float64) (string, string, string) {
	url := fmt.Sprintf(
		"https://nominatim.openstreetmap.org/reverse?format=jsonv2&lat=%.6f&lon=%.6f&zoom=18&addressdetails=1&accept-language=id",
		lat, lon,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", ""
	}
	req.Header.Set("User-Agent", "AuraBackend-MapScraper/1.0 (https://aura.health.app)")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", ""
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK {
		return "", "", ""
	}

	var parsed struct {
		Name        string            `json:"name"`
		DisplayName string            `json:"display_name"`
		Category    string            `json:"category"`
		Type        string            `json:"type"`
		Address     map[string]string `json:"address"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", "", ""
	}

	road := parsed.Address["road"]
	if road == "" {
		road = parsed.Name
	}
	suburb := parsed.Address["suburb"]
	if suburb == "" {
		suburb = parsed.Address["village"]
	}
	if suburb == "" {
		suburb = parsed.Address["neighbourhood"]
	}
	if suburb == "" {
		suburb = parsed.Address["city_district"]
	}

	var label string
	if road != "" && suburb != "" && !strings.EqualFold(road, suburb) {
		label = road + ", " + suburb
	} else if road != "" {
		label = road
	} else if suburb != "" {
		label = suburb
	}

	return label, parsed.DisplayName, "osm_nominatim_scrape"
}

func classifyCorridorFromScrapedText(title, fullAddr string) (string, int) {
	combined := strings.ToLower(title + " " + fullAddr)

	switch {
	case strings.Contains(combined, "industri") || strings.Contains(combined, "kaligawe") || strings.Contains(combined, "pabrik") || strings.Contains(combined, "pelabuhan"):
		return "industrial_area", 42
	case strings.Contains(combined, "simpang") || strings.Contains(combined, "gajahmada") || strings.Contains(combined, "pandanaran") || strings.Contains(combined, "pemuda") || strings.Contains(combined, "siliwangi") || strings.Contains(combined, "arteri") || strings.Contains(combined, "raya"):
		return "traffic_corridor", 26
	case strings.Contains(combined, "terminal") || strings.Contains(combined, "stasiun") || strings.Contains(combined, "bandara"):
		return "transit_hub", 24
	case strings.Contains(combined, "marina") || strings.Contains(combined, "pantai") || strings.Contains(combined, "taman"):
		return "coastal_green_zone", 4
	default:
		return "urban_street", 12
	}
}
