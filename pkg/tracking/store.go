package tracking

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrNotFound = errors.New("session not found")
var ErrSessionEnded = errors.New("session already ended")

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
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	if session.Status != "active" {
		return ErrSessionEnded
	}

	// Append new waypoints
	session.Waypoints = append(session.Waypoints, waypoints...)
	session.UpdatedAt = time.Now().UTC()

	_, err = s.col.Doc(sessionID).Set(ctx, session)
	return err
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

	session.Status = "completed"
	session.EndLocation = &endLocation
	session.EndedAt = &endLocation.Timestamp
	session.UpdatedAt = time.Now().UTC()

	// Store stats and analysis in separate fields for direct access in responses
	updatePaths := []firestore.Update{
		{Path: "status", Value: "completed"},
		{Path: "end_location", Value: endLocation},
		{Path: "ended_at", Value: endLocation.Timestamp},
		{Path: "updated_at", Value: session.UpdatedAt},
		{Path: "stats", Value: stats},
		{Path: "analysis", Value: analysis},
	}

	_, err = s.col.Doc(sessionID).Update(ctx, updatePaths)
	return err
}

// GetSessionsByDate retrieves all sessions for a user on a specific date.
func (s *Store) GetSessionsByDate(ctx context.Context, userID string, dateStr string) ([]Session, error) {
	// Parse date to time range
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return nil, fmt.Errorf("invalid date format: %w", err)
	}

	startOfDay := date.UTC()
	endOfDay := startOfDay.Add(24 * time.Hour)

	query := s.col.
		Where("user_id", "==", userID).
		Where("started_at", ">=", startOfDay).
		Where("started_at", "<", endOfDay).
		OrderBy("started_at", firestore.Asc)

	iter := query.Documents(ctx)
	defer iter.Stop()

	var sessions []Session
	for {
		doc, err := iter.Next()
		if err.Error() == "no more documents" {
			break
		}
		if err != nil {
			return nil, err
		}

		var session Session
		if err := doc.DataTo(&session); err == nil {
			sessions = append(sessions, session)
		}
	}

	return sessions, nil
}
