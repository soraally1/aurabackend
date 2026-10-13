package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	handler "aurabackend/api"
)

func TestHandlerRouting(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		target     string
		headers    map[string]string
		wantStatus int
	}{
		{
			name:       "Root URL direct",
			method:     http.MethodGet,
			target:     "/",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Health check direct",
			method:     http.MethodGet,
			target:     "/api/health",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Vercel rewrite via x-matched-path to health",
			method:     http.MethodGet,
			target:     "/api/index",
			headers:    map[string]string{"x-matched-path": "/api/health"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "Vercel rewrite via __path to health",
			method:     http.MethodGet,
			target:     "/api/index?__path=/api/health",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Vercel rewrite via x-matched-path to root",
			method:     http.MethodGet,
			target:     "/api/index",
			headers:    map[string]string{"x-matched-path": "/"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "Vercel rewrite via __path to root",
			method:     http.MethodGet,
			target:     "/api/index?__path=/",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Vercel rewrite via __path even if x-matched-path is /api/index",
			method:     http.MethodGet,
			target:     "/api/index?__path=/api/health",
			headers:    map[string]string{"x-matched-path": "/api/index"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "Unknown route returns 404",
			method:     http.MethodGet,
			target:     "/api/nonexistent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()

			handler.Handler(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d. Body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}

			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("failed to decode JSON response: %v", err)
			}
		})
	}
}
