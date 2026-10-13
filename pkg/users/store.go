// Package users persists user profiles in Firestore.
package users

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrNotFound is returned when a profile document does not exist.
var ErrNotFound = errors.New("user profile not found")

// User is the profile document stored at <collection>/<uid>.
// Credentials are never stored here; Firebase Auth owns them.
type User struct {
	UID          string     `firestore:"uid" json:"uid"`
	Email        string     `firestore:"email" json:"email"`
	Name         string     `firestore:"name" json:"name"`
	Usia         int        `firestore:"usia" json:"usia"`
	TinggiBadan  float64    `firestore:"tinggi_badan" json:"tinggi_badan"`
	BeratBadan   float64    `firestore:"berat_badan" json:"berat_badan"`
	WaktuDiJalan float64    `firestore:"waktu_di_jalan" json:"waktu_di_jalan"`
	CreatedAt    time.Time  `firestore:"createdAt" json:"createdAt"`
	UpdatedAt    time.Time  `firestore:"updatedAt" json:"updatedAt"`
	LastLoginAt  *time.Time `firestore:"lastLoginAt,omitempty" json:"lastLoginAt,omitempty"`
}

// Store reads and writes user profiles.
type Store struct {
	col *firestore.CollectionRef
}

// NewStore returns a Store backed by the given collection.
func NewStore(client *firestore.Client, collection string) *Store {
	return &Store{col: client.Collection(collection)}
}

// Create writes a new profile. It fails if the document already exists.
func (s *Store) Create(ctx context.Context, u *User) error {
	now := time.Now().UTC()
	u.CreatedAt, u.UpdatedAt = now, now
	_, err := s.col.Doc(u.UID).Create(ctx, u)
	return err
}

// Get fetches a profile by UID.
func (s *Store) Get(ctx context.Context, uid string) (*User, error) {
	snap, err := s.col.Doc(uid).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var u User
	if err := snap.DataTo(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// TouchLogin records the time of the latest successful login.
func (s *Store) TouchLogin(ctx context.Context, uid string) (time.Time, error) {
	now := time.Now().UTC()
	_, err := s.col.Doc(uid).Set(ctx, map[string]any{"lastLoginAt": now}, firestore.MergeAll)
	return now, err
}

// Update updates profile fields in Firestore.
func (s *Store) Update(ctx context.Context, uid string, updates map[string]any) error {
	updates["updatedAt"] = time.Now().UTC()
	_, err := s.col.Doc(uid).Set(ctx, updates, firestore.MergeAll)
	return err
}

