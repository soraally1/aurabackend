package osrm

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client provides access to OSRM routing API.
type Client struct {
	baseURL string
	timeout time.Duration
}

// NewClient creates a new OSRM client.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = "http://router.project-osrm.org"
	}
	return &Client{
		baseURL: baseURL,
		timeout: 10 * time.Second,
	}
}

// Route represents a route response from OSRM.
type Route struct {
	Distance float64        `json:"distance"` // meters
	Duration float64        `json:"duration"` // seconds
	Geometry Geometry       `json:"geometry"`
	Legs     []Leg          `json:"legs"`
}

// Geometry represents the encoded polyline.
type Geometry struct {
	Coordinates [][2]float64 `json:"coordinates"` // [lon, lat]
	Type        string       `json:"type"`
}

// Leg represents a segment of the route.
type Leg struct {
	Distance float64 `json:"distance"`
	Duration float64 `json:"duration"`
	Steps    []Step  `json:"steps"`
}

// Step represents a navigation step.
type Step struct {
	Distance    float64      `json:"distance"`
	Duration    float64      `json:"duration"`
	Name        string       `json:"name"`
	Instruction string       `json:"instruction,omitempty"`
	Maneuver    Maneuver     `json:"maneuver,omitempty"`
	Geometry    Geometry     `json:"geometry"`
}

// Maneuver represents turn-by-turn instructions.
type Maneuver struct {
	BearingAfter  float64 `json:"bearing_after"`
	BearingBefore float64 `json:"bearing_before"`
	Type          string  `json:"type"` // "turn", "new name", "depart", "arrive", etc.
	Modifier      string  `json:"modifier"` // "left", "right", "straight", etc.
	Location      [2]float64 `json:"location"`
}

// RouteResponse is the full response from OSRM.
type RouteResponse struct {
	Code   string  `json:"code"`
	Routes []Route `json:"routes"`
}

// GetRoute calculates a route between coordinates.
// coords: sequence of [lon, lat] pairs (at least 2)
func (c *Client) GetRoute(coords [][2]float64, mode string) (*Route, error) {
	if len(coords) < 2 {
		return nil, fmt.Errorf("at least 2 coordinates required")
	}

	// Build coordinate string: lon,lat;lon,lat;...
	coordParts := make([]string, len(coords))
	for i, coord := range coords {
		coordParts[i] = fmt.Sprintf("%.6f,%.6f", coord[0], coord[1])
	}
	coordStr := strings.Join(coordParts, ";")

	// Determine profile: driving, walking, cycling
	profile := "driving"
	if mode == "walking" {
		profile = "foot"
	} else if mode == "cycling" {
		profile = "bike"
	}

	url := fmt.Sprintf("%s/route/v1/%s/%s?overview=full&steps=true&geometries=geojson",
		c.baseURL, profile, coordStr)

	client := &http.Client{Timeout: c.timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OSRM API error: %d - %s", resp.StatusCode, string(body))
	}

	var routeResp RouteResponse
	if err := json.NewDecoder(resp.Body).Decode(&routeResp); err != nil {
		return nil, err
	}

	if routeResp.Code != "Ok" || len(routeResp.Routes) == 0 {
		return nil, fmt.Errorf("OSRM error: no route found (code: %s)", routeResp.Code)
	}

	return &routeResp.Routes[0], nil
}
