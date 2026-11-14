# API Endpoints Summary

## Quick Reference for 5 Implemented Endpoints

### 1. Calculate Route with Health Warnings
```
POST /api/v1/navigation/calculate-route
POST /api/navigation/calculate-route
POST /navigation/calculate-route

Public endpoint (no auth required)
Returns: Route with traffic data, health warnings, map markers
```

**Files**:
- `pkg/navigation/model.go` - Route models and mock data
- `pkg/handlers/navigation.go` - HTTP handler

**Mock Response Fields**:
- `route_id`: Unique identifier
- `distance_meters`: Total distance
- `duration_seconds`: Travel time
- `coordinates`: Polyline points
- `steps`: Turn-by-turn directions
- `traffic_segments`: Congestion areas
- `health_warnings`: Pollution hazards with recommendations
- `map_markers`: Visual markers for map

---

### 2. Start Tracking Session
```
POST /api/v1/tracking/start-session
POST /api/tracking/start-session
POST /tracking/start-session

Protected endpoint (requires Firebase token)
Returns: session_id, started_at, status
```

**Files**:
- `pkg/tracking/model.go` - Session models
- `pkg/tracking/store.go` - Firestore persistence
- `pkg/handlers/tracking.go` - HTTP handler (StartSession)

**Session Creation**:
- Generates unique session ID: `session_{user_id}_{unix_timestamp}`
- Stores in Firestore with status "active"
- User ID extracted from Firebase auth token

---

### 3. Sync Waypoints During Tracking
```
POST /api/v1/tracking/sync-waypoints
POST /api/tracking/sync-waypoints
POST /tracking/sync-waypoints

Protected endpoint (requires Firebase token)
Returns: synced_count, total_waypoints, last_sync_at
```

**Files**:
- `pkg/tracking/analyzer.go` - Waypoint processing
- `pkg/handlers/tracking.go` - HTTP handler (SyncWaypoints)

**Processing**:
1. Classify waypoints as "indoor" or "outdoor"
   - Indoor: distance < 60m AND stayed > 20 minutes
   - Outdoor: user is moving
2. Match with pollution zones within 1 km radius
3. Enrich with AQI, PM2.5, zone name
4. Append to session in Firestore

---

### 4. Stop Tracking Session & Get Health Analysis
```
POST /api/v1/tracking/stop-session
POST /api/tracking/stop-session
POST /tracking/stop-session

Protected endpoint (requires Firebase token)
Returns: stats, analysis with health recommendations
```

**Files**:
- `pkg/tracking/analyzer.go` - Stats calculation and health analysis
- `pkg/handlers/tracking.go` - HTTP handler (StopSession)

**Output**:
```
stats: {
  total_outdoor_minutes,
  total_indoor_minutes,
  average_outdoor_aqi,
  average_outdoor_pm25,
  max_aqi,
  worst_area_name,
  waypoints_processed,
  distance_km,
  duration_minutes
}

analysis: {
  impact_score (0-100),
  risk_level ("Rendah", "Sedang", "Tinggi"),
  summary (2-3 sentences),
  recommendations (array of 3-4 health tips)
}
```

---

### 5. Daily Journey Replay History
```
GET /api/v1/tracking/daily-history?date=YYYY-MM-DD
GET /api/tracking/daily-history
GET /tracking/daily-history

Protected endpoint (requires Firebase token)
Returns: All sessions and waypoints for specified date
```

**Files**:
- `pkg/handlers/tracking.go` - HTTP handler (DailyHistory)

**Query Parameters**:
- `date` (optional): YYYY-MM-DD format, defaults to today
- `user_id` (optional): Extracted from auth token

**Response**:
```
{
  date: "YYYY-MM-DD",
  sessions: [
    {
      session_id,
      session_name,
      started_at,
      ended_at,
      duration_minutes,
      distance_km,
      waypoints: [...]
    }
  ],
  combined_waypoints: [...],  // All waypoints from all sessions
  daily_stats: {...}          // Aggregated across all sessions
}
```

---

## Configuration

Add to `.env`:
```bash
TRACKING_COLLECTION=tracking_sessions
ZONES_COLLECTION=environmental_zones
GROQ_API_KEY=your_groq_api_key (optional)
```

## Authentication

Protected endpoints require:
```
Authorization: Bearer {firebase_id_token}
```

Token is verified via Firebase Admin SDK and user ID is extracted from the token.

---

## Key Files Implemented

| File | Purpose |
|------|---------|
| `pkg/tracking/model.go` | Session, Waypoint, Stats, HealthAnalysis data structures |
| `pkg/tracking/store.go` | Firestore CRUD operations for sessions |
| `pkg/tracking/analyzer.go` | Waypoint classification, zone matching, health analysis |
| `pkg/tracking/analyzer_test.go` | Unit tests (Haversine distance, classification, stats, analysis) |
| `pkg/navigation/model.go` | Route, Step, TrafficSegment, HealthWarning, MapMarker models |
| `pkg/handlers/tracking.go` | HTTP handlers: StartSession, SyncWaypoints, StopSession, DailyHistory |
| `pkg/handlers/navigation.go` | HTTP handler: CalculateRoute |
| `pkg/config/config.go` | Updated Config struct with TrackingCollection |

---

## How Endpoints Work Together

### Typical User Journey:

1. **Frontend**: Calls `POST /navigation/calculate-route` to show route options with health warnings
2. **Frontend**: User starts trip, calls `POST /tracking/start-session`
3. **Frontend**: During trip, periodically calls `POST /tracking/sync-waypoints` with GPS waypoints (every 10-15 points or 5 minutes)
4. **Frontend**: After trip, calls `POST /tracking/stop-session` to get stats and health recommendations
5. **Frontend**: User views history with `GET /tracking/daily-history` to replay their movements and see exposure trends

---

## Response Format

All endpoints return JSON with standard envelope:

### Success (2xx)
```json
{
  "success": true,
  "data": {
    // endpoint-specific data
  }
}
```

### Error (4xx/5xx)
```json
{
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable message"
  }
}
```

---

## Error Codes

| Code | Status | Meaning |
|------|--------|---------|
| `INVALID_INPUT` | 400 | Missing or malformed request data |
| `INVALID_COORDINATES` | 400 | Out-of-bounds lat/lon |
| `INVALID_DATE_FORMAT` | 400 | Date not in YYYY-MM-DD format |
| `UNAUTHORIZED` | 401 | Missing/invalid auth token |
| `FORBIDDEN` | 403 | Session belongs to different user |
| `SESSION_NOT_FOUND` | 404 | Session doesn't exist or expired |
| `SESSION_ALREADY_ENDED` | 400 | Cannot perform operation on completed session |
| `DATABASE_ERROR` | 500 | Firestore operation failed |
| `METHOD_NOT_ALLOWED` | 405 | Wrong HTTP method |

---

## Testing

```bash
# Build
go build ./cmd/server

# Test tracking package
go test -v ./pkg/tracking/

# Test all packages
go test -v ./...
```

---

## Deployment Checklist

- [ ] Firestore indexes created for `tracking_sessions` collection
- [ ] Firestore security rules restrict users to their own sessions
- [ ] CORS headers configured for frontend domain
- [ ] Environment variables set (TRACKING_COLLECTION, ZONES_COLLECTION, etc.)
- [ ] Rate limiting middleware implemented (optional but recommended)
- [ ] Monitoring/logging configured for API calls
- [ ] Tests pass with `go test -v ./...`
- [ ] Build succeeds with `go build ./cmd/server`

