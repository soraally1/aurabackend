package mobility

import (
	"context"
	"errors"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrNotFound indicates no daily analysis was found for the requested user and date.
var ErrNotFound = errors.New("mobility daily analysis not found")

// Store persists DailyAnalysis documents in Firestore.
type Store struct {
	col *firestore.CollectionRef
}

// NewStore creates a new Store for the designated mobility collection.
func NewStore(client *firestore.Client, collection string) *Store {
	if collection == "" {
		collection = "daily_mobility"
	}
	return &Store{
		col: client.Collection(collection),
	}
}

// Save stores or overwrites a DailyAnalysis document in Firestore.
func (s *Store) Save(ctx context.Context, analysis *DailyAnalysis) error {
	docID := analysis.ID
	if docID == "" {
		docID = analysis.UserID + "_" + analysis.Date
		analysis.ID = docID
	}
	_, err := s.col.Doc(docID).Set(ctx, analysis)
	return err
}

// Get retrieves a DailyAnalysis document by user ID and date.
func (s *Store) Get(ctx context.Context, userID, date string) (*DailyAnalysis, error) {
	docID := userID + "_" + date
	snap, err := s.col.Doc(docID).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var analysis DailyAnalysis
	if err := snap.DataTo(&analysis); err != nil {
		return nil, err
	}
	return &analysis, nil
}
