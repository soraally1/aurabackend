# API Endpoints Quick Start Guide

All 5 endpoints are now implemented and ready to use. This guide shows how to test them.

---

## Base URLs

- **Development**: `http://localhost:8080/api/v1`
- **Production**: `https://api.aura-health.app/api/v1`

Alternative path patterns (all work):
- `/api/v1/...` (recommended)
- `/api/...`
- `/...`

---

## 1. Calculate Route with Health Warnings

**Endpoint**: `POST /navigation/calculate-route`  
**Auth**: Not required (public)  
**Purpose**: Get turn-by-turn navigation with pollution warnings

### Request Example

```bash
curl -X POST http://localhost:8080/api/v1/navigation/calculate-route \
  -H "Content-Type: application/json" \
  -d '{
    "origin": {
      "latitude": -6.9932,
      "longitude": 110.4203
    },
    "destination": {
      "latitude": -7.0015,
      "longitude": 110.4128
    },
    "mode": "driving"
  }'
```

### Response

```json
{
  "success": true,
  "data": {
    "route_id": "route_xyz123_semarang",
    "distance_meters": 2450,
    "duration_seconds": 480,
    "coordinates": [[110.4203, -6.9932], ...],
    "steps": [
      {
        "instruction": "Belok kanan ke Jl. Pemuda",
        "distance_meters": 350,
        "duration_seconds": 60,
        "maneuver": "turn-right",
        "start_location": {"latitude": -6.9932, "longitude": 110.4203},
        "end_location": {"latitude": -6.9940, "longitude": 110.4210}
      }
    ],
    "health_warnings": [
      {
        "warning_id": "hw_001",
        "type": "high_pollution",
        "severity": "high",
        "title": "Zona Polusi Tinggi",
        "aqi": 156,
        "pm2_5": 145,
        "health_risks": ["Iritasi mata dan kulit"],
        "recommendations": ["Gunakan masker N95"]
      }
    ],
    "map_markers": [...]
  }
}
```

### Parameters

- `origin` (required): `{latitude, longitude}`
- `destination` (required): `{latitude, longitude}`
- `waypoints` (optional): Array of `{latitude, longitude}`
- `mode` (optional): "driving" (default) or "walking"
- `avoid_tolls` (optional): boolean
- `avoid_highways` (optional): boolean

---

## 2. Start Tracking Session

**Endpoint**: `POST /tracking/start-session`  
**Auth**: Required (Firebase Bearer token)  
**Purpose**: Create new GPS tracking session

### Request Example

```bash
curl -X POST http://localhost:8080/api/v1/tracking/start-session \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{
    "user_id": "user_abc123",
    "session_name": "Pulang Kantor",
    "start_location": {
      "latitude": -6.9932,
      "longitude": 110.4203,
      "timestamp": "2025-01-15T08:00:00Z"
    }
  }'
```

### Response

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

### Key Points

- Returns `session_id` - save this for sync and stop operations
- Status always "active" for new sessions
- User must be authenticated with Firebase

---

## 3. Sync Waypoints During Tracking

**Endpoint**: `POST /tracking/sync-waypoints`  
**Auth**: Required (Firebase Bearer token)  
**Purpose**: Submit GPS waypoints collected during journey

### Request Example

```bash
curl -X POST http://localhost:8080/api/v1/tracking/sync-waypoints \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{
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
  }'
```

### Response

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

### Key Points

- Call this periodically during journey (every 10-15 waypoints recommended)
- `is_final`: false for intermediate syncs, true for final sync
- Each waypoint is enriched with zone AQI/PM2.5 data server-side

---

## 4. Stop Tracking Session

**Endpoint**: `POST /tracking/stop-session`  
**Auth**: Required (Firebase Bearer token)  
**Purpose**: End tracking session and get health analysis

### Request Example

```bash
curl -X POST http://localhost:8080/api/v1/tracking/stop-session \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{
    "session_id": "session_user_abc123_1705310400",
    "user_id": "user_abc123",
    "end_location": {
      "latitude": -7.0015,
      "longitude": 110.4128,
      "timestamp": "2025-01-15T09:30:00Z"
    }
  }'
```

### Response (With AI Analysis)

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
      "summary": "Anda terpapar polusi sedang-tinggi selama 85 menit terutama di area Kaligawe...",
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

### Key Points

- Returns complete exposure statistics
- Includes AI-powered health analysis (via Groq)
- Classifies indoor vs outdoor time
- Identifies worst pollution area visited
- Provides personalized health recommendations

---

## 5. Get Daily Journey History

**Endpoint**: `GET /tracking/daily-history`  
**Auth**: Required (Firebase Bearer token)  
**Purpose**: Fetch all journeys and waypoints for a specific date

### Request Example

```bash
curl -X GET "http://localhost:8080/api/v1/tracking/daily-history?user_id=user_abc123&date=2025-01-15" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

### Response

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
        "waypoints": [...]
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

### Query Parameters

- `user_id` (required): User's unique ID
- `date` (required): Date in YYYY-MM-DD format

### Key Points

- Returns all sessions for that day
- Combined waypoints in chronological order
- Daily aggregated statistics
- Useful for journey replay functionality

---

## Error Responses

All endpoints return errors in this format:

```json
{
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable error message"
  }
}
```

### Common Errors

**400 Bad Request**
```json
{
  "success": false,
  "error": {
    "code": "INVALID_INPUT",
    "message": "request body must be valid JSON"
  }
}
```

**401 Unauthorized**
```json
{
  "success": false,
  "error": {
    "code": "UNAUTHORIZED",
    "message": "Missing or invalid Authorization header"
  }
}
```

**404 Not Found**
```json
{
  "success": false,
  "error": {
    "code": "SESSION_NOT_FOUND",
    "message": "Session not found or expired"
  }
}
```

**500 Internal Server Error**
```json
{
  "success": false,
  "error": {
    "code": "SERVER_ERROR",
    "message": "Internal server error"
  }
}
```

---

## Testing Flow

### Complete Journey Flow

```bash
# 1. Start tracking session
SESSION_ID=$(curl -s -X POST http://localhost:8080/api/v1/tracking/start-session \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{...}' | jq -r '.data.session_id')

# 2. Sync waypoints (simulate journey)
curl -X POST http://localhost:8080/api/v1/tracking/sync-waypoints \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "'$SESSION_ID'",
    "user_id": "user_abc123",
    "waypoints": [...],
    "is_final": false
  }'

# 3. Stop session and get analysis
curl -X POST http://localhost:8080/api/v1/tracking/stop-session \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "'$SESSION_ID'",
    "user_id": "user_abc123",
    "end_location": {...}
  }'

# 4. Get daily history
curl -X GET "http://localhost:8080/api/v1/tracking/daily-history?user_id=user_abc123&date=2025-01-15" \
  -H "Authorization: Bearer $TOKEN"
```

---

## Sample Coordinates (Semarang)

Use these real coordinates for testing:

| Location | Latitude | Longitude |
|----------|----------|-----------|
| Simpang Lima | -6.9932 | 110.4203 |
| Jl. Pemuda | -6.9940 | 110.4210 |
| Tugu Muda | -6.9700 | 110.3900 |
| Kaligawe Industrial | -6.9587 | 110.4578 |
| Randusari | -7.0050 | 110.4300 |

---

## Getting Firebase Bearer Token

For protected endpoints, you need a valid Firebase JWT token:

```bash
# Login
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@aura-health.app",
    "password": "TestPassword123!"
  }'

# Extract token from response
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login ... | jq -r '.data.id_token')
```

Then use in Authorization header:
```bash
-H "Authorization: Bearer $TOKEN"
```

---

## Debugging Tips

1. **Check Firestore Collections**
   - `tracking_sessions` - Active and completed sessions
   - `cached_routes` - Cached route calculations
   - `environmental_zones` - Pollution zones data

2. **Monitor Logs**
   - OSRM routing errors
   - Groq API failures (falls back to rule-based diagnosis)
   - Firestore connection issues

3. **Verify Environment**
   - Ensure GROQ_API_KEY is set
   - Check Firebase credentials are valid
   - Verify OSRM is accessible (http://router.project-osrm.org)

4. **Test Individual Components**
   - GET /zones/nearby - Test zone queries
   - POST /zones/seed - Populate initial zones
   - GET /health - Verify server is running

---

## Rate Limits (Recommended)

- `calculate-route`: 10 requests/minute per user
- `start-session`: 5 requests/day per user
- `sync-waypoints`: 100 requests/day per user
- `stop-session`: 5 requests/day per user
- `daily-history`: 50 requests/day per user

---

## Support

For issues or questions:
1. Check this quick start guide
2. Review backend-api.md for detailed specifications
3. Check IMPLEMENTATION_SUMMARY.md for architecture details
4. Review error messages in response
