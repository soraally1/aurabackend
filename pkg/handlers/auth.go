package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"unicode/utf8"

	"firebase.google.com/go/v4/auth"

	"aurabackend/pkg/firebase"
	"aurabackend/pkg/httpx"
	"aurabackend/pkg/middleware"
	"aurabackend/pkg/users"
)

const (
	minPasswordLen = 8
	maxPasswordLen = 128
	maxNameLen     = 100
)

type registerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type authResponse struct {
	User   *users.User      `json:"user"`
	Tokens *firebase.Tokens `json:"tokens,omitempty"`
}

// Register creates a Firebase Auth user, stores the profile in Firestore and
// returns a signed-in session.
//
// POST /api/auth/register  {"name","email","password"}
func Register(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	var req registerRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if msg := validateRegister(req); msg != "" {
		httpx.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", msg)
		return
	}

	ctx := r.Context()
	record, err := c.Auth.CreateUser(ctx, (&auth.UserToCreate{}).
		Email(req.Email).
		Password(req.Password).
		DisplayName(req.Name))
	if err != nil {
		switch {
		case auth.IsEmailAlreadyExists(err):
			httpx.Error(w, http.StatusConflict, "EMAIL_EXISTS", "An account with this email already exists")
		default:
			log.Printf("register: create auth user: %v", err)
			httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Could not create account")
		}
		return
	}

	profile := &users.User{UID: record.UID, Email: req.Email, Name: req.Name}
	store := users.NewStore(c.Firestore, c.Config.UsersCollection)
	if err := store.Create(ctx, profile); err != nil {
		log.Printf("register: create profile for %s: %v", record.UID, err)
		// Roll back so the email isn't stuck in a half-registered state.
		if delErr := c.Auth.DeleteUser(context.WithoutCancel(ctx), record.UID); delErr != nil {
			log.Printf("register: rollback delete %s: %v", record.UID, delErr)
		}
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Could not create account")
		return
	}

	// Sign the new user in so the client gets tokens immediately.
	session, err := c.Identity.SignInWithPassword(ctx, req.Email, req.Password)
	if err != nil {
		// The account exists; the client can fall back to /login.
		log.Printf("register: sign-in after create for %s: %v", record.UID, err)
		httpx.OK(w, http.StatusCreated, authResponse{User: profile})
		return
	}

	httpx.OK(w, http.StatusCreated, authResponse{User: profile, Tokens: &session.Tokens})
}

// Login verifies email/password and returns a session.
//
// POST /api/auth/login  {"email","password"}
func Login(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	var req loginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" {
		httpx.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "email and password are required")
		return
	}

	ctx := r.Context()
	session, err := c.Identity.SignInWithPassword(ctx, req.Email, req.Password)
	if err != nil {
		writeIdentityError(w, "login", err)
		return
	}

	store := users.NewStore(c.Firestore, c.Config.UsersCollection)
	profile, err := loadOrBackfillProfile(ctx, c, store, session.UID)
	if err != nil {
		log.Printf("login: load profile %s: %v", session.UID, err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Could not load profile")
		return
	}

	if t, err := store.TouchLogin(ctx, session.UID); err != nil {
		log.Printf("login: touch %s: %v", session.UID, err) // non-fatal
	} else {
		profile.LastLoginAt = &t
	}

	httpx.OK(w, http.StatusOK, authResponse{User: profile, Tokens: &session.Tokens})
}

// Refresh exchanges a refresh token for a new ID token.
//
// POST /api/auth/refresh  {"refreshToken"}
func Refresh(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	var req refreshRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if strings.TrimSpace(req.RefreshToken) == "" {
		httpx.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "refreshToken is required")
		return
	}

	_, tokens, err := c.Identity.RefreshIDToken(r.Context(), req.RefreshToken)
	if err != nil {
		writeIdentityError(w, "refresh", err)
		return
	}
	httpx.OK(w, http.StatusOK, map[string]any{"tokens": tokens})
}

// Me returns the authenticated user's profile.
//
// GET /api/auth/me  (Authorization: Bearer <idToken>)
func Me(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	uid := middleware.UID(r.Context())
	store := users.NewStore(c.Firestore, c.Config.UsersCollection)

	profile, err := loadOrBackfillProfile(r.Context(), c, store, uid)
	if err != nil {
		log.Printf("me: load profile %s: %v", uid, err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Could not load profile")
		return
	}
	httpx.OK(w, http.StatusOK, map[string]any{"user": profile})
}

// Logout revokes all refresh tokens for the user, signing them out everywhere.
// Existing ID tokens are rejected by the auth middleware's revocation check.
//
// POST /api/auth/logout  (Authorization: Bearer <idToken>)
func Logout(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	uid := middleware.UID(r.Context())
	if err := c.Auth.RevokeRefreshTokens(r.Context(), uid); err != nil {
		log.Printf("logout: revoke %s: %v", uid, err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Could not log out")
		return
	}
	httpx.OK(w, http.StatusOK, map[string]any{"message": "Logged out"})
}

// loadOrBackfillProfile returns the Firestore profile, creating it from the
// Auth record if missing (e.g. users added via the Firebase console).
func loadOrBackfillProfile(ctx context.Context, c *firebase.Clients, store *users.Store, uid string) (*users.User, error) {
	profile, err := store.Get(ctx, uid)
	if err == nil {
		return profile, nil
	}
	if !errors.Is(err, users.ErrNotFound) {
		return nil, err
	}

	record, err := c.Auth.GetUser(ctx, uid)
	if err != nil {
		return nil, err
	}
	profile = &users.User{UID: uid, Email: record.Email, Name: record.DisplayName}
	if err := store.Create(ctx, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

func validateRegister(req registerRequest) string {
	switch {
	case req.Name == "":
		return "name is required"
	case utf8.RuneCountInString(req.Name) > maxNameLen:
		return "name must be at most 100 characters"
	case req.Email == "":
		return "email is required"
	case !isValidEmail(req.Email):
		return "email is invalid"
	case len(req.Password) < minPasswordLen:
		return "password must be at least 8 characters"
	case len(req.Password) > maxPasswordLen:
		return "password must be at most 128 characters"
	}
	return ""
}

func isValidEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s
}

// writeIdentityError maps Firebase Auth REST errors to API responses without
// leaking whether an email is registered.
func writeIdentityError(w http.ResponseWriter, op string, err error) {
	var ie *firebase.IdentityError
	if !errors.As(err, &ie) {
		log.Printf("%s: %v", op, err)
		httpx.Error(w, http.StatusBadGateway, "UPSTREAM_ERROR", "Authentication service unavailable")
		return
	}

	switch ie.Code() {
	case "EMAIL_NOT_FOUND", "INVALID_PASSWORD", "INVALID_LOGIN_CREDENTIALS", "INVALID_EMAIL":
		httpx.Error(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
	case "USER_DISABLED":
		httpx.Error(w, http.StatusForbidden, "USER_DISABLED", "This account has been disabled")
	case "TOO_MANY_ATTEMPTS_TRY_LATER":
		httpx.Error(w, http.StatusTooManyRequests, "TOO_MANY_ATTEMPTS", "Too many attempts, please try again later")
	case "TOKEN_EXPIRED", "INVALID_REFRESH_TOKEN", "USER_NOT_FOUND", "INVALID_GRANT_TYPE", "MISSING_REFRESH_TOKEN":
		httpx.Error(w, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "Session expired, please log in again")
	default:
		// e.g. PASSWORD_LOGIN_DISABLED, OPERATION_NOT_ALLOWED, invalid API key.
		log.Printf("%s: %v", op, ie)
		httpx.Error(w, http.StatusInternalServerError, "AUTH_CONFIG_ERROR", "Authentication is misconfigured")
	}
}
