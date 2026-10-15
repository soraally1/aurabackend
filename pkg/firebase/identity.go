package firebase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The Admin SDK cannot verify a user's password, so login goes through the
// Firebase Auth (Identity Toolkit) REST API using the project's Web API key.
const (
	signInURL        = "https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword"
	signInWithIdpURL = "https://identitytoolkit.googleapis.com/v1/accounts:signInWithIdp"
	refreshURL       = "https://securetoken.googleapis.com/v1/token"
)

// IdentityClient talks to the Firebase Auth REST API.
type IdentityClient struct {
	apiKey           string
	http             *http.Client
	signInURL        string
	signInWithIdpURL string
	refreshURL       string
}

// NewIdentityClient creates an IdentityClient.
func NewIdentityClient(apiKey string) *IdentityClient {
	return &IdentityClient{
		apiKey:           apiKey,
		http:             &http.Client{Timeout: 10 * time.Second},
		signInURL:        signInURL,
		signInWithIdpURL: signInWithIdpURL,
		refreshURL:       refreshURL,
	}
}

// Tokens are the credentials returned to the client after sign-in.
type Tokens struct {
	IDToken      string `json:"idToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int    `json:"expiresIn"` // seconds
}

// SignInResult is the outcome of a successful sign-in.
type SignInResult struct {
	UID         string
	Email       string
	DisplayName string
	Tokens      Tokens
}

// IdentityError is an error returned by the Firebase Auth REST API,
// e.g. "INVALID_LOGIN_CREDENTIALS" or "USER_DISABLED".
type IdentityError struct {
	Status  int
	Message string
}

func (e *IdentityError) Error() string {
	return fmt.Sprintf("identity toolkit: %s (http %d)", e.Message, e.Status)
}

// Code returns the error code without any trailing detail
// ("TOO_MANY_ATTEMPTS_TRY_LATER : ..." -> "TOO_MANY_ATTEMPTS_TRY_LATER").
func (e *IdentityError) Code() string {
	code, _, _ := strings.Cut(e.Message, " ")
	return code
}

// SignInWithPassword verifies an email/password pair and returns tokens.
func (c *IdentityClient) SignInWithPassword(ctx context.Context, email, password string) (*SignInResult, error) {
	body, _ := json.Marshal(map[string]any{
		"email":             email,
		"password":          password,
		"returnSecureToken": true,
	})

	var out struct {
		LocalID      string `json:"localId"`
		Email        string `json:"email"`
		DisplayName  string `json:"displayName"`
		IDToken      string `json:"idToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    string `json:"expiresIn"`
	}
	if err := c.do(ctx, c.signInURL, "application/json", bytes.NewReader(body), &out); err != nil {
		return nil, err
	}

	return &SignInResult{
		UID:         out.LocalID,
		Email:       out.Email,
		DisplayName: out.DisplayName,
		Tokens: Tokens{
			IDToken:      out.IDToken,
			RefreshToken: out.RefreshToken,
			ExpiresIn:    atoiOr(out.ExpiresIn, 3600),
		},
	}, nil
}

// SignInWithGoogle exchanges a Google ID token or access token with Firebase
// Identity Toolkit (accounts:signInWithIdp) for Firebase tokens and user info.
func (c *IdentityClient) SignInWithGoogle(ctx context.Context, idToken, accessToken string) (*SignInResult, error) {
	var postBodyParts []string
	if idToken != "" {
		postBodyParts = append(postBodyParts, "id_token="+url.QueryEscape(idToken))
	}
	if accessToken != "" {
		postBodyParts = append(postBodyParts, "access_token="+url.QueryEscape(accessToken))
	}
	postBodyParts = append(postBodyParts, "providerId=google.com")

	body, _ := json.Marshal(map[string]any{
		"postBody":          strings.Join(postBodyParts, "&"),
		"requestUri":        "http://localhost",
		"returnSecureToken": true,
	})

	var out struct {
		LocalID      string `json:"localId"`
		Email        string `json:"email"`
		DisplayName  string `json:"displayName"`
		IDToken      string `json:"idToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    string `json:"expiresIn"`
	}
	if err := c.do(ctx, c.signInWithIdpURL, "application/json", bytes.NewReader(body), &out); err != nil {
		return nil, err
	}

	return &SignInResult{
		UID:         out.LocalID,
		Email:       out.Email,
		DisplayName: out.DisplayName,
		Tokens: Tokens{
			IDToken:      out.IDToken,
			RefreshToken: out.RefreshToken,
			ExpiresIn:    atoiOr(out.ExpiresIn, 3600),
		},
	}, nil
}

// RefreshIDToken exchanges a refresh token for a fresh ID token.
func (c *IdentityClient) RefreshIDToken(ctx context.Context, refreshToken string) (uid string, tokens *Tokens, err error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	var out struct {
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    string `json:"expires_in"`
		UserID       string `json:"user_id"`
	}
	if err := c.do(ctx, c.refreshURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()), &out); err != nil {
		return "", nil, err
	}

	return out.UserID, &Tokens{
		IDToken:      out.IDToken,
		RefreshToken: out.RefreshToken,
		ExpiresIn:    atoiOr(out.ExpiresIn, 3600),
	}, nil
}

func (c *IdentityClient) do(ctx context.Context, endpoint, contentType string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"?key="+url.QueryEscape(c.apiKey), body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("identity toolkit request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error.Message == "" {
			e.Error.Message = http.StatusText(resp.StatusCode)
		}
		return &IdentityError{Status: resp.StatusCode, Message: e.Error.Message}
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func atoiOr(s string, fallback int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return fallback
}
