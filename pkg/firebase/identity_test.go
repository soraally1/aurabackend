package firebase

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSignInWithGoogle_IDToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.RawQuery, "key=test-api-key") {
			t.Errorf("expected key in query, got %s", r.URL.RawQuery)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}

		var req struct {
			PostBody          string `json:"postBody"`
			RequestUri        string `json:"requestUri"`
			ReturnSecureToken bool   `json:"returnSecureToken"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("failed to parse json: %v", err)
		}

		if !strings.Contains(req.PostBody, "id_token=mock-google-id-token") {
			t.Errorf("expected id_token in postBody, got %s", req.PostBody)
		}
		if !strings.Contains(req.PostBody, "providerId=google.com") {
			t.Errorf("expected providerId=google.com in postBody, got %s", req.PostBody)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"localId": "google-user-123",
			"email": "user@google.com",
			"displayName": "Google User",
			"idToken": "firebase-id-token-abc",
			"refreshToken": "firebase-refresh-token-xyz",
			"expiresIn": "3600"
		}`))
	}))
	defer server.Close()

	client := NewIdentityClient("test-api-key")
	client.signInWithIdpURL = server.URL

	res, err := client.SignInWithGoogle(context.Background(), "mock-google-id-token", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if res.UID != "google-user-123" {
		t.Errorf("expected UID 'google-user-123', got %s", res.UID)
	}
	if res.Email != "user@google.com" {
		t.Errorf("expected email 'user@google.com', got %s", res.Email)
	}
	if res.DisplayName != "Google User" {
		t.Errorf("expected displayName 'Google User', got %s", res.DisplayName)
	}
	if res.Tokens.IDToken != "firebase-id-token-abc" {
		t.Errorf("expected idToken 'firebase-id-token-abc', got %s", res.Tokens.IDToken)
	}
	if res.Tokens.RefreshToken != "firebase-refresh-token-xyz" {
		t.Errorf("expected refreshToken 'firebase-refresh-token-xyz', got %s", res.Tokens.RefreshToken)
	}
	if res.Tokens.ExpiresIn != 3600 {
		t.Errorf("expected expiresIn 3600, got %d", res.Tokens.ExpiresIn)
	}
}

func TestSignInWithGoogle_AccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			PostBody string `json:"postBody"`
		}
		_ = json.Unmarshal(body, &req)

		if !strings.Contains(req.PostBody, "access_token=mock-access-token") {
			t.Errorf("expected access_token in postBody, got %s", req.PostBody)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"localId": "google-user-456",
			"email": "user2@google.com",
			"idToken": "firebase-id-token-def",
			"refreshToken": "firebase-refresh-token-uvw",
			"expiresIn": "3600"
		}`))
	}))
	defer server.Close()

	client := NewIdentityClient("test-api-key")
	client.signInWithIdpURL = server.URL

	res, err := client.SignInWithGoogle(context.Background(), "", "mock-access-token")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if res.UID != "google-user-456" {
		t.Errorf("expected UID 'google-user-456', got %s", res.UID)
	}
}

func TestSignInWithGoogle_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{
			"error": {
				"message": "INVALID_IDP_RESPONSE : Bad token"
			}
		}`))
	}))
	defer server.Close()

	client := NewIdentityClient("test-api-key")
	client.signInWithIdpURL = server.URL

	_, err := client.SignInWithGoogle(context.Background(), "invalid-token", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	ie, ok := err.(*IdentityError)
	if !ok {
		t.Fatalf("expected *IdentityError, got %T", err)
	}
	if ie.Code() != "INVALID_IDP_RESPONSE" {
		t.Errorf("expected code 'INVALID_IDP_RESPONSE', got %s", ie.Code())
	}
}
