// Package config loads runtime configuration from environment variables.
package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Config holds all runtime configuration for the service.
type Config struct {
	// ServiceAccountJSON is the raw Firebase service account key (JSON).
	ServiceAccountJSON []byte
	// ProjectID is the Firebase / GCP project ID.
	ProjectID string
	// WebAPIKey is the Firebase Web API key, required for password sign-in
	// through the Identity Toolkit REST API.
	WebAPIKey string
	// FirestoreDatabaseID is the Firestore database to use ("(default)" if unset).
	FirestoreDatabaseID string
	// UsersCollection is the Firestore collection where user profiles are stored.
	UsersCollection string
	// ZonesCollection is the Firestore collection where traffic and pollution zones are stored.
	ZonesCollection string
	// HospitalsCollection is the Firestore collection where hospital records are stored.
	HospitalsCollection string
}

// Load reads configuration from the environment.
//
// Required:
//   - FIREBASE_SERVICE_ACCOUNT_KEY: service account JSON (raw or base64-encoded)
//   - FIREBASE_WEB_API_KEY:         Firebase Web API key
//
// Optional:
//   - FIREBASE_PROJECT_ID:    overrides the project_id inside the service account
//   - FIRESTORE_DATABASE_ID:  defaults to "(default)"
//   - USERS_COLLECTION:       defaults to "users"
//   - ZONES_COLLECTION:       defaults to "environmental_zones"
//   - HOSPITALS_COLLECTION:   defaults to "hospitals"
func Load() (*Config, error) {
	rawKey := strings.TrimSpace(os.Getenv("FIREBASE_SERVICE_ACCOUNT_KEY"))
	if rawKey == "" {
		return nil, errors.New("FIREBASE_SERVICE_ACCOUNT_KEY is not set")
	}

	saJSON, err := decodeServiceAccount(rawKey)
	if err != nil {
		return nil, err
	}

	var sa struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(saJSON, &sa); err != nil {
		return nil, fmt.Errorf("FIREBASE_SERVICE_ACCOUNT_KEY is not valid JSON: %w", err)
	}

	projectID := getEnv("FIREBASE_PROJECT_ID", sa.ProjectID)
	if projectID == "" {
		return nil, errors.New("project ID not found: set FIREBASE_PROJECT_ID or use a service account with project_id")
	}

	apiKey := strings.TrimSpace(os.Getenv("FIREBASE_WEB_API_KEY"))
	if apiKey == "" {
		return nil, errors.New("FIREBASE_WEB_API_KEY is not set")
	}

	return &Config{
		ServiceAccountJSON:  saJSON,
		ProjectID:           projectID,
		WebAPIKey:           apiKey,
		FirestoreDatabaseID: getEnv("FIRESTORE_DATABASE_ID", "(default)"),
		UsersCollection:     getEnv("USERS_COLLECTION", "users"),
		ZonesCollection:     getEnv("ZONES_COLLECTION", "environmental_zones"),
		HospitalsCollection: getEnv("HOSPITALS_COLLECTION", "hospitals"),
	}, nil
}

// decodeServiceAccount accepts either raw JSON or base64-encoded JSON.
// Base64 is handy on platforms where multi-line env values are awkward.
func decodeServiceAccount(raw string) ([]byte, error) {
	if strings.HasPrefix(raw, "{") {
		return []byte(raw), nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.New("FIREBASE_SERVICE_ACCOUNT_KEY must be raw JSON or base64-encoded JSON")
	}
	return decoded, nil
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
