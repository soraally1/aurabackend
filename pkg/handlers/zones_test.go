package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseNearbyQuery(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantLat    float64
		wantLon    float64
		wantRadius float64
		wantErr    bool
	}{
		{
			name:       "valid standard query with custom radius",
			query:      "lat=-6.2088&lon=106.8229&radius=10.5",
			wantLat:    -6.2088,
			wantLon:    106.8229,
			wantRadius: 10.5,
			wantErr:    false,
		},
		{
			name:       "valid standard query default radius",
			query:      "lat=-6.2088&lon=106.8229",
			wantLat:    -6.2088,
			wantLon:    106.8229,
			wantRadius: 5.0,
			wantErr:    false,
		},
		{
			name:    "missing lat",
			query:   "lon=106.8229",
			wantErr: true,
		},
		{
			name:    "missing lon",
			query:   "lat=-6.2088",
			wantErr: true,
		},
		{
			name:    "invalid lat range",
			query:   "lat=95.0&lon=106.8229",
			wantErr: true,
		},
		{
			name:    "invalid lon range",
			query:   "lat=-6.2088&lon=200.0",
			wantErr: true,
		},
		{
			name:    "invalid radius above maximum",
			query:   "lat=-6.2088&lon=106.8229&radius=75.0",
			wantErr: true,
		},
		{
			name:    "invalid radius negative",
			query:   "lat=-6.2088&lon=106.8229&radius=-2",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/zones/nearby?"+tt.query, nil)
			lat, lon, rad, err := parseNearbyQuery(req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseNearbyQuery() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if lat != tt.wantLat || lon != tt.wantLon || rad != tt.wantRadius {
					t.Errorf("got lat=%.4f lon=%.4f rad=%.2f, want lat=%.4f lon=%.4f rad=%.2f",
						lat, lon, rad, tt.wantLat, tt.wantLon, tt.wantRadius)
				}
			}
		})
	}
}
