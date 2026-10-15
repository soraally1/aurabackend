package hospitals

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrNotFound indicates the requested hospital does not exist.
var ErrNotFound = errors.New("hospital not found")

// Store manages hospital documents in Google Cloud Firestore.
type Store struct {
	client *firestore.Client
	col    *firestore.CollectionRef
}

// NewStore initializes a new Store for the designated hospitals collection.
func NewStore(client *firestore.Client, collection string) *Store {
	if collection == "" {
		collection = "hospitals"
	}
	return &Store{
		client: client,
		col:    client.Collection(collection),
	}
}

// Upsert inserts or updates a hospital document in Firestore using its ID.
func (s *Store) Upsert(ctx context.Context, h *Hospital) error {
	if h.ID == "" {
		h.ID = GenerateID(h.Name)
	}
	if h.CreatedAt.IsZero() {
		h.CreatedAt = time.Now().UTC()
	}
	if h.Type == "" {
		h.Type = "hospital"
	}
	if h.Facilities == nil {
		h.Facilities = []string{}
	}

	_, err := s.col.Doc(h.ID).Set(ctx, h)
	return err
}

// BatchUpsert saves multiple hospitals to Firestore in batches.
func (s *Store) BatchUpsert(ctx context.Context, hospitals []Hospital) (int, error) {
	if len(hospitals) == 0 {
		return 0, nil
	}

	saved := 0
	batch := s.client.Batch()
	batchCount := 0

	for i := range hospitals {
		h := &hospitals[i]
		if h.ID == "" {
			h.ID = GenerateID(h.Name)
		}
		if h.CreatedAt.IsZero() {
			h.CreatedAt = time.Now().UTC()
		}
		if h.Type == "" {
			h.Type = "hospital"
		}
		if h.Facilities == nil {
			h.Facilities = []string{}
		}

		docRef := s.col.Doc(h.ID)
		batch.Set(docRef, h)
		batchCount++
		saved++

		// Firestore batch limit is 500 writes
		if batchCount >= 400 {
			if _, err := batch.Commit(ctx); err != nil {
				return saved - batchCount, err
			}
			batch = s.client.Batch()
			batchCount = 0
		}
	}

	if batchCount > 0 {
		if _, err := batch.Commit(ctx); err != nil {
			return saved - batchCount, err
		}
	}

	return saved, nil
}

// Get fetches a single hospital document by ID.
func (s *Store) Get(ctx context.Context, id string) (*Hospital, error) {
	snap, err := s.col.Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var h Hospital
	if err := snap.DataTo(&h); err != nil {
		return nil, err
	}
	return &h, nil
}

// ListAll retrieves all hospital documents from Firestore.
func (s *Store) ListAll(ctx context.Context) ([]Hospital, error) {
	iter := s.col.Documents(ctx)
	defer iter.Stop()

	var result []Hospital
	for {
		doc, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}

		var h Hospital
		if err := doc.DataTo(&h); err == nil {
			result = append(result, h)
		}
	}

	return result, nil
}

// FindNearby retrieves hospitals, computes distances using Haversine,
// filters within radiusKm, sorts by distance ascending, and applies limit.
func (s *Store) FindNearby(ctx context.Context, lat, lon, radiusKm float64, limit int) ([]Hospital, error) {
	if radiusKm <= 0 {
		radiusKm = 15.0
	}
	if limit <= 0 {
		limit = 10
	}

	all, err := s.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	// If Firestore collection is empty, auto-seed curated hospitals so the user gets real data
	if len(all) == 0 {
		curated := CuratedSemarangHospitals()
		_, _ = s.BatchUpsert(ctx, curated)
		all = curated
	}

	type hospitalWithDist struct {
		h    Hospital
		dist float64
	}

	var matched []hospitalWithDist
	for _, h := range all {
		dist := Haversine(lat, lon, h.Latitude, h.Longitude)
		if dist <= radiusKm {
			h.DistanceKm = math.Round(dist*100) / 100
			matched = append(matched, hospitalWithDist{h: h, dist: dist})
		}
	}

	// Sort by distance ascending
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].dist < matched[j].dist
	})

	if len(matched) > limit {
		matched = matched[:limit]
	}

	output := make([]Hospital, len(matched))
	for i, m := range matched {
		output[i] = m.h
	}

	return output, nil
}
