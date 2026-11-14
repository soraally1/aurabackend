# API Implementation Guide - 5 Endpoints Completed

## Quick Reference

All 5 endpoints from `backend-api.md` have been fully implemented and tested.

## Endpoint Summary

### 1. Navigate Route with Health Warnings
- **Endpoint**: `POST /api/v1/navigation/calculate-route`
- **Auth**: Public (no token required)
- **File**: `pkg/handlers/navigation.go`
- **Response**: Route with steps, traffic, health warnings, map markers

### 2. Start Tracking Session
- **Endpoint**: `POST /api/v1/tracking/start-session`
- **Auth**: Required (Firebase token)
- **File**: `pkg/handlers/tracking.go`
- **Response**: Session ID, started_at, status

### 3. Sync GPS Waypoints
- **Endpoint**: `POST /api/v1/tracking/sync-waypoints`
- **Auth**: Required (Firebase token)
- **File**: `pkg/handlers/tracking.go`
- **Processing**: Indoor/outdoor classification + zone enrichment
- **Response**: Synced count, total waypoints, last sync time

### 4. Stop Session with AI Analysis
- **Endpoint**: `POST /api/v1/tracking/stop-session`
- **Auth**: Required (Firebase token)
- **File**: `pkg/handlers/tracking.go`
- **AI**: Groq LLM with fallback to rule-based analysis
- **Response**: Stats + health analysis with recommendations

### 5. Get Daily Journey History
- **Endpoint**: `GET /api/v1/tracking/daily-history?date=YYYY-MM-DD`
- **Auth**: Required (Firebase token)
- **File**: `pkg/handlers/tracking.go`
- **Response**: All sessions + combined waypoints + daily stats

## Implementation Details

### New Files Created
- `pkg/tracking/groq.go` - Groq AI client for health analysis

### Modified Files
- `pkg/tracking/analyzer.go` - Added Groq support methods
- `pkg/handlers/tracking.go` - Enhanced StopSession with AI analysis

### Existing Infrastructure Used
- `pkg/handlers/navigation.go` - Already implemented
- `pkg/handlers/tracking.go` - Extended with Groq integration
- `pkg/app/router.go` - Routes already registered
- `pkg/tracking/store.go` - Firestore persistence
- `pkg/zones/` - Zone matching and enrichment
- `pkg/middleware/` - Auth and CORS

## Testing

All tests pass:
```bash
go test ./... -v

Results:
✅ pkg/tracking (5 tests)
✅ pkg/handlers (8 tests)
✅ pkg/zones (1 test)
✅ pkg/hospitals (3 tests)
✅ pkg/mobility (4 tests)
✅ pkg/analytics (1 test)
✅ pkg/firebase (3 tests)
✅ pkg/geoutils (3 tests)
```

## Building

```bash
cd /Users/daffa/JOINTS/aurabackend
go build ./...
```

## Feature Highlights

### Waypoint Processing
- Automatic indoor/outdoor classification
- Distance threshold: 60 meters
- Duration threshold: 20 minutes for "stay"
- Zone matching within 1 km radius
- AQI/PM2.5 data attachment

### Health Analysis
- **With Groq API**:
  - Calls llama-3.3-70b-versatile model
  - Returns impact score (0-100), risk level, recommendations
  - Falls back to qwen and gpt-oss models if primary fails
  
- **Without Groq API**:
  - Uses high-fidelity rule-based analysis
  - Still returns same response format
  - No external dependencies

### Statistics Calculation
- Total journey time and distance (haversine)
- Indoor vs outdoor time split
- Average outdoor AQI/PM2.5
- Peak AQI and worst area identification
- Waypoint count processed

## Configuration

Required environment variables:
- `FIREBASE_SERVICE_ACCOUNT_KEY` - Firebase credentials
- `FIREBASE_WEB_API_KEY` - Firebase Web API key
- `GROQ_API_KEY` or `GROQ_AURA2` - Groq API key (optional, falls back to rule-based)

Collection names (with defaults):
- `TRACKING_COLLECTION` - "tracking_sessions"
- `ZONES_COLLECTION` - "environmental_zones"

## Error Handling

All endpoints return consistent error format:
```json
{
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable message"
  }
}
```

Common error codes:
- `INVALID_INPUT` - Request validation failed
- `UNAUTHORIZED` - Missing/invalid auth token
- `SESSION_NOT_FOUND` - Session doesn't exist
- `SESSION_ALREADY_ENDED` - Can't modify ended session
- `DATABASE_ERROR` - Firestore operation failed

## API Validation

✅ Request validation for all fields
✅ Coordinate range checking (Indonesia bounds)
✅ Date format validation (YYYY-MM-DD)
✅ Session ownership checks
✅ Active status verification

## Response Format

All responses use consistent envelope:

**Success (200/201)**:
```json
{
  "success": true,
  "data": { /* endpoint-specific data */ }
}
```

**Error (4xx/5xx)**:
```json
{
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Details"
  }
}
```

## Integration Checklist

- [x] Endpoints implemented
- [x] Routes registered
- [x] Auth middleware integrated
- [x] Firestore persistence working
- [x] Zone enrichment working
- [x] Groq AI integration with fallback
- [x] Tests passing
- [x] Error handling
- [x] Documentation

## Testing the Endpoints

### Example Flow

```bash
# 1. Start session
curl -X POST http://localhost:3000/api/v1/tracking/start-session \
  -H "Authorization: Bearer {token}" \
  -H "Content-Type: application/json" \
  -d '{
    "session_name": "Morning commute",
    "start_location": {
      "latitude": -6.9932,
      "longitude": 110.4203,
      "timestamp": "2025-01-15T08:00:00Z"
    }
  }'

# 2. Sync waypoints (multiple times)
curl -X POST http://localhost:3000/api/v1/tracking/sync-waypoints \
  -H "Authorization: Bearer {token}" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "session_user_abc123_1705310400",
    "waypoints": [
      {
        "latitude": -6.9940,
        "longitude": 110.4210,
        "timestamp": "2025-01-15T08:05:00Z",
        "accuracy_meters": 12,
        "speed_mps": 5.2,
        "heading": 145
      }
    ],
    "is_final": false
  }'

# 3. Stop session
curl -X POST http://localhost:3000/api/v1/tracking/stop-session \
  -H "Authorization: Bearer {token}" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "session_user_abc123_1705310400",
    "end_location": {
      "latitude": -7.0015,
      "longitude": 110.4128,
      "timestamp": "2025-01-15T09:30:00Z"
    }
  }'

# 4. Get daily history
curl -X GET "http://localhost:3000/api/v1/tracking/daily-history?date=2025-01-15" \
  -H "Authorization: Bearer {token}"

# 5. Calculate route
curl -X POST http://localhost:3000/api/v1/navigation/calculate-route \
  -H "Content-Type: application/json" \
  -d '{
    "origin": {"latitude": -6.9932, "longitude": 110.4203},
    "destination": {"latitude": -7.0015, "longitude": 110.4128},
    "mode": "driving"
  }'
```

## Performance Considerations

- Waypoint processing: O(n) where n = waypoint count
- Zone matching: Uses geospatial indexing in Firestore
- Groq API calls: ~2-5 seconds typically, with 15s timeout
- Session storage: Firestore collections indexed by user_id and date

## Monitoring

Recommended metrics to track:
- API response times (p50, p95, p99)
- Groq API success rate and latency
- Session creation/completion rates
- Waypoint sync frequency
- Average waypoint counts per session
- Error rates by endpoint

## Documentation References

- Full spec: `backend-api.md`
- Implementation details: See artifact "5 API Endpoints Implementation Complete"
- Test examples: Run `go test -v` in pkg/tracking and pkg/handlers
