# Navigation & Tracking API Implementation Guide

This document describes the implementation of 5 REST API endpoints for in-app navigation and GPS tracking with health analysis.

## Overview

All endpoints are implemented in Go using the Gin-compatible HTTP router and Firestore for persistence. The APIs follow the specification in `backend-api.md`.

## Implemented Endpoints

### 1. POST /api/v1/navigation/calculate-route
**Package**: `pkg/navigation`  
**Handler**: `handlers.CalculateRoute`

Calculates routes between two points with turn-by-turn navigation, traffic segments, and health warnings based on pollution zones.

**Key Features**:
- Validates coordinate ranges (Indonesia bounds)
- Supports origin, destination, and optional waypoints
- Returns mock route data with realistic Semarang locations (can be extended to call real routing APIs)
- Includes traffic segments with congestion severity
- Integrates with pollution zones to generate health warnings
- Maps markers for visualization

**Request Example**:
```json
{
  "origin": {"latitude": -6.9932, "longitude": 110.4203},
  "destination": {"latitude": -7.0015, "longitude": 110.4128},
  "mode": "driving",
  "avoid_tolls": false
}
```

**Response Example**:
```json
{
  "success": true,
  "data": {
    "route_id": "route_20261115003845",
    "distance_meters": 2450,
    "duration_seconds": 480,
    "coordinates": [[110.4203, -6.9932], ...],
    "steps": [...],
    "traffic_segments": [...],
    "health_warnings": [...],
    "map_markers": [...]
  }
}
```

**Response Fields**:
- `route_id`: Unique route identifier for caching
- `distance_meters`: Total route distance in meters
- `duration_seconds`: Total travel time (normal traffic)
- `coordinates`: Array of [lon, lat] pairs for polyline
- `steps`: Turn-by-turn navigation instructions with maneuvers
- `traffic_segments`: Areas with congestion and estimated delays
- `health_warnings`: Environmental hazards (pollution, traffic) with health risks and recommendations
- `map_markers`: Markers for map display

---

### 2. POST /api/v1/tracking/start-session
**Package**: `pkg/tracking`  
**Handler**: `handlers.StartSession`  
**Auth**: Protected (requires Firebase token)

Creates a new active tracking session for GPS tracking.

**Request Example**:
```json
{
  "session_name": "Pulang Kantor",
  "start_location": {
    "latitude": -6.9932,
    "longitude": 110.4203,
    "timestamp": "2025-01-15T08:00:00Z"
  }
}
```

**Response Example**:
```json
{
  "success": true,
  "data": {
    "session_id": "session_user_abc123_1705310400",
    "started_at": "2025-01-15T08:00:00Z",
    "status": "active"
  }
}
```

**Key Features**:
- Generates unique session ID: `session_{user_id}_{unix_timestamp}`
- Stores session in Firestore with `status: "active"`
- Returns session ID for use in subsequent sync and stop operations
- User ID extracted from Firebase auth token

---

### 3. POST /api/v1/tracking/sync-waypoints
**Package**: `pkg/tracking`  
**Handler**: `handlers.SyncWaypoints`  
**Auth**: Protected (requires Firebase token)

Syncs GPS waypoints collected since the last sync during an active session.

**Request Example**:
```json
{
  "session_id": "session_user_abc123_1705310400",
  "waypoints": [
    {
      "latitude": -6.9932,
      "longitude": 110.4203,
      "timestamp": "2025-01-15T08:00:00Z",
      "accuracy_meters": 12,
      "speed_mps": 5.2,
      "heading": 145
    },
    {
      "latitude": -6.9940,
      "longitude": 110.4210,
      "timestamp": "2025-01-15T08:05:00Z",
      "accuracy_meters": 8,
      "speed_mps": 3.8,
      "heading": 152
    }
  ],
  "is_final": false
}
```

**Response Example**:
```json
{
  "success": true,
  "data": {
    "synced_count": 2,
    "session_id": "session_user_abc123_1705310400",
    "total_waypoints": 25,
    "last_sync_at": "2025-01-15T08:05:30Z"
  }
}
```

**Processing Pipeline**:
1. **Validation**: Verifies session exists, is active, and belongs to user
2. **Classification**: Classifies each waypoint as "indoor" or "outdoor"
   - Indoor: Distance < 60m from previous waypoint AND stayed > 20 minutes
   - Outdoor: User is moving
3. **Zone Enrichment**: Matches waypoints with pollution zones within 1 km radius
   - Retrieves AQI, PM2.5, and zone name for each waypoint
4. **Persistence**: Appends processed waypoints to session in Firestore

**Waypoint Classification Logic**:
```
for each waypoint:
  distance = haversineDistance(prev_wp, current_wp)
  duration = current_wp.timestamp - prev_wp.timestamp
  
  if distance < 60m AND duration >= 20 minutes:
    classification = "indoor"
    is_stay = true
  else:
    classification = "outdoor"
    is_stay = false
```

---

### 4. POST /api/v1/tracking/stop-session
**Package**: `pkg/tracking`  
**Handler**: `handlers.StopSession`  
**Auth**: Protected (requires Firebase token)

Ends an active tracking session and generates exposure statistics with health analysis.

**Request Example**:
```json
{
  "session_id": "session_user_abc123_1705310400",
  "end_location": {
    "latitude": -7.0015,
    "longitude": 110.4128,
    "timestamp": "2025-01-15T09:30:00Z"
  }
}
```

**Response Example**:
```json
{
  "success": true,
  "data": {
    "session_id": "session_user_abc123_1705310400",
    "started_at": "2025-01-15T08:00:00Z",
    "ended_at": "2025-01-15T09:30:00Z",
    "duration_minutes": 90,
    "distance_km": 12.5,
    "stats": {
      "total_outdoor_minutes": 85,
      "total_indoor_minutes": 5,
      "average_outdoor_aqi": 132,
      "average_outdoor_pm25": 98,
      "max_aqi": 178,
      "worst_area_name": "Kaligawe Industrial Zone",
      "worst_area_aqi": 178,
      "waypoints_processed": 52
    },
    "analysis": {
      "impact_score": 72,
      "risk_level": "Sedang",
      "summary": "Anda terpapar polusi sedang-tinggi...",
      "recommendations": [
        "Gunakan pelembab kulit setelah tiba",
        "Cuci wajah dengan pembersih lembut",
        "Minum air putih minimal 500ml",
        "Hindari area Kaligawe di perjalanan esok hari..."
      ]
    }
  }
}
```

**Statistics Calculation**:
- `distance_km`: Haversine distance between consecutive waypoints
- `duration_minutes`: Total session duration
- `total_outdoor_minutes`: Sum of outdoor waypoint durations
- `total_indoor_minutes`: Sum of indoor waypoint durations
- `average_outdoor_aqi`: Mean AQI of outdoor waypoints
- `average_outdoor_pm25`: Mean PM2.5 of outdoor waypoints
- `max_aqi`: Maximum AQI reached during session
- `worst_area_name`: Zone name with highest AQI

**Health Analysis Algorithm**:
```
timeFactor = min(40, (outdoor_minutes / 120) * 40)
aqiFactor = min(50, (avg_outdoor_aqi / 200) * 50)
peakFactor = max(0, min(10, (max_aqi - 100) * 0.1))

impact_score = min(100, max(10, timeFactor + aqiFactor + peakFactor))

Risk Levels:
  impact_score < 38  → "Rendah"
  38 ≤ score < 68    → "Sedang"
  68 ≤ score ≤ 100   → "Tinggi"
```

**Recommendations** are generated based on risk level:
- **Rendah**: Maintain hydration, ventilate room, optional mask
- **Sedang**: Use medical mask, rinse face/nose, consume vitamin C, air purifier
- **Tinggi**: Mandatory N95/KF94 mask, nasal saline rinse, warm water + honey, HEPA air purifier

---

### 5. GET /api/v1/tracking/daily-history
**Package**: `pkg/tracking`  
**Handler**: `handlers.DailyHistory`  
**Auth**: Protected (requires Firebase token)

Fetches all tracking sessions and waypoints for a specific date (for replay and analysis).

**Query Parameters**:
- `date` (required): YYYY-MM-DD format (default: today)
- `user_id` (optional): Extracted from auth token

**Request Example**:
```
GET /api/v1/tracking/daily-history?date=2025-01-15&user_id=user_abc123
```

**Response Example**:
```json
{
  "success": true,
  "data": {
    "date": "2025-01-15",
    "sessions": [
      {
        "session_id": "session_user_abc123_1705310400",
        "session_name": "Pulang Kantor",
        "started_at": "2025-01-15T08:00:00Z",
        "ended_at": "2025-01-15T09:30:00Z",
        "duration_minutes": 90,
        "distance_km": 12.5,
        "waypoints": [
          {
            "latitude": -6.9932,
            "longitude": 110.4203,
            "timestamp": "2025-01-15T08:00:00Z",
            "classification": "outdoor",
            "is_stay": false,
            "aqi": 125,
            "pm2_5": 92,
            "zone_name": "Simpang Lima"
          }
        ]
      }
    ],
    "combined_waypoints": [...],
    "daily_stats": {
      "total_outdoor_minutes": 180,
      "total_indoor_minutes": 60,
      "average_outdoor_aqi": 138,
      "average_outdoor_pm25": 105,
      "max_aqi": 178,
      "worst_area_name": "Kaligawe Industrial Zone",
      "worst_area_aqi": 178,
      "waypoints_processed": 120
    }
  }
}
```

**Key Features**:
- Returns all sessions for the specified date (24-hour window)
- Aggregates waypoints from all sessions in `combined_waypoints`
- Calculates daily statistics across all sessions
- Returns empty `daily_stats` if no sessions found
- Supports date replay for historical analysis

---

## Database Schema

### Firestore Collections

#### `tracking_sessions`
Stores individual tracking sessions and their waypoints.

```json
{
  "id": "session_user_abc123_1705310400",
  "user_id": "user_abc123",
  "session_name": "Pulang Kantor",
  "status": "completed",
  "started_at": Timestamp,
  "ended_at": Timestamp,
  "start_location": {
    "latitude": float64,
    "longitude": float64,
    "timestamp": Timestamp
  },
  "end_location": {
    "latitude": float64,
    "longitude": float64,
    "timestamp": Timestamp
  },
  "waypoints": [
    {
      "latitude": float64,
      "longitude": float64,
      "timestamp": Timestamp,
      "accuracy_meters": float64,
      "speed_mps": float64,
      "heading": float64,
      "classification": "outdoor|indoor",
      "is_stay": boolean,
      "zone_name": string,
      "aqi": int,
      "pm2_5": float64
    }
  ],
  "stats": {
    "total_outdoor_minutes": float64,
    "total_indoor_minutes": float64,
    "average_outdoor_aqi": float64,
    "average_outdoor_pm25": float64,
    "max_aqi": int,
    "worst_area_name": string,
    "worst_area_aqi": int,
    "waypoints_processed": int,
    "distance_km": float64,
    "duration_minutes": float64
  },
  "analysis": {
    "impact_score": float64,
    "risk_level": "Rendah|Sedang|Tinggi",
    "summary": string,
    "recommendations": [string]
  },
  "created_at": Timestamp,
  "updated_at": Timestamp
}
```

---

## Project Structure

```
pkg/
├── tracking/
│   ├── model.go          # Data models (Session, Waypoint, Stats, etc.)
│   ├── store.go          # Firestore persistence layer
│   ├── analyzer.go       # Waypoint processing and health analysis
│   └── analyzer_test.go  # Unit tests for analyzer
├── navigation/
│   └── model.go          # Route response models and mock data generation
├── handlers/
│   ├── tracking.go       # HTTP handlers for tracking endpoints
│   ├── navigation.go     # HTTP handler for route calculation
│   └── ... (existing handlers)
└── ... (existing packages)
```

---

## Configuration

Add the following environment variables to `.env`:

```bash
# Firestore collection names
TRACKING_COLLECTION=tracking_sessions
ZONES_COLLECTION=environmental_zones
MOBILITY_COLLECTION=daily_mobility

# For future enhancements
GROQ_API_KEY=your_groq_api_key
```

---

## Integration with Existing Systems

### Zone Matching
Waypoints are enriched with pollution data by querying the `environmental_zones` collection within a 1 km radius using geohash-based queries in `zones.Store.FindNearby()`.

### Authentication
All protected endpoints (`/tracking/*`) require a valid Firebase ID token in the `Authorization: Bearer` header. The user ID is extracted from the token and validated to ensure users can only access their own sessions.

### Error Handling
Standard error responses follow the pattern:
```json
{
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable error message"
  }
}
```

Common error codes:
- `INVALID_INPUT`: Missing or malformed request data
- `INVALID_COORDINATES`: Out-of-bounds coordinates
- `SESSION_NOT_FOUND`: Session doesn't exist or is expired
- `SESSION_ALREADY_ENDED`: Cannot perform operation on completed session
- `FORBIDDEN`: Session belongs to different user
- `DATABASE_ERROR`: Firestore operation failed
- `METHOD_NOT_ALLOWED`: Wrong HTTP method

---

## Testing

Run unit tests for the tracking package:

```bash
go test -v ./pkg/tracking/
```

Tests cover:
- Haversine distance calculations
- Waypoint classification logic (indoor vs outdoor)
- Statistics aggregation
- Health analysis score generation

---

## Future Enhancements

1. **Real Routing API Integration**: Replace mock route generation with calls to Google Maps, OpenRouteService, or TomTom APIs
2. **Groq AI Integration**: Queue Groq API calls asynchronously for more sophisticated health analysis
3. **Traffic Data Caching**: Cache traffic API responses (expensive calls) for 15 minutes
4. **Historical Analytics**: Generate weekly/monthly exposure trends and alerts
5. **Geofencing**: Alert users when entering high-pollution zones
6. **Real-time Notifications**: Push notifications for health warnings during navigation
7. **Route Optimization**: Suggest pollution-aware routes that avoid worst areas

---

## Deployment Notes

- Ensure Firestore indexes are created for the `tracking_sessions` collection:
  - Composite index: `(user_id, started_at)` with `status` filter
- Set appropriate Firestore security rules to restrict access to user's own sessions
- Configure CORS headers to allow frontend domain
- Implement rate limiting on `/navigation/calculate-route` (10 req/min) and `/tracking/sync-waypoints` (100 req/day)

---

## API Rate Limits (Recommended)

Based on `backend-api.md`:

| Endpoint | Limit | Window |
|----------|-------|--------|
| POST /navigation/calculate-route | 10 requests | per minute per user |
| POST /tracking/start-session | 5 requests | per day per user |
| POST /tracking/sync-waypoints | 100 requests | per day per user |
| POST /tracking/stop-session | 5 requests | per day per user |
| GET /tracking/daily-history | 50 requests | per day per user |

Implement rate limiting in middleware to return `HTTP 429 Too Many Requests` with `Retry-After` header when limits exceeded.

