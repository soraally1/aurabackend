# Backend API Specification

**For**: In-App Navigation & Daily Movement Tracking  
**Created**: 2025-01-15  
**Status**: Ready for Implementation

---

## Overview

This document specifies **5 API endpoints** required for the frontend navigation and tracking features. All endpoints use **REST over HTTPS** with JSON payloads.

**Base URL**: `https://api.aura-health.app/api/v1` (or your backend domain)

**Authentication**: Include user token in `Authorization: Bearer {token}` header for all endpoints

---

## Endpoint 1: Calculate Route with Health Warnings

**Method**: `POST`  
**Path**: `/navigation/calculate-route`  
**Description**: Calculate route between two points with traffic and health warning data

### Request

```json
{
  "origin": {
    "latitude": -6.9932,
    "longitude": 110.4203
  },
  "destination": {
    "latitude": -7.0015,
    "longitude": 110.4128
  },
  "waypoints": [
    {
      "latitude": -6.9950,
      "longitude": 110.4150
    }
  ],
  "mode": "driving",
  "avoid_tolls": false,
  "avoid_highways": false
}
```

**Request Fields**:
- `origin` (required): Starting point as {latitude, longitude}
- `destination` (required): End point as {latitude, longitude}
- `waypoints` (optional): Array of intermediate stops
- `mode` (optional): "driving" or "walking", default: "driving"
- `avoid_tolls` (optional): Boolean, default: false
- `avoid_highways` (optional): Boolean, default: false

### Response (Success 200)

```json
{
  "success": true,
  "data": {
    "route_id": "route_xyz123_semarang",
    "distance_meters": 2450,
    "duration_seconds": 480,
    "polyline": "encoded_polyline_string_here",
    "coordinates": [
      [110.4203, -6.9932],
      [110.4210, -6.9940],
      [110.4215, -6.9945],
      [110.4128, -7.0015]
    ],
    "steps": [
      {
        "instruction": "Belok kanan ke Jl. Pemuda",
        "distance_meters": 350,
        "duration_seconds": 60,
        "maneuver": "turn-right",
        "start_location": {
          "latitude": -6.9932,
          "longitude": 110.4203
        },
        "end_location": {
          "latitude": -6.9940,
          "longitude": 110.4210
        }
      },
      {
        "instruction": "Lurus terus ke Jl. Gajah Mada",
        "distance_meters": 800,
        "duration_seconds": 200,
        "maneuver": "straight",
        "start_location": {
          "latitude": -6.9940,
          "longitude": 110.4210
        },
        "end_location": {
          "latitude": -6.9987,
          "longitude": 110.4300
        }
      }
    ],
    "traffic_segments": [
      {
        "segment_index": 0,
        "severity": "heavy",
        "delay_seconds": 120,
        "affected_coordinates": [
          [110.4203, -6.9932],
          [110.4210, -6.9940]
        ]
      }
    ],
    "health_warnings": [
      {
        "warning_id": "hw_001",
        "type": "heavy_traffic",
        "severity": "high",
        "title": "Area Macet Parah",
        "message": "Kemacetan tinggi di Jl. Pemuda. Potensi paparan polusi PM2.5: 145 µg/m³",
        "location": {
          "latitude": -6.9940,
          "longitude": 110.4210
        },
        "aqi": 156,
        "pm2_5": 145,
        "health_risks": [
          "Iritasi mata dan kulit",
          "Gangguan pernapasan ringan"
        ],
        "recommendations": [
          "Gunakan masker N95",
          "Tutup jendela kendaraan",
          "Aktifkan AC recirculation mode"
        ],
        "icon": "warning",
        "color": "#EF4444"
      },
      {
        "warning_id": "hw_002",
        "type": "high_pollution",
        "severity": "medium",
        "title": "Zona Polusi Tinggi",
        "message": "Area industri Kaligawe - AQI 178 (Tidak Sehat)",
        "location": {
          "latitude": -6.9587,
          "longitude": 110.4578
        },
        "aqi": 178,
        "pm2_5": 162,
        "health_risks": [
          "ISPA (Infeksi Saluran Pernapasan Akut)",
          "Eksaserbasi asma"
        ],
        "recommendations": [
          "Hindari aktivitas outdoor",
          "Gunakan air purifier di dalam kendaraan"
        ],
        "icon": "cloud",
        "color": "#9333EA"
      }
    ],
    "map_markers": [
      {
        "marker_id": "marker_001",
        "category": "heavy_traffic",
        "location": {
          "latitude": -6.9940,
          "longitude": 110.4210
        },
        "title": "Macet Parah - Jl. Pemuda",
        "icon": "warning",
        "color": "#EF4444"
      },
      {
        "marker_id": "marker_002",
        "category": "high_pollution",
        "location": {
          "latitude": -6.9587,
          "longitude": 110.4578
        },
        "title": "Polusi Tinggi - Kaligawe",
        "icon": "cloud",
        "color": "#9333EA"
      }
    ]
  }
}
```

**Response Fields**:
- `route_id`: Unique route identifier (for caching)
- `distance_meters`: Total route distance
- `duration_seconds`: Total travel time (normal traffic)
- `polyline`: Encoded polyline (Google format) - optional if using coordinates
- `coordinates`: Array of [lon, lat] pairs for drawing route polyline
- `steps`: Turn-by-turn navigation instructions
- `traffic_segments`: Areas with heavy traffic and delays
- `health_warnings`: Environmental hazards along route
- `map_markers`: Markers to display on map (different categories)

### Error Response (400/500)

```json
{
  "success": false,
  "error": "No route found between origin and destination",
  "error_code": "NO_ROUTE"
}
```

**Possible Error Codes**:
- `INVALID_COORDINATES` - Lat/lon out of bounds
- `NO_ROUTE` - Cannot find path between points
- `TIMEOUT` - Request took too long (use default mock data)
- `SERVICE_UNAVAILABLE` - Backend service down

---

## Endpoint 2: Start Tracking Session

**Method**: `POST`  
**Path**: `/tracking/start-session`  
**Description**: Create a new active tracking session

### Request

```json
{
  "user_id": "user_abc123",
  "session_name": "Pulang Kantor",
  "start_location": {
    "latitude": -6.9932,
    "longitude": 110.4203,
    "timestamp": "2025-01-15T08:00:00Z"
  }
}
```

**Request Fields**:
- `user_id` (required): User UUID from auth system
- `session_name` (optional): User-defined journey name
- `start_location` (required): {latitude, longitude, timestamp (ISO 8601)}

### Response (Success 201)

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

**Response Fields**:
- `session_id`: Unique session ID (use for sync/stop operations)
- `started_at`: Server timestamp when session created
- `status`: Always "active" for new sessions

### Error Response (400/500)

```json
{
  "success": false,
  "error": "Invalid user_id",
  "error_code": "INVALID_USER"
}
```

---

## Endpoint 3: Sync Waypoints During Tracking

**Method**: `POST`  
**Path**: `/tracking/sync-waypoints`  
**Description**: Periodically sync collected waypoints (hybrid smart sync)

### Request

```json
{
  "session_id": "session_user_abc123_1705310400",
  "user_id": "user_abc123",
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

**Request Fields**:
- `session_id` (required): Session ID from start-session
- `user_id` (required): User UUID
- `waypoints` (required): Array of GPS waypoints collected since last sync
  - `latitude`, `longitude` (required)
  - `timestamp` (required): ISO 8601 format
  - `accuracy_meters` (optional): GPS accuracy estimate
  - `speed_mps` (optional): Speed in meters/second
  - `heading` (optional): Compass bearing 0-360 degrees
- `is_final` (required): Boolean - true if this is final sync on stop

### Response (Success 200)

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

**Response Fields**:
- `synced_count`: Number of waypoints accepted in this sync
- `session_id`: Echoed session ID for confirmation
- `total_waypoints`: Total waypoints stored for this session so far
- `last_sync_at`: Server timestamp of sync

### Error Response (400/500)

```json
{
  "success": false,
  "error": "Session not found or expired",
  "error_code": "SESSION_NOT_FOUND"
}
```

---

## Endpoint 4: Stop Tracking Session

**Method**: `POST`  
**Path**: `/tracking/stop-session`  
**Description**: End tracking session and return final statistics and AI analysis

### Request

```json
{
  "session_id": "session_user_abc123_1705310400",
  "user_id": "user_abc123",
  "end_location": {
    "latitude": -7.0015,
    "longitude": 110.4128,
    "timestamp": "2025-01-15T09:30:00Z"
  }
}
```

**Request Fields**:
- `session_id` (required): Session ID from start-session
- `user_id` (required): User UUID
- `end_location` (required): {latitude, longitude, timestamp (ISO 8601)}

### Response (Success 200)

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
      "summary": "Anda terpapar polusi sedang-tinggi selama 85 menit terutama di area Kaligawe yang terkenal dengan emisi tinggi dari sektor industri.",
      "recommendations": [
        "Gunakan pelembab kulit setelah tiba",
        "Cuci wajah dengan pembersih lembut",
        "Minum air putih minimal 500ml",
        "Hindari area Kaligawe di perjalanan esok hari jika memungkinkan"
      ]
    }
  }
}
```

**Response Fields**:
- `stats`: Exposure statistics object
  - `total_outdoor_minutes`: Minutes spent outdoors during journey
  - `total_indoor_minutes`: Minutes spent indoors
  - `average_outdoor_aqi`: Average AQI during outdoor time
  - `average_outdoor_pm25`: Average PM2.5 concentration
  - `max_aqi`: Highest AQI reached during journey
  - `worst_area_name`: Name/description of highest pollution zone
  - `worst_area_aqi`: AQI value of worst zone
  - `waypoints_processed`: Number of waypoints analyzed
- `analysis`: AI-generated health analysis (via Groq API)
  - `impact_score`: 0-100 scale
  - `risk_level`: "Rendah", "Sedang", "Tinggi", "Berbahaya"
  - `summary`: Human-readable explanation
  - `recommendations`: Array of actionable health tips

### Error Response (400/500)

```json
{
  "success": false,
  "error": "Session already ended",
  "error_code": "SESSION_ALREADY_ENDED"
}
```

---

## Endpoint 5: Get Daily Journey History

**Method**: `GET`  
**Path**: `/tracking/daily-history`  
**Description**: Fetch all journeys and waypoints for a specific date (for replay)

### Request

**Query Parameters**:
- `user_id` (required): User UUID
- `date` (required): Date in YYYY-MM-DD format

**Example**:
```
GET /api/v1/tracking/daily-history?user_id=user_abc123&date=2025-01-15
```

### Response (Success 200)

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
    "combined_waypoints": [
      {
        "latitude": -6.9932,
        "longitude": 110.4203,
        "timestamp": "2025-01-15T08:00:00Z",
        "classification": "outdoor",
        "is_stay": false,
        "aqi": 125,
        "pm2_5": 92,
        "zone_name": "Simpang Lima"
      },
      {
        "latitude": -6.9940,
        "longitude": 110.4210,
        "timestamp": "2025-01-15T08:05:00Z",
        "classification": "outdoor",
        "is_stay": false,
        "aqi": 145,
        "pm2_5": 112,
        "zone_name": "Jl. Pemuda"
      }
    ],
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

**Response Fields**:
- `date`: The requested date (YYYY-MM-DD)
- `sessions`: Array of all tracking sessions for that day
- `combined_waypoints`: All waypoints from all sessions, chronologically sorted
- `daily_stats`: Aggregated statistics across all sessions
- `waypoints` (in each session): Individual waypoints with:
  - `classification`: "indoor" or "outdoor" (inferred from movement patterns)
  - `is_stay`: Boolean (true if user stayed in this location > 20 minutes)
  - `aqi`: AQI value at that location (from zones database)
  - `pm2_5`: PM2.5 concentration at that location
  - `zone_name`: Name of pollution zone or location

### Response (No Data 200)

```json
{
  "success": true,
  "data": {
    "date": "2025-01-14",
    "sessions": [],
    "combined_waypoints": [],
    "daily_stats": {
      "total_outdoor_minutes": 0,
      "total_indoor_minutes": 0,
      "average_outdoor_aqi": 0,
      "average_outdoor_pm25": 0,
      "max_aqi": 0,
      "worst_area_name": "-",
      "worst_area_aqi": 0,
      "waypoints_processed": 0
    }
  }
}
```

### Error Response (400)

```json
{
  "success": false,
  "error": "Invalid date format, use YYYY-MM-DD",
  "error_code": "INVALID_DATE_FORMAT"
}
```

---

## Data Processing Logic

### How Backend Should Process Waypoints

**Step 1: Classify Indoor vs Outdoor**
```
For each waypoint:
  - If distance from previous waypoint < 60m AND previous waypoint duration >= 20 minutes:
    - Mark as "indoor" (user stayed in place)
  - Else:
    - Mark as "outdoor" (user is moving)
```

**Step 2: Match with Pollution Zones**
```
For each waypoint:
  - Query Semarang zones database (lat/lon index)
  - Find closest zone within 1 km radius
  - Get current AQI, PM2.5 from that zone
  - Set zone_name, aqi, pm2_5 on waypoint
```

**Step 3: Calculate Exposure Statistics**
```
total_outdoor_minutes = SUM(duration of all waypoints where classification='outdoor')
total_indoor_minutes = SUM(duration of all waypoints where classification='indoor')
average_outdoor_aqi = AVERAGE(aqi of outdoor waypoints)
average_outdoor_pm25 = AVERAGE(pm2_5 of outdoor waypoints)
max_aqi = MAX(aqi across all waypoints)
worst_area = waypoint with maximum aqi
```

**Step 4: Generate AI Analysis (via Groq API)**
```
Input to Groq LLM:
- Daily exposure stats
- Worst areas visited
- Time spent in each area
- User's age, health conditions (from profile - optional)

Prompt: "Analyze this user's exposure to pollution and provide health recommendations in Indonesian"

Output: impact_score, risk_level, summary, recommendations
```

---

## Authentication & Headers

**All Requests Must Include**:
```
Authorization: Bearer {jwt_token}
Content-Type: application/json
User-Agent: AURA-Mobile/1.0
```

**Token Generation** (your auth system):
- Issued after user login
- Includes: user_id, exp (expiry), issued_at
- Backend validates token signature on each request

---

## Error Handling

**Standard Error Response Format**:
```json
{
  "success": false,
  "error": "Human-readable error message",
  "error_code": "MACHINE_READABLE_CODE",
  "timestamp": "2025-01-15T09:30:45Z"
}
```

**Common HTTP Status Codes**:
- `200 OK` - Request successful
- `201 Created` - Resource created (session started)
- `400 Bad Request` - Invalid input (validation error)
- `401 Unauthorized` - Missing/invalid auth token
- `404 Not Found` - Resource not found
- `500 Internal Server Error` - Backend error
- `503 Service Unavailable` - Temporary backend unavailable (frontend should show cached data)

**Frontend Retry Strategy**:
- On 5xx errors: Retry up to 3 times with exponential backoff (1s, 2s, 4s)
- On 4xx errors: Don't retry, show error to user
- On timeout: Treat as 503, use mock/cached data

---

## Rate Limiting

**Recommended Limits**:
- `calculate-route`: 10 requests/minute per user
- `start-session`: 5 requests/day per user
- `sync-waypoints`: 100 requests/day per user (auto-sync every 10-15 waypoints)
- `stop-session`: 5 requests/day per user
- `daily-history`: 50 requests/day per user

**Response Header on Rate Limit**:
```
HTTP/1.1 429 Too Many Requests
Retry-After: 60
X-RateLimit-Limit: 10
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1705310460
```

---

## Caching Strategy

**Frontend Caching**:
- Route calculation results: Cache for 5 minutes (same origin/dest)
- Daily history: Cache for 5 minutes (same date)
- Zone data: Cache for 1 hour

**Backend Caching**:
- Traffic data from external APIs: Cache for 15 minutes
- Zone AQI values: Cache for 30 minutes
- Route polylines: Cache for 1 hour

---

## Testing the API

**Test User Credentials**:
```
email: test@aura-health.app
password: TestPassword123!
user_id: user_test_12345
```

**Example Test Flow**:
1. Login → Get JWT token
2. Call `start-session` with token → Get session_id
3. Simulate GPS waypoints → Call `sync-waypoints` 3x
4. Call `stop-session` → Get stats and analysis
5. Call `daily-history` for today → Should see the session
6. Call `calculate-route` between two points → Get warnings and markers

---

## OpenAPI/Swagger (Optional)

Convert this to OpenAPI 3.0 format for auto-generating documentation and SDKs:

```yaml
openapi: 3.0.0
info:
  title: AURA Navigation & Tracking API
  version: 1.0.0
servers:
  - url: https://api.aura-health.app/api/v1

components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT

security:
  - bearerAuth: []

paths:
  /navigation/calculate-route:
    post:
      # ... endpoint details
  /tracking/start-session:
    post:
      # ... endpoint details
  # ... etc
```

---

## Notes for Backend Developer

1. **Geospatial Queries**: Use PostGIS (PostgreSQL) or MongoDB geospatial indexing for zone matching
2. **Real-time Traffic**: Cache TomTom/HERE traffic API responses (expensive API calls)
3. **AI Analysis**: Queue Groq API calls asynchronously (don't block HTTP response)
4. **Data Retention**: Implement automatic cleanup of waypoints older than 90 days
5. **Monitoring**: Log all API calls, track response times, alert on error rates > 5%
6. **Versioning**: Use `/api/v1` path prefix for future API versions (`/api/v2`, etc.)
