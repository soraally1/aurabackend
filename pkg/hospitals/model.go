package hospitals

import (
	"math"
	"regexp"
	"strings"
	"time"
)

// Hospital represents the Firestore document and JSON response for a hospital.
type Hospital struct {
	ID               string    `firestore:"id" json:"id"`
	Name             string    `firestore:"name" json:"name"`
	Address          string    `firestore:"address" json:"address"`
	Latitude         float64   `firestore:"latitude" json:"latitude"`
	Longitude        float64   `firestore:"longitude" json:"longitude"`
	Rating           float64   `firestore:"rating" json:"rating"`
	UserRatingsTotal int       `firestore:"user_ratings_total" json:"user_ratings_total"`
	Phone            string    `firestore:"phone" json:"phone"`
	OpenHours        string    `firestore:"open_hours" json:"open_hours"`
	Type             string    `firestore:"type" json:"type"` // "hospital"
	ImageURL         string    `firestore:"image_url" json:"image_url"`
	Facilities       []string  `firestore:"facilities" json:"facilities"`
	CreatedAt        time.Time `firestore:"created_at" json:"created_at,omitempty"`
	DistanceKm       float64   `firestore:"-" json:"distance_km,omitempty"`
}

// Haversine computes the distance between two coordinates in kilometers using
// the standard spherical law of cosines / haversine formulation.
func Haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0 // Earth's mean radius in KM
	dLat := (lat2 - lat1) * (math.Pi / 180.0)
	dLon := (lon2 - lon1) * (math.Pi / 180.0)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*(math.Pi/180.0))*math.Cos(lat2*(math.Pi/180.0))*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

// IsValidHospital implements strict filtering so Puskesmas, Klinik Pratama,
// Balai Pengobatan, Apotek, or Laboratorium are excluded.
func IsValidHospital(name string) bool {
	lower := strings.ToLower(name)

	// Blacklist kata kunci non-RS:
	blacklists := []string{"puskesmas", "pkm", "klinik", "clinic", "pratama", "apotek", "pharmacy", "laboratorium", "lab"}
	for _, b := range blacklists {
		if strings.Contains(lower, b) {
			return false
		}
	}

	// Whitelist kata kunci RS:
	whitelists := []string{"rs", "rumah sakit", "rsud", "rsup", "rsu", "hospital", "smc"}
	for _, w := range whitelists {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-z0-9]+`)

// GenerateID produces a URL-friendly slug from the hospital name.
// E.g. "RSUP Dr. Kariadi" -> "rsup-dr-kariadi"
func GenerateID(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	slug := nonAlphanumericRegex.ReplaceAllString(lower, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "hospital-" + time.Now().Format("20060102150405")
	}
	return slug
}
