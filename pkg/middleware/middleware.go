// Package middleware contains HTTP middleware.
package middleware

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"firebase.google.com/go/v4/auth"

	"aurabackend/pkg/httpx"
)

type ctxKey struct{}

// RequireAuth verifies the "Authorization: Bearer <idToken>" header using the
// Firebase Admin SDK and stores the decoded token in the request context.
// Tokens revoked via logout are rejected.
func RequireAuth(client *auth.Client, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		scheme, raw, ok := strings.Cut(header, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(raw) == "" {
			httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or malformed Authorization header")
			return
		}

		token, err := client.VerifyIDTokenAndCheckRevoked(r.Context(), strings.TrimSpace(raw))
		if err != nil {
			switch {
			case auth.IsIDTokenRevoked(err):
				httpx.Error(w, http.StatusUnauthorized, "TOKEN_REVOKED", "Session has been revoked, please log in again")
			case auth.IsUserDisabled(err):
				httpx.Error(w, http.StatusForbidden, "USER_DISABLED", "This account has been disabled")
			default:
				httpx.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "Invalid or expired token")
			}
			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, token)))
	})
}

// Token returns the verified Firebase token for the request, or nil.
func Token(ctx context.Context) *auth.Token {
	t, _ := ctx.Value(ctxKey{}).(*auth.Token)
	return t
}

// UID returns the authenticated user's UID, or "".
func UID(ctx context.Context) string {
	if t := Token(ctx); t != nil {
		return t.UID
	}
	return ""
}

// CORS adds CORS headers and answers preflight requests.
// allowed may contain "*" to allow any origin.
func CORS(allowed []string, next http.Handler) http.Handler {
	allowAll := slices.Contains(allowed, "*")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		h := w.Header()
		if origin != "" {
			switch {
			case allowAll:
				h.Set("Access-Control-Allow-Origin", "*")
			case slices.Contains(allowed, origin):
				h.Set("Access-Control-Allow-Origin", origin)
				h.Add("Vary", "Origin")
			}
		}

		if r.Method == http.MethodOptions {
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RewritePath restores the original request path on serverless platforms like Vercel.
func RewritePath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ResolvePath(r)
		next.ServeHTTP(w, r)
	})
}

// ResolvePath extracts the original path from __path query param, x-forwarded-uri,
// or x-matched-path, ensuring router matching succeeds.
func ResolvePath(r *http.Request) {
	// 1. Query parameter __path from vercel.json rewrite: /(.*) -> /api/index?__path=/$1
	if p := r.URL.Query().Get("__path"); p != "" {
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		setPath(r, p)

		// Clean up __path query parameter
		q := r.URL.Query()
		q.Del("__path")
		r.URL.RawQuery = q.Encode()
		return
	}

	// 2. Fallback to x-forwarded-uri header (Vercel proxy/edge original URI)
	if p := r.Header.Get("x-forwarded-uri"); p != "" {
		if u, err := url.Parse(p); err == nil && u.Path != "" {
			setPath(r, u.Path)
			return
		}
		setPath(r, p)
		return
	}

	// 3. Fallback to x-matched-path only if valid and not an internal function entrypoint
	if p := r.Header.Get("x-matched-path"); p != "" {
		if p != "/api/index" && p != "/api" && p != "/go" && p != "/" {
			if u, err := url.Parse(p); err == nil && u.Path != "" {
				setPath(r, u.Path)
				return
			}
			setPath(r, p)
			return
		}
	}
}

func setPath(r *http.Request, p string) {
	r.URL.Path = p
	r.URL.RawPath = p
	r.RequestURI = p
}

// Recover turns panics into 500 responses instead of crashing the function.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %s %s: %v", r.Method, r.URL.Path, rec)
				httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
