package navigation

// CalculateRouteRequest represents the request for POST /navigation/calculate-route.
type CalculateRouteRequest struct {
	Origin        Coordinate   `json:"origin"`
	Destination   Coordinate   `json:"destination"`
	Waypoints     []Coordinate `json:"waypoints,omitempty"`
	Mode          string       `json:"mode,omitempty"` // "driving" or "walking"
	AvoidTolls    bool         `json:"avoid_tolls,omitempty"`
	AvoidHighways bool         `json:"avoid_highways,omitempty"`
}

// Coordinate represents a geographic point.
type Coordinate struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// RouteStep represents a turn-by-turn navigation step.
type RouteStep struct {
	Name     string  `json:"name"`
	Distance float64 `json:"distance"`
	Duration float64 `json:"duration"`
	Modifier string  `json:"modifier"`
	Type     string  `json:"type"`
}

// RouteResult holds the parsed navigation response.
type RouteResult struct {
	Code     string    `json:"code"`
	Features []Feature `json:"features,omitempty"`
	Routes   []Route   `json:"routes,omitempty"`
	Message  string    `json:"message,omitempty"`
}

// Feature represents a GeoJSON feature in the route response.
type Feature struct {
	Type       string    `json:"type"`
	Geometry   *Geometry `json:"geometry,omitempty"`
	Properties any       `json:"properties,omitempty"`
}

// Geometry represents GeoJSON geometry (coordinates, type, etc.).
type Geometry struct {
	Type        string      `json:"type"`
	Coordinates [][]float64 `json:"coordinates"`
}

// Route represents a single route option from the routing service.
type Route struct {
	Distance float64 `json:"distance"`
	Duration float64 `json:"duration"`
	Geometry struct {
		Coordinates [][]float64 `json:"coordinates"`
		Type        string      `json:"type"`
	} `json:"geometry"`
	Legs []struct {
		Steps []struct {
			Name     string  `json:"name"`
			Distance float64 `json:"distance"`
			Duration float64 `json:"duration"`
			Maneuver struct {
				Modifier string `json:"modifier"`
				Type     string `json:"type"`
			} `json:"maneuver"`
		} `json:"steps"`
	} `json:"legs"`
}
