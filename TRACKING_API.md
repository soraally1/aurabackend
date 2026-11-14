# Tracking API Implementation Guide

This document describes the implementation of 5 API endpoints for GPS tracking, route calculation, and health analysis.

## Implemented Endpoints

### 1. POST /tracking/start-session
**Status**: ✅ Implemented

Starts a new GPS tracking session. Creates a session record in Firestore that stores waypoints collected during a journey.

**Authentication**: Required (protected endpoint)

**Request**:
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

**Response** (201 Created):
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

**Implementation Details**:
- Generates unique session ID based on user ID and timestamp
- Validates start location coordinates
- Stores session in Firestore collection: `tracking_sessions`
- User ID is extracted from Firebase auth token (cannot be overridden)

**Handler**: `pkg/handlers/tracking.go` - `StartSession()`

---

### 2. POST /tracking/sync-waypoints
**Status**: ✅ Implemented

Periodically syncs collected GPS waypoints to the server during an active tracking session.

**Authentication**: Required (protected endpoint)

**Request**:
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

**Response** (200 OK):
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

**Implementation Details**:
- Validates session exists and belongs to authenticated user
- Verifies session is still active (not already ended)
- Enhances waypoints with zone data (AQI, PM2.5 values from nearby zones)
- Classifies waypoints as "indoor" or "outdoor" using heuristic:
  - If distance < 60m AND time gap > 20 minutes → "indoor"
  - Otherwise → "outdoor"
- Appends waypoints to session in Firestore
- Returns total waypoint count for this session

**Smart Sync Features**:
- Mobile app can call this endpoint every 10-15 waypoints
- `is_final` flag indicates whether this is the final sync before session ends
- Hybrid sync reduces bandwidth while maintaining data accuracy

**Handler**: `pkg/handlers/tracking.go` - `SyncWaypoints()`

---

### 3. POST /tracking/stop-session
**Status**: ✅ Implemented

Ends an active tracking session and returns aggregated exposure statistics with AI health analysis.

**Authentication**: Required (protected endpoint)

**Request**:
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

**Response** (200 OK):
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
      "summary": "Anda terpapar polusi sedang-tinggi selama 85 menit...",
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

**Implementation Details**:
- Validates session exists and belongs to user
- Verifies session is still active
- Calculates aggregated exposure statistics:
  - Separates indoor vs outdoor waypoints
  - Computes average AQI and PM2.5 for outdoor periods
  - Tracks worst pollution zone encountered
  - Calculates total duration and distance
- Calls Groq API (Llama-3.3-70b-versatile) for AI health analysis:
  - Provides system prompt with medical context (lung specialist)
  - Sends daily stats and worst areas
  - Receives structured JSON with impact score, risk level, and recommendations
- Falls back to rule-based clinical diagnosis if Groq API unavailable
- Updates session status to "completed" in Firestore
- Stores stats and analysis for later retrieval

**Health Analysis Features**:
- **Impact Score**: 0-100 scale calculated from exposure duration and AQI levels
- **Risk Level**: "Rendah" (Low), "Sedang" (Medium), "Tinggi" (High), or "Berbahaya" (Dangerous)
- **Doctor Diagnosis**: Medical explanation of respiratory impact
- **Recommendations**: 3-4 actionable health tips based on exposure data

**Handler**: `pkg/handlers/tracking.go` - `StopSession()`

---

### 4. GET /tracking/daily-history
**Status**: ✅ Implemented

Retrieves all tracking sessions and waypoints for a specific date, enabling replay functionality.

**Authentication**: Required (protected endpoint)

**Request**:
```
GET /tracking/daily-history?date=2025-01-15
```

**Response** (200 OK):
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

**Implementation Details**:
- Validates date format (YYYY-MM-DD)
- Defaults to today if no date provided
- Retrieves all sessions for authenticated user on specified date
- Aggregates waypoints across all sessions
- Calculates daily statistics (totals and averages)
- Returns sessions separately from combined waypoints for flexibility
- Allows frontend to:
  - Show timeline of sessions with durations and distances
  - Replay entire day's journey chronologically
  - View aggregated daily exposure metrics

**Query Parameters**:
- `date` (optional): YYYY-MM-DD format, defaults to today

**Handler**: `pkg/handlers/tracking.go` - `DailyHistory()`

---

### 5. POST /navigation/calculate-route
**Status**: ✅ Implemented

Calculates optimal routes with turn-by-turn instructions, traffic data, and health warnings based on pollution zones.

**Authentication**: Public (no authentication required)

**Request**:
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
  "waypoints": [],
  "mode": "driving",
  "avoid_tolls": false,
  "avoid_highways": false
}
```

**Response** (200 OK):
```json
{
  "success": true,
  "data": {
    "route_id": "route_20250115080045",
    "distance_meters": 2450,
    "duration_seconds": 480,
    "polyline": "...",
    "coordinates": [
      [110.4203, -6.9932],
      [110.4210, -6.9940],
      [110.4215, -6.9945]
    ],
    "steps": [
      {
        "instruction": "Belok kanan ke Jl. Pemuda",
        "distance_meters": 350,
        "duration_seconds": 60,
        "maneuver": "turn-right",
        "start_location": { "latitude": -6.9932, "longitude": 110.4203 },
        "end_location": { "latitude": -6.9940, "longitude": 110.4210 }
      }
    ],
    "traffic_segments": [
      {
        "segment_index": 0,
        "severity": "heavy",
        "delay_seconds": 120,
        "affected_coordinates": [[110.4203, -6.9932], [110.4210, -6.9940]]
      }
    ],
    "health_warnings": [
      {
        "warning_id": "hw_001",
        "type": "heavy_traffic",
        "severity": "high",
        "title": "Area Macet Parah",
        "message": "Kemacetan tinggi di Jl. Pemuda. Potensi paparan polusi PM2.5: 145 µg/m³",
        "location": { "latitude": -6.9940, "longitude": 110.4210 },
        "aqi": 156,
        "pm2_5": 145,
        "health_risks": ["Iritasi mata dan kulit", "Gangguan pernapasan ringan"],
        "recommendations": ["Gunakan masker N95", "Tutup jendela kendaraan"],
        "icon": "warning",
        "color": "#EF4444"
      }
    ],
    "map_markers": [
      {
        "marker_id": "marker_001",
        "category": "heavy_traffic",
        "location": { "latitude": -6.9940, "longitude": 110.4210 },
        "title": "Macet Parah - Jl. Pemuda",
        "icon": "warning",
        "color": "#EF4444"
      }
    ]
  }
}
```

**Implementation Details**:
- Validates coordinate ranges (Indonesia bounds)
- Defaults to "driving" mode if not specified
- Validates mode is either "driving" or "walking"
- Generates mock route with realistic Semarang data
- Returns comprehensive route information:
  - Turn-by-turn navigation steps with maneuvers
  - Traffic segments indicating congestion and delays
  - Health warnings for high-pollution or high-traffic areas
  - Map markers for UI display
- Current implementation returns mock data; production would integrate with:
  - Google Maps API, OpenRouteService, or OSRM for routing
  - Real-time traffic API (TomTom, HERE)
  - Zones database for health warnings

**Route Parameters**:
- `origin` (required): Starting point {latitude, longitude}
- `destination` (required): End point {latitude, longitude}
- `waypoints` (optional): Intermediate stops
- `mode` (optional): "driving" or "walking", default: "driving"
- `avoid_tolls` (optional): Boolean, default: false
- `avoid_highways` (optional): Boolean, default: false

**Handler**: `pkg/handlers/navigation.go` - `CalculateRoute()`

---

## Architecture & Data Flow

### Session Lifecycle

```
START SESSION (POST /tracking/start-session)
    ↓
SYNC WAYPOINTS (POST /tracking/sync-waypoints) - multiple times
    ├─ Mobile app collects GPS points
    ├─ Sends batch every 10-15 waypoints
    └─ Server enhances with zone data (AQI, PM2.5)
    ↓
STOP SESSION (POST /tracking/stop-session)
    ├─ Calculates aggregated stats
    ├─ Calls Groq API for health analysis
    └─ Returns stats + AI recommendations
    ↓
RETRIEVE HISTORY (GET /tracking/daily-history)
    └─ Shows all journeys + combined waypoints for replay
```

### Data Enhancement Pipeline

```
Raw GPS Waypoint
    ↓
Zone Lookup (geospatial query)
    ├─ Find nearest zone within 1 km radius
    └─ Get current AQI, PM2.5 values
    ↓
Classification (indoor/outdoor)
    ├─ Heuristic: distance < 60m + time gap > 20 min = indoor
    └─ Otherwise = outdoor
    ↓
Enriched Waypoint
    ├─ Classification: "indoor" | "outdoor"
    ├─ Zone name, AQI, PM2.5
    └─ Stored in Firestore
```

### Health Analysis Pipeline

```
Session Completed (Stop)
    ↓
Calculate Stats
    ├─ Separate outdoor/indoor durations
    ├─ Average AQI/PM2.5 for outdoor
    ├─ Track worst area
    └─ Calculate impact score
    ↓
Call Groq API (Llama-3.3-70b-versatile)
    ├─ System prompt: lung specialist context
    ├─ Input: daily stats + worst areas
    ├─ Timeout: 15 seconds
    └─ Output: JSON with diagnosis + recommendations
    ↓
Fallback Rule-Based Diagnosis (if API fails)
    ├─ Time factor: up to 40 points for 120 mins outdoor
    ├─ AQI factor: up to 50 points for AQI 200
    ├─ Peak hotspot factor: up to 10 points
    └─ Generates clinically accurate diagnosis
    ↓
Response to Frontend
    ├─ Impact score (0-100)
    ├─ Risk level (Rendah/Sedang/Tinggi/Berbahaya)
    ├─ Medical summary
    └─ 3-4 actionable recommendations
```

---

## Firestore Collections

### tracking_sessions
```json
{
  "id": "session_user_abc123_1705310400",
  "user_id": "user_abc123",
  "session_name": "Pulang Kantor",
  "status": "active|completed",
  "started_at": "2025-01-15T08:00:00Z",
  "ended_at": "2025-01-15T09:30:00Z (only if completed)",
  "start_location": { "latitude": -6.9932, "longitude": 110.4203, "timestamp": "..." },
  "end_location": { "latitude": -7.0015, "longitude": 110.4128, "timestamp": "..." },
  "waypoints": [
    { "latitude": -6.9932, "longitude": 110.4203, "timestamp": "...", ... }
  ],
  "stats": { "total_outdoor_minutes": 85, ... },
  "analysis": { "impact_score": 72, "risk_level": "Sedang", ... },
  "created_at": "2025-01-15T08:00:00Z",
  "updated_at": "2025-01-15T09:30:00Z"
}
```

---

## Error Handling

### Common Errors

**Invalid Coordinates**
```json
{
  "success": false,
  "error": {
    "code": "INVALID_COORDINATES",
    "message": "origin coordinates out of valid range"
  }
}
```
Status: 400

**Session Not Found**
```json
{
  "success": false,
  "error": {
    "code": "SESSION_NOT_FOUND",
    "message": "Session not found or expired"
  }
}
```
Status: 404

**Access Denied**
```json
{
  "success": false,
  "error": {
    "code": "ACCESS_DENIED",
    "message": "This session does not belong to you"
  }
}
```
Status: 403

**Unauthorized**
```json
{
  "success": false,
  "error": {
    "code": "UNAUTHORIZED",
    "message": "User ID not found in token"
  }
}
```
Status: 401

---

## Testing

Run tests:
```bash
go test ./pkg/handlers -v -run "TestCalculate"
```

Test coverage includes:
- Distance calculation using Haversine formula
- Session stats aggregation
- Waypoint classification logic
- Input validation for all endpoints

---

## Environment Variables

Required:
- `FIREBASE_SERVICE_ACCOUNT_KEY` - Firebase service account JSON
- `FIREBASE_WEB_API_KEY` - Firebase Web API key
- `GROQ_API_KEY` or `GROQ_AURA2` - Groq API key for AI analysis

Defaults:
- `TRACKING_COLLECTION` - "tracking_sessions"
- `ZONES_COLLECTION` - "environmental_zones"
- `MOBILITY_COLLECTION` - "daily_mobility"

---

## Frontend Integration

### Example Usage (JavaScript/TypeScript)

```typescript
// 1. Start tracking session
const startResponse = await fetch('/api/v1/tracking/start-session', {
  method: 'POST',
  headers: { 'Authorization': `Bearer ${token}` },
  body: JSON.stringify({
    session_name: 'Commute',
    start_location: { latitude, longitude, timestamp: new Date().toISOString() }
  })
});
const { data: { session_id } } = await startResponse.json();

// 2. Collect GPS points and sync periodically
const syncWaypoints = async (waypoints) => {
  await fetch('/api/v1/tracking/sync-waypoints', {
    method: 'POST',
    headers: { 'Authorization': `Bearer ${token}` },
    body: JSON.stringify({
      session_id,
      waypoints,
      is_final: false
    })
  });
};

// 3. End session with stats
const stopResponse = await fetch('/api/v1/tracking/stop-session', {
  method: 'POST',
  headers: { 'Authorization': `Bearer ${token}` },
  body: JSON.stringify({
    session_id,
    end_location: { latitude, longitude, timestamp: new Date().toISOString() }
  })
});
const { data: { stats, analysis } } = await stopResponse.json();

// 4. Show history for replay
const historyResponse = await fetch(`/api/v1/tracking/daily-history?date=2025-01-15`, {
  headers: { 'Authorization': `Bearer ${token}` }
});
const { data: { sessions, combined_waypoints, daily_stats } } = await historyResponse.json();
```

---

## Performance Considerations

- **Waypoint Sync**: Send waypoints in batches of 10-15 to reduce bandwidth
- **Zone Lookup**: Geospatial queries cached locally; refetch every 30 seconds max
- **Groq API**: Timeout set to 15 seconds; falls back to rule-based diagnosis
- **Firestore Indexing**: Composite index on (user_id, started_at) for daily history queries
- **Data Retention**: Consider archiving sessions older than 90 days

---

## Future Enhancements

1. **Real Routing Integration**: Replace mock routes with Google Maps or OpenRouteService
2. **Real-time Traffic**: Integrate TomTom or HERE traffic APIs
3. **Route Optimization**: Alternative routes to avoid high-pollution zones
4. **Predictive Health Alerts**: Warn users before entering high-pollution areas
5. **Social Features**: Share safe routes with other users
6. **Machine Learning**: Detect travel patterns and anomalies

