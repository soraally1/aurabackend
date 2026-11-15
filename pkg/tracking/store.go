package tracking

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrNotFound = errors.New("session not found")
var ErrSessionEnded = errors.New("session already ended")

// wibLocation represents Asia/Jakarta (UTC+7) for daily history date boundaries.
var wibLocation = time.FixedZone("WIB", 7*3600)

// Store manages persistence of tracking sessions in Firestore.
type Store struct {
	col *firestore.CollectionRef
}

// NewStore returns a new Store backed by the designated collection.
func NewStore(client *firestore.Client, collection string) *Store {
	return &Store{col: client.Collection(collection)}
}

// CreateSession inserts a new tracking session into Firestore.
func (s *Store) CreateSession(ctx context.Context, session *Session) error {
	now := time.Now().UTC()
	session.CreatedAt = now
	session.UpdatedAt = now
	session.Status = "active"

	_, err := s.col.Doc(session.ID).Set(ctx, session)
	return err
}

// GetSession retrieves a single session by ID.
func (s *Store) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	snap, err := s.col.Doc(sessionID).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var session Session
	if err := snap.DataTo(&session); err != nil {
		return nil, err
	}
	return &session, nil
}

// UpdateWaypoints appends waypoints to a session and updates the modified timestamp.
func (s *Store) UpdateWaypoints(ctx context.Context, sessionID string, waypoints []Waypoint) error {
	_, err := s.AppendWaypoints(ctx, sessionID, waypoints)
	return err
}

// AppendWaypoints appends waypoints to a session and returns the new total waypoint count.
func (s *Store) AppendWaypoints(ctx context.Context, sessionID string, waypoints []Waypoint) (int, error) {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return 0, err
	}

	if session.Status != "active" {
		return 0, ErrSessionEnded
	}

	session.Waypoints = append(session.Waypoints, waypoints...)
	session.UpdatedAt = time.Now().UTC()

	updatePaths := []firestore.Update{
		{Path: "waypoints", Value: session.Waypoints},
		{Path: "updated_at", Value: session.UpdatedAt},
	}

	if _, err := s.col.Doc(sessionID).Update(ctx, updatePaths); err != nil {
		return 0, err
	}
	return len(session.Waypoints), nil
}

// EndSession marks a session as completed with stats and analysis.
func (s *Store) EndSession(ctx context.Context, sessionID string, endLocation Location, stats SessionStats, analysis HealthAnalysis) error {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	if session.Status != "active" {
		return ErrSessionEnded
	}

	if stats.WorstAreaName == "" {
		stats.WorstAreaName = "-"
	}

	now := time.Now().UTC()
	updatePaths := []firestore.Update{
		{Path: "status", Value: "completed"},
		{Path: "end_location", Value: endLocation},
		{Path: "ended_at", Value: endLocation.Timestamp},
		{Path: "updated_at", Value: now},
		{Path: "stats", Value: stats},
		{Path: "analysis", Value: analysis},
	}

	_, err = s.col.Doc(sessionID).Update(ctx, updatePaths)
	return err
}

// GetSessionsByDate retrieves all sessions for a user on a specific date (WIB / UTC+7).
// Queries by user_id (single-field index) and filters/sorts in Go to avoid Firestore composite index errors.
func (s *Store) GetSessionsByDate(ctx context.Context, userID string, dateStr string) ([]Session, error) {
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return nil, fmt.Errorf("invalid date format: %w", err)
	}

	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, wibLocation).UTC()
	endOfDay := startOfDay.Add(24 * time.Hour)

	iter := s.col.Where("user_id", "==", userID).Documents(ctx)
	defer iter.Stop()

	sessions := make([]Session, 0)
	for {
		doc, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}

		var session Session
		if err := doc.DataTo(&session); err != nil {
			continue
		}

		startedUTC := session.StartedAt.UTC()
		if !startedUTC.Before(startOfDay) && startedUTC.Before(endOfDay) {
			sessions = append(sessions, session)
		}
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartedAt.Before(sessions[j].StartedAt)
	})

	return sessions, nil
}
