package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"aurabackend/pkg/firebase"
	"aurabackend/pkg/hospitals"
	"aurabackend/pkg/httpx"
)

var routeClient = hospitals.NewRouteClient()

// Routes handles GET /api/v1/routes (and /api/routes, /routes).
// Query params:
//   - origin_lat: user origin latitude (required)
//   - origin_lon: user origin longitude (required)
//   - dest_lat: destination hospital latitude (required)
//   - dest_lon: destination hospital longitude (required)
func Routes(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	if r.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	q := r.URL.Query()

	originLatStr := strings.TrimSpace(q.Get("origin_lat"))
	originLonStr := strings.TrimSpace(q.Get("origin_lon"))
	destLatStr := strings.TrimSpace(q.Get("dest_lat"))
	destLonStr := strings.TrimSpace(q.Get("dest_lon"))

	if originLatStr == "" || originLonStr == "" || destLatStr == "" || destLonStr == "" {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", "Query parameters origin_lat, origin_lon, dest_lat, and dest_lon are all required")
		return
	}

	originLat, err := strconv.ParseFloat(originLatStr, 64)
	if err != nil || originLat < -90 || originLat > 90 {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", "origin_lat must be a valid latitude between -90 and 90")
		return
	}

	originLon, err := strconv.ParseFloat(originLonStr, 64)
	if err != nil || originLon < -180 || originLon > 180 {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", "origin_lon must be a valid longitude between -180 and 180")
		return
	}

	destLat, err := strconv.ParseFloat(destLatStr, 64)
	if err != nil || destLat < -90 || destLat > 90 {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", "dest_lat must be a valid latitude between -90 and 90")
		return
	}

	destLon, err := strconv.ParseFloat(destLonStr, 64)
	if err != nil || destLon < -180 || destLon > 180 {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", "dest_lon must be a valid longitude between -180 and 180")
		return
	}

	result, err := routeClient.FetchRoute(r.Context(), originLat, originLon, destLat, destLon)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "ROUTING_ERROR", fmt.Sprintf("Failed to compute route: %v", err))
		return
	}

	httpx.OK(w, http.StatusOK, result)
}

// NearbyHospitals handles GET /api/v1/hospitals/nearby (and /api/hospitals/nearby, /hospitals/nearby).
// Query params:
//   - lat: user latitude (required)
//   - lon: user longitude (required)
//   - radius_km: radius in km (optional, default: 15.0)
//   - limit: maximum results (optional, default: 10)
func NearbyHospitals(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	if r.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	q := r.URL.Query()

	latStr := strings.TrimSpace(q.Get("lat"))
	lonStr := strings.TrimSpace(q.Get("lon"))
	if latStr == "" || lonStr == "" {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", "Query parameters lat and lon are required")
		return
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil || lat < -90 || lat > 90 {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", "lat must be a valid latitude between -90 and 90")
		return
	}

	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil || lon < -180 || lon > 180 {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", "lon must be a valid longitude between -180 and 180")
		return
	}

	radiusKm := 15.0
	if rStr := strings.TrimSpace(q.Get("radius_km")); rStr != "" {
		if rParsed, err := strconv.ParseFloat(rStr, 64); err == nil && rParsed > 0 {
			radiusKm = rParsed
		}
	}

	limit := 10
	if lStr := strings.TrimSpace(q.Get("limit")); lStr != "" {
		if lParsed, err := strconv.Atoi(lStr); err == nil && lParsed > 0 {
			limit = lParsed
		}
	}

	store := hospitals.NewStore(c.Firestore, c.Config.HospitalsCollection)
	results, err := store.FindNearby(r.Context(), lat, lon, radiusKm, limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", fmt.Sprintf("Failed to fetch nearby hospitals: %v", err))
		return
	}

	httpx.OK(w, http.StatusOK, results)
}

// ListHospitals handles GET /api/v1/hospitals (and /api/hospitals, /hospitals).
func ListHospitals(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	if r.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	store := hospitals.NewStore(c.Firestore, c.Config.HospitalsCollection)
	all, err := store.ListAll(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}

	// Auto-seed if collection empty
	if len(all) == 0 {
		curated := hospitals.CuratedSemarangHospitals()
		_, _ = store.BatchUpsert(r.Context(), curated)
		all = curated
	}

	// Optional query filter
	qName := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if qName != "" {
		filtered := make([]hospitals.Hospital, 0)
		for _, h := range all {
			if strings.Contains(strings.ToLower(h.Name), qName) || strings.Contains(strings.ToLower(h.Address), qName) {
				filtered = append(filtered, h)
			}
		}
		httpx.OK(w, http.StatusOK, filtered)
		return
	}

	httpx.OK(w, http.StatusOK, all)
}

// GetHospital handles GET /api/v1/hospitals/{id}.
func GetHospital(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	if r.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		// Fallback for path parsing if PathValue not set
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) > 0 {
			id = parts[len(parts)-1]
		}
	}

	if id == "" || id == "hospitals" || id == "nearby" {
		httpx.Error(w, http.StatusBadRequest, "INVALID_INPUT", "Hospital ID required")
		return
	}

	store := hospitals.NewStore(c.Firestore, c.Config.HospitalsCollection)
	h, err := store.Get(r.Context(), id)
	if err != nil {
		if err == hospitals.ErrNotFound {
			httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Hospital not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}

	httpx.OK(w, http.StatusOK, h)
}

// SeedHospitals handles POST /api/v1/hospitals/seed (and /api/hospitals/seed, /hospitals/seed).
func SeedHospitals(w http.ResponseWriter, r *http.Request, c *firebase.Clients) {
	if r.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	store := hospitals.NewStore(c.Firestore, c.Config.HospitalsCollection)
	curated := hospitals.CuratedSemarangHospitals()

	count, err := store.BatchUpsert(r.Context(), curated)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", fmt.Sprintf("Failed to seed hospitals: %v", err))
		return
	}

	httpx.OK(w, http.StatusOK, map[string]any{
		"message":        "Hospitals seeded successfully",
		"hospitals_count": count,
		"collection":     c.Config.HospitalsCollection,
	})
}
