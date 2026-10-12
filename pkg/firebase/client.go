// Package firebase wires up the Firebase Admin SDK (Auth + Firestore) and the
// Identity Toolkit REST client used for password sign-in.
package firebase

import (
	"context"
	"fmt"
	"sync"

	"cloud.google.com/go/firestore"
	fb "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"

	"aurabackend/pkg/config"
)

// Clients bundles every Firebase dependency the handlers need.
type Clients struct {
	Config    *config.Config
	Auth      *auth.Client
	Firestore *firestore.Client
	Identity  *IdentityClient
}

var (
	once    sync.Once
	clients *Clients
	initErr error
)

// Get returns process-wide Firebase clients, initializing them on first use.
// On Vercel this lets warm invocations reuse connections instead of
// re-authenticating on every request.
func Get(ctx context.Context) (*Clients, error) {
	once.Do(func() {
		clients, initErr = newClients(ctx)
	})
	return clients, initErr
}

func newClients(ctx context.Context) (*Clients, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	// Initialization must not be tied to a single request's lifetime, since the
	// clients outlive it.
	ctx = context.WithoutCancel(ctx)
	cred := option.WithAuthCredentialsJSON(option.ServiceAccount, cfg.ServiceAccountJSON)

	app, err := fb.NewApp(ctx, &fb.Config{ProjectID: cfg.ProjectID}, cred)
	if err != nil {
		return nil, fmt.Errorf("firebase app: %w", err)
	}

	authClient, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("firebase auth: %w", err)
	}

	fsClient, err := firestore.NewClientWithDatabase(ctx, cfg.ProjectID, cfg.FirestoreDatabaseID, cred)
	if err != nil {
		return nil, fmt.Errorf("firestore: %w", err)
	}

	return &Clients{
		Config:    cfg,
		Auth:      authClient,
		Firestore: fsClient,
		Identity:  NewIdentityClient(cfg.WebAPIKey),
	}, nil
}
