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
	Name         string   `json:"name"`
	Email        string   `json:"email"`
	Password     string   `json:"password"`
	Usia         *int     `json:"usia"`
	TinggiBadan  *float64 `json:"tinggi_badan"`
	BeratBadan   *float64 `json:"berat_badan"`
	WaktuDiJalan *float64 `json:"waktu_di_jalan"`
}

type updateProfileRequest struct {
	Name         *string  `json:"name"`
	Usia         *int     `json:"usia"`
	TinggiBadan  *float64 `json:"tinggi_badan"`
	BeratBadan   *float64 `json:"berat_badan"`
	WaktuDiJalan *float64 `json:"waktu_di_jalan"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type googleLoginRequest struct {
	IDToken           string   `json:"idToken"`
	IDTokenSnake      string   `json:"id_token"`
	AccessToken       string   `json:"accessToken"`
	AccessTokenSnake  string   `json:"access_token"`
	RefreshToken      string   `json:"refreshToken"`
	RefreshTokenSnake string   `json:"refresh_token"`
	Name              *string  `json:"name"`
	Usia              *int     `json:"usia"`
	TinggiBadan       *float64 `json:"tinggi_badan"`
	BeratBadan        *float64 `json:"berat_badan"`
	WaktuDiJalan      *float64 `json:"waktu_di_jalan"`
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

	profile := &users.User{
		UID:          record.UID,
		Email:        req.Email,
		Name:         req.Name,
		Usia:         *req.Usia,
		TinggiBadan:  *req.TinggiBadan,
		BeratBadan:   *req.BeratBadan,
		WaktuDiJalan: *req.WaktuDiJalan,
	}
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

// GoogleLogin handles sign-in with Google via Firebase Auth.
// It accepts either a Firebase ID token (from client-side Firebase Auth SDK)
// or a Google OAuth ID token / access token (which is exchanged via Identity Toolkit).
//
// POST /api/auth/google  {"idToken":"..."} or {"accessToken":"..."}
func GoogleLogin(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	var req googleLoginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	if req.IDToken == "" && req.IDTokenSnake != "" {
		req.IDToken = req.IDTokenSnake
	}
	if req.AccessToken == "" && req.AccessTokenSnake != "" {
		req.AccessToken = req.AccessTokenSnake
	}
	if req.RefreshToken == "" && req.RefreshTokenSnake != "" {
		req.RefreshToken = req.RefreshTokenSnake
	}

	req.IDToken = strings.TrimSpace(req.IDToken)
	req.AccessToken = strings.TrimSpace(req.AccessToken)

	if req.IDToken == "" && req.AccessToken == "" {
		httpx.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "idToken or accessToken is required")
		return
	}

	if msg := validateOptionalProfile(req.Name, req.Usia, req.TinggiBadan, req.BeratBadan, req.WaktuDiJalan); msg != "" {
		httpx.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", msg)
		return
	}

	ctx := r.Context()
	var (
		uid    string
		tokens *firebase.Tokens
	)

	// 1. If idToken is provided, first attempt to verify it as a Firebase ID token
	// (common when the client uses the Firebase Client SDK with signInWithPopup).
	if req.IDToken != "" {
		if verifiedToken, err := c.Auth.VerifyIDToken(ctx, req.IDToken); err == nil {
			uid = verifiedToken.UID
			tokens = &firebase.Tokens{
				IDToken:      req.IDToken,
				RefreshToken: req.RefreshToken,
				ExpiresIn:    3600,
			}
		}
	}

	// 2. If not a Firebase ID token (e.g. Google OAuth ID token / access token),
	// exchange it with Firebase Auth via Identity Toolkit.
	if uid == "" {
		session, err := c.Identity.SignInWithGoogle(ctx, req.IDToken, req.AccessToken)
		if err != nil {
			writeIdentityError(w, "google login", err)
			return
		}
		uid = session.UID
		tokens = &session.Tokens
	}

	store := users.NewStore(c.Firestore, c.Config.UsersCollection)
	profile, err := loadOrBackfillProfile(ctx, c, store, uid)
	if err != nil {
		log.Printf("google login: load profile %s: %v", uid, err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Could not load profile")
		return
	}

	// Apply any optional profile fields sent in the request.
	updates := make(map[string]any)
	if req.Name != nil {
		cleanName := strings.TrimSpace(*req.Name)
		if cleanName != "" {
			updates["name"] = cleanName
			profile.Name = cleanName
			if _, err := c.Auth.UpdateUser(ctx, uid, (&auth.UserToUpdate{}).DisplayName(cleanName)); err != nil {
				log.Printf("google login: sync auth displayName for %s: %v", uid, err)
			}
		}
	}
	if req.Usia != nil {
		updates["usia"] = *req.Usia
		profile.Usia = *req.Usia
	}
	if req.TinggiBadan != nil {
		updates["tinggi_badan"] = *req.TinggiBadan
		profile.TinggiBadan = *req.TinggiBadan
	}
	if req.BeratBadan != nil {
		updates["berat_badan"] = *req.BeratBadan
		profile.BeratBadan = *req.BeratBadan
	}
	if req.WaktuDiJalan != nil {
		updates["waktu_di_jalan"] = *req.WaktuDiJalan
		profile.WaktuDiJalan = *req.WaktuDiJalan
	}
	if len(updates) > 0 {
		if err := store.Update(ctx, uid, updates); err != nil {
			log.Printf("google login: store update %s: %v", uid, err)
		}
	}

	if t, err := store.TouchLogin(ctx, uid); err != nil {
		log.Printf("google login: touch %s: %v", uid, err) // non-fatal
	} else {
		profile.LastLoginAt = &t
	}

	httpx.OK(w, http.StatusOK, authResponse{User: profile, Tokens: tokens})
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

// UpdateProfile updates the authenticated user's profile.
//
// PUT /api/auth/profile  (Authorization: Bearer <idToken>)
func UpdateProfile(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	var req updateProfileRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	if msg := validateUpdateProfile(req); msg != "" {
		httpx.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", msg)
		return
	}

	uid := middleware.UID(r.Context())
	store := users.NewStore(c.Firestore, c.Config.UsersCollection)

	// Ensure profile exists in Firestore (backfilling if created externally).
	if _, err := loadOrBackfillProfile(r.Context(), c, store, uid); err != nil {
		log.Printf("update profile: load/backfill %s: %v", uid, err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Could not load profile")
		return
	}

	updates := make(map[string]any)
	if req.Name != nil {
		cleanName := strings.TrimSpace(*req.Name)
		updates["name"] = cleanName
		if _, err := c.Auth.UpdateUser(r.Context(), uid, (&auth.UserToUpdate{}).DisplayName(cleanName)); err != nil {
			log.Printf("update profile: sync auth displayName for %s: %v", uid, err)
		}
	}
	if req.Usia != nil {
		updates["usia"] = *req.Usia
	}
	if req.TinggiBadan != nil {
		updates["tinggi_badan"] = *req.TinggiBadan
	}
	if req.BeratBadan != nil {
		updates["berat_badan"] = *req.BeratBadan
	}
	if req.WaktuDiJalan != nil {
		updates["waktu_di_jalan"] = *req.WaktuDiJalan
	}

	if err := store.Update(r.Context(), uid, updates); err != nil {
		log.Printf("update profile: store update %s: %v", uid, err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Could not update profile")
		return
	}

	profile, err := store.Get(r.Context(), uid)
	if err != nil {
		log.Printf("update profile: get updated profile %s: %v", uid, err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Could not load updated profile")
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
// Auth record if missing (e.g. users added via the Firebase console or OAuth).
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
	name := strings.TrimSpace(record.DisplayName)
	if name == "" && record.Email != "" {
		name = strings.Split(record.Email, "@")[0]
	}
	profile = &users.User{UID: uid, Email: record.Email, Name: name}
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
	case req.Usia == nil:
		return "usia is required"
	case *req.Usia <= 0 || *req.Usia > 150:
		return "usia must be between 1 and 150"
	case req.TinggiBadan == nil:
		return "tinggi_badan is required"
	case *req.TinggiBadan <= 0 || *req.TinggiBadan > 300:
		return "tinggi_badan must be between 1 and 300 cm"
	case req.BeratBadan == nil:
		return "berat_badan is required"
	case *req.BeratBadan <= 0 || *req.BeratBadan > 500:
		return "berat_badan must be between 1 and 500 kg"
	case req.WaktuDiJalan == nil:
		return "waktu_di_jalan is required"
	case *req.WaktuDiJalan < 0 || *req.WaktuDiJalan > 24:
		return "waktu_di_jalan must be between 0 and 24 hours"
	}
	return ""
}

func validateUpdateProfile(req updateProfileRequest) string {
	if req.Name == nil && req.Usia == nil && req.TinggiBadan == nil && req.BeratBadan == nil && req.WaktuDiJalan == nil {
		return "at least one field must be provided"
	}
	return validateOptionalProfile(req.Name, req.Usia, req.TinggiBadan, req.BeratBadan, req.WaktuDiJalan)
}

func validateOptionalProfile(name *string, usia *int, tinggi *float64, berat *float64, waktu *float64) string {
	if name != nil {
		cleanName := strings.TrimSpace(*name)
		if cleanName == "" {
			return "name cannot be empty"
		}
		if utf8.RuneCountInString(cleanName) > maxNameLen {
			return "name must be at most 100 characters"
		}
	}
	if usia != nil {
		if *usia <= 0 || *usia > 150 {
			return "usia must be between 1 and 150"
		}
	}
	if tinggi != nil {
		if *tinggi <= 0 || *tinggi > 300 {
			return "tinggi_badan must be between 1 and 300 cm"
		}
	}
	if berat != nil {
		if *berat <= 0 || *berat > 500 {
			return "berat_badan must be between 1 and 500 kg"
		}
	}
	if waktu != nil {
		if *waktu < 0 || *waktu > 24 {
			return "waktu_di_jalan must be between 0 and 24 hours"
		}
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
	case "EMAIL_NOT_FOUND", "INVALID_PASSWORD", "INVALID_LOGIN_CREDENTIALS", "INVALID_EMAIL",
		"INVALID_IDP_RESPONSE", "INVALID_ID_TOKEN", "INVALID_CREDENTIAL", "FEDERATED_USER_ID_ALREADY_LINKED":
		httpx.Error(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid credentials or token")
	case "USER_DISABLED":
		httpx.Error(w, http.StatusForbidden, "USER_DISABLED", "This account has been disabled")
	case "OPERATION_NOT_ALLOWED":
		log.Printf("%s: %v", op, ie)
		httpx.Error(w, http.StatusForbidden, "PROVIDER_NOT_ALLOWED", "Google sign-in is not enabled in Firebase Authentication")
	case "TOO_MANY_ATTEMPTS_TRY_LATER":
		httpx.Error(w, http.StatusTooManyRequests, "TOO_MANY_ATTEMPTS", "Too many attempts, please try again later")
	case "TOKEN_EXPIRED", "INVALID_REFRESH_TOKEN", "USER_NOT_FOUND", "INVALID_GRANT_TYPE", "MISSING_REFRESH_TOKEN":
		httpx.Error(w, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "Session expired, please log in again")
	default:
		// e.g. PASSWORD_LOGIN_DISABLED, invalid API key.
		log.Printf("%s: %v", op, ie)
		httpx.Error(w, http.StatusInternalServerError, "AUTH_CONFIG_ERROR", "Authentication is misconfigured")
	}
}
