# API Quick Reference

## All 5 Endpoints

### 1. POST /navigation/calculate-route
Calculate routes with turn-by-turn instructions and health warnings.

```bash
curl -X POST http://localhost:8080/api/v1/navigation/calculate-route \
  -H "Content-Type: application/json" \
  -d '{
    "origin": {"latitude": -6.9932, "longitude": 110.4203},
    "destination": {"latitude": -7.0015, "longitude": 110.4128},
    "mode": "driving"
  }'
```

**Response**: Route with steps, traffic segments, health warnings, and map markers

---

### 2. POST /tracking/start-session
Start a new GPS tracking session.

```bash
TOKEN="your_firebase_id_token"
curl -X POST http://localhost:8080/api/v1/tracking/start-session \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "session_name": "Commute to Office",
    "start_location": {
      "latitude": -6.9932,
      "longitude": 110.4203,
      "timestamp": "2025-01-15T08:00:00Z"
    }
  }'
```

**Response**: `{"session_id": "session_...", "started_at": "...", "status": "active"}`

---

### 3. POST /tracking/sync-waypoints
Sync GPS waypoints during an active session.

```bash
TOKEN="your_firebase_id_token"
SESSION_ID="session_user_abc123_1705310400"

curl -X POST http://localhost:8080/api/v1/tracking/sync-waypoints \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "'$SESSION_ID'",
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
  }'
```

**Response**: `{"synced_count": 2, "total_waypoints": 25, "last_sync_at": "..."}`

**Call this endpoint**:
- Every 10-15 waypoints collected
- Before final waypoint, set `is_final: true`
- Reduces bandwidth while maintaining accuracy

---

### 4. POST /tracking/stop-session
End tracking session and get health analysis.

```bash
TOKEN="your_firebase_id_token"
SESSION_ID="session_user_abc123_1705310400"

curl -X POST http://localhost:8080/api/v1/tracking/stop-session \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "'$SESSION_ID'",
    "end_location": {
      "latitude": -7.0015,
      "longitude": 110.4128,
      "timestamp": "2025-01-15T09:30:00Z"
    }
  }'
```

**Response**: 
```json
{
  "session_id": "...",
  "started_at": "...",
  "ended_at": "...",
  "duration_minutes": 90,
  "distance_km": 12.5,
  "stats": {
    "total_outdoor_minutes": 85,
    "average_outdoor_aqi": 132,
    "average_outdoor_pm25": 98,
    "max_aqi": 178,
    "worst_area_name": "Kaligawe Industrial Zone"
  },
  "analysis": {
    "impact_score": 72,
    "risk_level": "Sedang",
    "summary": "Anda terpapar polusi sedang-tinggi...",
    "recommendations": ["Gunakan masker...", "Cuci wajah...", ...]
  }
}
```

---

### 5. GET /tracking/daily-history
Get all sessions and waypoints for a date (replay).

```bash
TOKEN="your_firebase_id_token"
curl http://localhost:8080/api/v1/tracking/daily-history?date=2025-01-15 \
  -H "Authorization: Bearer $TOKEN"
```

**Response**: 
```json
{
  "date": "2025-01-15",
  "sessions": [
    {
      "session_id": "...",
      "session_name": "...",
      "started_at": "...",
      "ended_at": "...",
      "waypoints": [...]
    }
  ],
  "combined_waypoints": [...],
  "daily_stats": {
    "total_outdoor_minutes": 180,
    "average_outdoor_aqi": 138,
    "max_aqi": 178,
    "waypoints_processed": 120
  }
}
```

---

## Common Error Codes

| Code | HTTP Status | Meaning |
|------|------------|---------|
| `INVALID_COORDINATES` | 400 | Coordinates out of valid range |
| `INVALID_INPUT` | 400 | Missing or malformed request field |
| `INVALID_DATE_FORMAT` | 400 | Date not in YYYY-MM-DD format |
| `METHOD_NOT_ALLOWED` | 405 | Wrong HTTP method (GET vs POST) |
| `UNAUTHORIZED` | 401 | Missing/invalid Bearer token |
| `ACCESS_DENIED` | 403 | Session doesn't belong to authenticated user |
| `SESSION_NOT_FOUND` | 404 | Session ID doesn't exist |
| `SESSION_NOT_ACTIVE` | 400 | Session already ended |
| `DATABASE_ERROR` | 500 | Firestore operation failed |

---

## Data Models

### Session
```go
{
  "id": "session_user_abc123_1705310400",
  "user_id": "user_abc123",
  "session_name": "Commute",
  "status": "active|completed",
  "started_at": "2025-01-15T08:00:00Z",
  "ended_at": null,
  "start_location": {"latitude": -6.9932, "longitude": 110.4203, "timestamp": "..."},
  "end_location": null,
  "waypoints": [...],
  "stats": null,
  "analysis": null,
  "created_at": "2025-01-15T08:00:00Z",
  "updated_at": "2025-01-15T08:00:00Z"
}
```

### Waypoint
```go
{
  "latitude": -6.9932,
  "longitude": 110.4203,
  "timestamp": "2025-01-15T08:00:00Z",
  "accuracy_meters": 12,
  "speed_mps": 5.2,
  "heading": 145,
  "classification": "outdoor|indoor",
  "is_stay": false,
  "zone_name": "Simpang Lima",
  "aqi": 125,
  "pm2_5": 92
}
```

### SessionStats
```go
{
  "total_outdoor_minutes": 85,
  "total_indoor_minutes": 5,
  "average_outdoor_aqi": 132,
  "average_outdoor_pm25": 98,
  "max_aqi": 178,
  "worst_area_name": "Kaligawe Industrial Zone",
  "worst_area_aqi": 178,
  "waypoints_processed": 52,
  "distance_km": 12.5,
  "duration_minutes": 90
}
```

### HealthAnalysis
```go
{
  "impact_score": 72.5,
  "risk_level": "Sedang",
  "summary": "Paparan polusi Anda hari ini berada pada tingkat sedang...",
  "recommendations": [
    "Gunakan masker N95...",
    "Cuci wajah dengan pembersih lembut...",
    "Konsumsi buah bervitamin C...",
    "Nyalakan ventilasi mekanis..."
  ]
}
```

---

## Implementation Details

### Files Modified
- `pkg/handlers/tracking.go` - 5 handler functions
- `pkg/handlers/tracking_test.go` - Unit tests
- `pkg/zones/store.go` - Added `FindNearestZone()` method

### Supported Routes
- `/api/v1/...` - Primary API path
- `/api/...` - Alternative path
- `/...` - Direct path (for Vercel compatibility)

### Authentication
- **Public endpoints**: `/navigation/calculate-route`
- **Protected endpoints**: All `/tracking/*` routes
  - Require: `Authorization: Bearer <firebase_id_token>`
  - User ID extracted from token (cannot be spoofed)

### Firestore Collections
- `tracking_sessions` - Stores all session data
  - Index recommended: (user_id, started_at)
  - TTL policy: Delete after 90 days

---

## Testing

Run all tests:
```bash
go test ./pkg/handlers -v -run "TestCalculate"
```

Build verification:
```bash
go build ./cmd/server
```

---

## Configuration

Environment variables (in `.env` file):

```bash
# Required
FIREBASE_SERVICE_ACCOUNT_KEY=<json_or_base64>
FIREBASE_WEB_API_KEY=<api_key>

# Optional (with defaults)
TRACKING_COLLECTION=tracking_sessions
ZONES_COLLECTION=environmental_zones
GROQ_API_KEY=<api_key>  # For AI health analysis
```

---

## Frontend Integration (Example)

### React/TypeScript
```typescript
// 1. Start tracking
const response = await fetch('/api/v1/tracking/start-session', {
  method: 'POST',
  headers: {
    'Authorization': `Bearer ${token}`,
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({
    session_name: 'Morning Commute',
    start_location: {
      latitude: currentLocation.lat,
      longitude: currentLocation.lng,
      timestamp: new Date().toISOString()
    }
  })
});

const { data: { session_id } } = await response.json();

// 2. Collect GPS points (e.g., every 10-15 points)
const waypoints = [/* collected GPS points */];
await fetch('/api/v1/tracking/sync-waypoints', {
  method: 'POST',
  headers: { 'Authorization': `Bearer ${token}` },
  body: JSON.stringify({
    session_id,
    waypoints,
    is_final: false
  })
});

// 3. End session
const endResponse = await fetch('/api/v1/tracking/stop-session', {
  method: 'POST',
  headers: { 'Authorization': `Bearer ${token}` },
  body: JSON.stringify({
    session_id,
    end_location: {
      latitude: finalLocation.lat,
      longitude: finalLocation.lng,
      timestamp: new Date().toISOString()
    }
  })
});

const { data: { stats, analysis } } = await endResponse.json();
// Show stats and AI health recommendations to user

// 4. View history
const historyResponse = await fetch(
  `/api/v1/tracking/daily-history?date=${today}`,
  { headers: { 'Authorization': `Bearer ${token}` } }
);
const { data: { sessions, combined_waypoints } } = await historyResponse.json();
// Use combined_waypoints to replay journey on map
```

---

## Performance Tips

1. **Waypoint Sync**: Send in batches of 10-15 GPS points, not individually
2. **Zone Lookup**: Zones are cached locally; refresh every 30 seconds max
3. **Groq API**: Timeout is 15 seconds; falls back to rule-based diagnosis
4. **Database**: Add composite index on (user_id, started_at) for date queries
5. **Data Retention**: Archive sessions older than 90 days to Firestore archive bucket

---

## Troubleshooting

### "Session not found"
- Session ID might have typo
- Session might have been deleted (>90 days old)
- Session might belong to different user

### "Invalid coordinates"
- Latitude must be between -90 and 90
- Longitude must be between -180 and 180
- For Semarang area: lat -7.0 to -6.9, lon 110.4 to 110.5

### "Unauthorized"
- Missing `Authorization` header
- Token expired (try refreshing)
- Token from different Firebase project

### "Groq API unavailable"
- Falls back to rule-based diagnosis automatically
- Health analysis still provided, just not AI-generated
- Check `GROQ_API_KEY` environment variable

---

## API Versions

Current: `v1`
- Base: `/api/v1/`
- All endpoints documented above

Future: `v2` (not yet planned)

---

## Rate Limiting (Recommended)

Suggested limits per user:
- `calculate-route`: 10 req/min
- `start-session`: 5 req/day
- `sync-waypoints`: 100 req/day
- `stop-session`: 5 req/day
- `daily-history`: 50 req/day

Can be implemented via middleware if needed.

