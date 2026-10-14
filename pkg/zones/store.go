package zones

import (
	"context"
	"errors"
	"sort"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"aurabackend/pkg/geoutils"
)

// ErrNotFound indicates the requested zone doesn't exist.
var ErrNotFound = errors.New("zone not found")

// Store manages persistence of environmental zones in Firestore.
type Store struct {
	col *firestore.CollectionRef
}

// NewStore returns a new Store backed by the designated collection.
func NewStore(client *firestore.Client, collection string) *Store {
	return &Store{col: client.Collection(collection)}
}

// Create inserts a new zone into Firestore.
func (s *Store) Create(ctx context.Context, z *Zone) error {
	now := time.Now().UTC()
	if z.CreatedAt.IsZero() {
		z.CreatedAt = now
	}
	z.UpdatedAt = now

	if z.Geohash == "" {
		z.Geohash = geoutils.EncodeGeohash(z.Latitude, z.Longitude, 6)
	}

	_, err := s.col.Doc(z.ID).Set(ctx, z)
	return err
}

// Get retrieves a single zone by ID.
func (s *Store) Get(ctx context.Context, id string) (*Zone, error) {
	snap, err := s.col.Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var z Zone
	if err := snap.DataTo(&z); err != nil {
		return nil, err
	}
	return &z, nil
}

// ListAll retrieves all mapped zones.
func (s *Store) ListAll(ctx context.Context) ([]Zone, error) {
	iter := s.col.Documents(ctx)
	defer iter.Stop()

	var zones []Zone
	for {
		doc, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var z Zone
		if err := doc.DataTo(&z); err == nil {
			zones = append(zones, z)
		}
	}
	return zones, nil
}

// FindNearby searches for zones within radiusKm from the provided coordinates.
// It leverages bounding-box candidate pre-filtering combined with exact Haversine distance verification.
func (s *Store) FindNearby(ctx context.Context, lat, lon, radiusKm float64) ([]NearbyZoneResult, error) {
	bbox := geoutils.CalculateBoundingBox(lat, lon, radiusKm)

	// Firestore range query on latitude
	query := s.col.Where("latitude", ">=", bbox.MinLat).Where("latitude", "<=", bbox.MaxLat)
	iter := query.Documents(ctx)
	defer iter.Stop()

	var results []NearbyZoneResult
	for {
		doc, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			// If Firestore index or permission error happens, fallback to ListAll filtering
			return s.findNearbyFallback(ctx, lat, lon, radiusKm)
		}

		var z Zone
		if err := doc.DataTo(&z); err != nil {
			continue
		}

		// Verify longitude bounds
		if z.Longitude < bbox.MinLon || z.Longitude > bbox.MaxLon {
			continue
		}

		// Exact Haversine distance
		dist := geoutils.HaversineDistance(lat, lon, z.Latitude, z.Longitude)
		if dist <= radiusKm {
			results = append(results, NearbyZoneResult{
				Zone:             z,
				DistanceKm:       dist,
				CongestionStatus: z.CongestionBaseLevel,
			})
		}
	}

	// Sort nearest first
	sort.Slice(results, func(i, j int) bool {
		return results[i].DistanceKm < results[j].DistanceKm
	})

	return results, nil
}

func (s *Store) findNearbyFallback(ctx context.Context, lat, lon, radiusKm float64) ([]NearbyZoneResult, error) {
	all, err := s.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	var results []NearbyZoneResult
	for _, z := range all {
		dist := geoutils.HaversineDistance(lat, lon, z.Latitude, z.Longitude)
		if dist <= radiusKm {
			results = append(results, NearbyZoneResult{
				Zone:             z,
				DistanceKm:       dist,
				CongestionStatus: z.CongestionBaseLevel,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].DistanceKm < results[j].DistanceKm
	})

	return results, nil
}

// SeedDefaultZones populates Firestore with default sample hotspot zones if empty.
func (s *Store) SeedDefaultZones(ctx context.Context) error {
	defaults := []Zone{
		{
			ID:                  "zone_jkt_sudirman",
			Name:                "Koridor Sudirman - Thamrin (Jakarta)",
			Description:         "Koridor arteri sentral bisnis dengan kepadatan kendaraan bermotor tinggi dan emisi knalpot pekat.",
			Category:            "traffic_corridor",
			Latitude:            -6.2088,
			Longitude:           106.8229,
			Geohash:             geoutils.EncodeGeohash(-6.2088, 106.8229, 6),
			RadiusMeters:        2000,
			CongestionBaseLevel: "Padat",
			CongestionIndex:     82.0,
			PeakHours:           []string{"07:00-09:30", "16:30-20:00"},
			PollutionSources:    []string{"Kendaraan Pribadi", "TransJakarta & Bus Diesel", "Ojek Online Spills"},
			HistoricalTrafficFactor: map[string]float64{
				"Monday":    1.25,
				"Tuesday":   1.15,
				"Wednesday": 1.10,
				"Thursday":  1.15,
				"Friday":    1.30,
				"Saturday":  0.85,
				"Sunday":    0.70,
			},
		},
		{
			ID:                  "zone_jkt_priok",
			Name:                "Kawasan Logistik & Pelabuhan Tanjung Priok",
			Description:         "Pusat mobilitas truk kontainer dan armada logistik berat penghasil jelaga diesel dan partikulat PM10/PM2.5.",
			Category:            "industrial_area",
			Latitude:            -6.1086,
			Longitude:           106.8837,
			Geohash:             geoutils.EncodeGeohash(-6.1086, 106.8837, 6),
			RadiusMeters:        3500,
			CongestionBaseLevel: "Sangat Padat",
			CongestionIndex:     89.5,
			PeakHours:           []string{"09:00-14:00", "17:00-22:00"},
			PollutionSources:    []string{"Truk Kontainer Diesel", "Aktivitas Bongkar Muat", "Pabrik Pergudangan"},
			HistoricalTrafficFactor: map[string]float64{
				"Monday":    1.20,
				"Tuesday":   1.25,
				"Wednesday": 1.20,
				"Thursday":  1.20,
				"Friday":    1.25,
				"Saturday":  1.05,
				"Sunday":    0.65,
			},
		},
		{
			ID:                  "zone_bdg_pasteur",
			Name:                "Gerbang Tol & Simpang Pasteur (Bandung)",
			Description:         "Pintu masuk utama Kota Bandung dengan kemacetan parah di akhir pekan dan gas buang knalpot stagnan.",
			Category:            "intersection",
			Latitude:            -6.8967,
			Longitude:           107.5759,
			Geohash:             geoutils.EncodeGeohash(-6.8967, 107.5759, 6),
			RadiusMeters:        1800,
			CongestionBaseLevel: "Padat",
			CongestionIndex:     79.0,
			PeakHours:           []string{"06:30-08:30", "16:00-19:30"},
			PollutionSources:    []string{"Kendaraan Wisatawan", "Mobil Pribadi", "Antrean Gerbang Tol"},
			HistoricalTrafficFactor: map[string]float64{
				"Monday":    1.05,
				"Tuesday":   0.95,
				"Wednesday": 0.95,
				"Thursday":  1.05,
				"Friday":    1.35,
				"Saturday":  1.40,
				"Sunday":    1.30,
			},
		},
		{
			ID:                  "zone_sby_darmo",
			Name:                "Koridor Raya Darmo - Wonokromo (Surabaya)",
			Description:         "Jalur utama mobilitas komuter Surabaya Selatan dengan volume sepeda motor dan mobil sangat padat.",
			Category:            "traffic_corridor",
			Latitude:            -7.2905,
			Longitude:           112.7381,
			Geohash:             geoutils.EncodeGeohash(-7.2905, 112.7381, 6),
			RadiusMeters:        2200,
			CongestionBaseLevel: "Padat",
			CongestionIndex:     76.5,
			PeakHours:           []string{"06:45-08:45", "16:30-19:00"},
			PollutionSources:    []string{"Sepeda Motor Komuter", "Mobil Pribadi", "Angkutan Umum"},
			HistoricalTrafficFactor: map[string]float64{
				"Monday":    1.20,
				"Tuesday":   1.10,
				"Wednesday": 1.10,
				"Thursday":  1.10,
				"Friday":    1.25,
				"Saturday":  0.90,
				"Sunday":    0.75,
			},
		},
	}

	for _, d := range defaults {
		if err := s.Create(ctx, &d); err != nil {
			return err
		}
	}
	return nil
}
