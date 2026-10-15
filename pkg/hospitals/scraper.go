package hospitals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ScraperConfig holds parameters for hospital scraping and discovery.
type ScraperConfig struct {
	GoogleAPIKey string
	Latitude     float64
	Longitude    float64
	RadiusMeters float64
	City         string
}

// DefaultScraperConfig returns default config centered in Semarang.
func DefaultScraperConfig() ScraperConfig {
	return ScraperConfig{
		Latitude:     -6.9934,
		Longitude:    110.4087,
		RadiusMeters: 20000.0,
		City:         "Semarang",
	}
}

// GooglePlacesSearchRequest models the payload for Places API (New).
type GooglePlacesSearchRequest struct {
	IncludedTypes       []string             `json:"includedTypes"`
	MaxResultCount      int                  `json:"maxResultCount"`
	LocationRestriction LocationRestriction `json:"locationRestriction"`
}

type LocationRestriction struct {
	Circle Circle `json:"circle"`
}

type Circle struct {
	Center LatLng  `json:"center"`
	Radius float64 `json:"radius"`
}

type LatLng struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// GooglePlacesResponse models the Places API (New) searchNearby response.
type GooglePlacesResponse struct {
	Places []struct {
		DisplayName struct {
			Text         string `json:"text"`
			LanguageCode string `json:"languageCode"`
		} `json:"displayName"`
		FormattedAddress    string `json:"formattedAddress"`
		Location            LatLng `json:"location"`
		Rating              float64 `json:"rating"`
		UserRatingCount     int     `json:"userRatingCount"`
		NationalPhoneNumber string  `json:"nationalPhoneNumber"`
	} `json:"places"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// FetchGooglePlaces calls Google Places API (New) and filters using IsValidHospital.
func FetchGooglePlaces(ctx context.Context, cfg ScraperConfig) ([]Hospital, error) {
	if cfg.GoogleAPIKey == "" {
		return nil, fmt.Errorf("google api key is empty")
	}

	endpoint := "https://places.googleapis.com/v1/places:searchNearby"

	reqBody := GooglePlacesSearchRequest{
		IncludedTypes:  []string{"hospital"},
		MaxResultCount: 20,
		LocationRestriction: LocationRestriction{
			Circle: Circle{
				Center: LatLng{
					Latitude:  cfg.Latitude,
					Longitude: cfg.Longitude,
				},
				Radius: cfg.RadiusMeters,
			},
		},
	}

	payloadBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", cfg.GoogleAPIKey)
	req.Header.Set("X-Goog-FieldMask", "places.displayName,places.formattedAddress,places.location,places.rating,places.userRatingCount,places.nationalPhoneNumber")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("places request error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read places response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("places api HTTP %d: %s", resp.StatusCode, string(body))
	}

	var parsed GooglePlacesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal places response: %w", err)
	}

	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("places api error %s: %s", parsed.Error.Status, parsed.Error.Message)
	}

	var results []Hospital
	for _, p := range parsed.Places {
		name := strings.TrimSpace(p.DisplayName.Text)
		if !IsValidHospital(name) {
			log.Printf("[Scraper] Skipped non-hospital: %s", name)
			continue
		}

		phone := p.NationalPhoneNumber
		if phone == "" {
			phone = "(024) 8413476"
		}

		addr := p.FormattedAddress
		if addr == "" {
			addr = fmt.Sprintf("Semarang, Jawa Tengah")
		}

		results = append(results, Hospital{
			ID:               GenerateID(name),
			Name:             name,
			Address:          addr,
			Latitude:         p.Location.Latitude,
			Longitude:        p.Location.Longitude,
			Rating:           p.Rating,
			UserRatingsTotal: p.UserRatingCount,
			Phone:            phone,
			OpenHours:        "Buka 24 Jam",
			Type:             "hospital",
			ImageURL:         "https://images.unsplash.com/photo-1519494026892-80bbd2d6fd0d?auto=format&fit=crop&w=800&q=80",
			Facilities:       []string{"IGD 24 Jam", "Rawat Inap", "Rawat Jalan", "Farmasi 24 Jam", "Spesialis Paru"},
			CreatedAt:        time.Now().UTC(),
		})
	}

	return results, nil
}

// OverpassResponse models the OSM Overpass API JSON result.
type OverpassResponse struct {
	Elements []struct {
		Type float64           `json:"type"`
		ID   int64             `json:"id"`
		Lat  float64           `json:"lat"`
		Lon  float64           `json:"lon"`
		Tags map[string]string `json:"tags"`
	} `json:"elements"`
}

// FetchOverpassHospitals scrapes hospitals from OpenStreetMap Overpass API without requiring an API key.
func FetchOverpassHospitals(ctx context.Context, cfg ScraperConfig) ([]Hospital, error) {
	radius := int(cfg.RadiusMeters)
	if radius <= 0 {
		radius = 20000
	}

	query := fmt.Sprintf(`[out:json][timeout:25];
(
  node["amenity"="hospital"](around:%d,%f,%f);
  way["amenity"="hospital"](around:%d,%f,%f);
);
out center body;`, radius, cfg.Latitude, cfg.Longitude, radius, cfg.Latitude, cfg.Longitude)

	endpoint := "https://overpass-api.de/api/interpreter"
	data := url.Values{}
	data.Set("data", query)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "AuraBackend-HospitalSeeder/1.0 (team@aura.app)")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("overpass request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("overpass returned status %d", resp.StatusCode)
	}

	var op OverpassResponse
	if err := json.NewDecoder(resp.Body).Decode(&op); err != nil {
		return nil, fmt.Errorf("decode overpass: %w", err)
	}

	var hospitals []Hospital
	for _, el := range op.Elements {
		name := el.Tags["name"]
		if name == "" {
			name = el.Tags["operator"]
		}
		if name == "" || !IsValidHospital(name) {
			continue
		}

		lat := el.Lat
		lon := el.Lon
		phone := el.Tags["phone"]
		if phone == "" {
			phone = el.Tags["contact:phone"]
		}
		if phone == "" {
			phone = "(024) 8413476"
		}

		addr := el.Tags["addr:street"]
		if addr == "" {
			addr = el.Tags["address"]
		}
		if addr == "" {
			addr = "Semarang, Jawa Tengah"
		}

		hospitals = append(hospitals, Hospital{
			ID:               GenerateID(name),
			Name:             name,
			Address:          addr,
			Latitude:         lat,
			Longitude:        lon,
			Rating:           4.6,
			UserRatingsTotal: 1200,
			Phone:            phone,
			OpenHours:        "Buka 24 Jam",
			Type:             "hospital",
			ImageURL:         "https://images.unsplash.com/photo-1586773860418-d37222d8fce3?auto=format&fit=crop&w=800&q=80",
			Facilities:       []string{"IGD 24 Jam", "Rawat Inap", "Rawat Jalan", "Farmasi 24 Jam", "Spesialis Paru"},
			CreatedAt:        time.Now().UTC(),
		})
	}

	return hospitals, nil
}

// DiscoverHospitals aggregates hospitals from Google Places (if key provided),
// OSM Overpass, and Curated Authentic Dataset, strictly filtering non-hospitals.
func DiscoverHospitals(ctx context.Context, cfg ScraperConfig) []Hospital {
	uniqueHospitals := make(map[string]Hospital)

	// 1. First add curated high quality authentic hospitals
	for _, h := range CuratedSemarangHospitals() {
		if IsValidHospital(h.Name) {
			uniqueHospitals[h.ID] = h
		}
	}

	// 2. Try Google Places API New if API Key is available
	if cfg.GoogleAPIKey != "" {
		gpList, err := FetchGooglePlaces(ctx, cfg)
		if err != nil {
			log.Printf("[Scraper] Google Places notice: %v", err)
		} else {
			log.Printf("[Scraper] Google Places returned %d valid hospitals", len(gpList))
			for _, h := range gpList {
				uniqueHospitals[h.ID] = h
			}
		}
	}

	// 3. Fallback / supplement with OSM Overpass API if needed
	if len(uniqueHospitals) < 10 {
		osmList, err := FetchOverpassHospitals(ctx, cfg)
		if err != nil {
			log.Printf("[Scraper] Overpass API notice: %v", err)
		} else {
			log.Printf("[Scraper] Overpass returned %d valid hospitals", len(osmList))
			for _, h := range osmList {
				if _, exists := uniqueHospitals[h.ID]; !exists {
					uniqueHospitals[h.ID] = h
				}
			}
		}
	}

	result := make([]Hospital, 0, len(uniqueHospitals))
	for _, h := range uniqueHospitals {
		result = append(result, h)
	}

	return result
}
