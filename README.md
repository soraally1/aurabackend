# Aura Backend

Go API for **register / login**. It uses **Firebase Authentication** for accounts and **Cloud Firestore** for user profiles, and it's set up to deploy on **Vercel**.

## Project structure

```
api/index.go            Vercel serverless entrypoint (every route is rewritten here)
cmd/server/main.go      Local dev server (go run ./cmd/server)
pkg/app/                Router + wiring
pkg/handlers/           Endpoint handlers (auth, health)
pkg/middleware/         Bearer-token auth, CORS, panic recovery
pkg/firebase/           Admin SDK clients + Firebase Auth REST client
pkg/users/              Firestore user profile store
pkg/config/             Environment config
pkg/httpx/              JSON response helpers
vercel.json             Rewrites all paths -> /api/index
```

## How it works

| Step | Mechanism |
|---|---|
| Register | Admin SDK `CreateUser` → Firestore `users/{uid}` doc → password sign-in to return tokens |
| Login | Firebase Auth REST `accounts:signInWithPassword` (the Admin SDK can't check passwords) |
| Google Login | Verifies Firebase ID token or exchanges Google ID/Access token via `accounts:signInWithIdp` → Firestore `users/{uid}` doc backfill |
| Protected routes | `Authorization: Bearer <idToken>` checked with Admin SDK `VerifyIDTokenAndCheckRevoked` |
| Refresh | Firebase secure token REST endpoint |
| Logout | Admin SDK `RevokeRefreshTokens` (signs the user out on all devices) |

Passwords are stored only in Firebase Auth, never in Firestore.

## Firebase setup

1. **Authentication → Sign-in method** → enable **Email/Password** and **Google**.
2. **Firestore Database** → create a database.
3. **Project settings → Service accounts** → *Generate new private key* (this downloads a JSON file).
4. **Project settings → General** → copy the **Web API Key**.

## Environment variables

| Name | Required | Description |
|---|---|---|
| `FIREBASE_SERVICE_ACCOUNT_KEY` | ✅ | Service account JSON, as one line or base64 |
| `FIREBASE_WEB_API_KEY` | ✅ | Firebase Web API key |
| `FIREBASE_PROJECT_ID` | | Overrides `project_id` from the service account |
| `FIRESTORE_DATABASE_ID` | | Defaults to `(default)` |
| `USERS_COLLECTION` | | Defaults to `users` |
| `ALLOWED_ORIGINS` | | Comma-separated CORS origins, defaults to `*` |

To base64-encode the key (this avoids trouble with multi-line values):

```bash
base64 -i serviceAccountKey.json | tr -d '\n'
```

## Run locally

```bash
cp .env.example .env   # fill in values
go run ./cmd/server    # http://localhost:8080
```

## Deploy to Vercel

```bash
npm i -g vercel
vercel                 # first deploy / link project
vercel env add FIREBASE_SERVICE_ACCOUNT_KEY
vercel env add FIREBASE_WEB_API_KEY
vercel --prod
```

You can also import the Git repo in the Vercel dashboard and add the env vars under *Settings → Environment Variables*. Redeploy after you change env vars.

## API

All responses use this envelope:

```json
{ "success": true,  "data": { ... } }
{ "success": false, "error": { "code": "INVALID_CREDENTIALS", "message": "..." } }
```

### `POST /api/auth/register`
```json
{
  "name": "Daffa",
  "email": "daffa@example.com",
  "password": "supersecret",
  "usia": 25,
  "tinggi_badan": 170,
  "berat_badan": 65,
  "waktu_di_jalan": 2
}
```
`201` → `{ user, tokens: { idToken, refreshToken, expiresIn } }`
Errors: `422 VALIDATION_ERROR`, `409 EMAIL_EXISTS`

### `POST /api/auth/login`
```json
{ "email": "daffa@example.com", "password": "supersecret" }
```
`200` → `{ user, tokens }`
Errors: `401 INVALID_CREDENTIALS`, `403 USER_DISABLED`, `429 TOO_MANY_ATTEMPTS`

### `POST /api/auth/google`
Menerima token dari Firebase Client SDK (`idToken` Firebase) atau Google OAuth token (`idToken` / `accessToken` Google):
```json
{
  "idToken": "<firebase_id_token_atau_google_id_token>",
  "accessToken": "<opsional_google_access_token>",
  "name": "Daffa",
  "usia": 25,
  "tinggi_badan": 170,
  "berat_badan": 65,
  "waktu_di_jalan": 2
}
```
Field `name`, `usia`, `tinggi_badan`, `berat_badan`, `waktu_di_jalan` bersifat opsional (dapat diisi nanti via `PUT /api/auth/profile`).

`200` → `{ user, tokens: { idToken, refreshToken, expiresIn } }`
Errors: `401 INVALID_CREDENTIALS`, `403 USER_DISABLED`, `403 PROVIDER_NOT_ALLOWED`, `422 VALIDATION_ERROR`

### `POST /api/auth/refresh`
```json
{ "refreshToken": "..." }
```
`200` → `{ tokens }`. The ID token expires after 1 hour, so call this endpoint to get a new one.

### `GET /api/auth/me`
Header `Authorization: Bearer <idToken>` → `200 { user }`

### `PUT /api/auth/profile`
Header `Authorization: Bearer <idToken>`
Update satu atau lebih data profil:
```json
{
  "name": "Daffa",
  "usia": 26,
  "tinggi_badan": 172,
  "berat_badan": 67,
  "waktu_di_jalan": 1.5
}
```
`200` → `{ user }`
Errors: `401 INVALID_REFRESH_TOKEN / UNAUTHORIZED`, `422 VALIDATION_ERROR`

### `POST /api/auth/logout`
Header `Authorization: Bearer <idToken>` → revokes all of the user's sessions.

### `GET /api/health`
Liveness check.

### curl example

```bash
BASE=http://localhost:8080
curl -X POST $BASE/api/auth/register -H 'Content-Type: application/json' \
  -d '{
    "name": "Daffa",
    "email": "daffa@example.com",
    "password": "supersecret",
    "usia": 25,
    "tinggi_badan": 170,
    "berat_badan": 65,
    "waktu_di_jalan": 2
  }'

TOKEN=$(curl -s -X POST $BASE/api/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"daffa@example.com","password":"supersecret"}' | jq -r .data.tokens.idToken)

curl $BASE/api/auth/me -H "Authorization: Bearer $TOKEN"

# Update profil
curl -X PUT $BASE/api/auth/profile -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"usia": 26, "tinggi_badan": 171, "berat_badan": 66, "waktu_di_jalan": 1.5}'
```

---

## Hospital & Navigation Endpoints

### 1. Rute & Navigasi Turn-by-Turn
- **Method & Path:** `GET /api/v1/routes`
- **Query Params:**
  - `origin_lat`: Latitude posisi awal / user (required)
  - `origin_lon`: Longitude posisi awal / user (required)
  - `dest_lat`: Latitude posisi tujuan / RS (required)
  - `dest_lon`: Longitude posisi tujuan / RS (required)
- **Response JSON:**
```json
{
  "success": true,
  "data": {
    "distance_meters": 3301.3,
    "duration_seconds": 361.7,
    "coordinates": [
      [110.41257, -7.001692],
      [110.412745, -7.001899]
    ],
    "steps": [
      {
        "name": "Jl. Veteran",
        "distance": 350.0,
        "duration": 45.0,
        "modifier": "left",
        "type": "turn"
      }
    ]
  }
}
```

### 2. Rumah Sakit Terdekat (Nearby)
- **Method & Path:** `GET /api/v1/hospitals/nearby`
- **Query Params:**
  - `lat`: Latitude user (required)
  - `lon`: Longitude user (required)
  - `radius_km`: Radius pencarian dalam KM (default: `15`)
  - `limit`: Jumlah maksimal RS dikembalikan (default: `10`)
- **Response JSON:**
```json
{
  "success": true,
  "data": [
    {
      "id": "rsup-dr-kariadi",
      "name": "RSUP Dr. Kariadi",
      "address": "Jl. DR. Sutomo No.16, Randusari, Semarang Selatan",
      "latitude": -6.9934,
      "longitude": 110.4087,
      "rating": 4.8,
      "user_ratings_total": 4520,
      "phone": "(024) 8413476",
      "open_hours": "Buka 24 Jam",
      "type": "hospital",
      "image_url": "https://...",
      "facilities": ["IGD 24 Jam", "Rawat Inap", "Rawat Jalan", "Farmasi 24 Jam", "Spesialis Paru"],
      "distance_km": 0.0
    }
  ]
}
```

### 3. List & Detail Rumah Sakit
- `GET /api/v1/hospitals` (List semua RS, opsi filter `?q=kariadi`)
- `GET /api/v1/hospitals/{id}` (Detail RS berdasarkan slug ID)

### 4. Hospital Scraper & Seeder CLI
Tool scraping & sinkronisasi data rumah sakit ke Firestore:
```bash
# Menjalankan seeder langsung ke Firestore:
go run ./cmd/seeder

# Menjalankan dengan mode preview (dry-run):
go run ./cmd/seeder -dry-run

# Custom koordinat dan radius:
go run ./cmd/seeder -lat -6.9934 -lon 110.4087 -radius 20000
```
Fitur Seeder:
- Mendukung Google Places API (New) jika key tersedia.
- Fallback otomatis ke Overpass API (OpenStreetMap) dan dataset terkurasi Semarang.
- Strict blacklist filter: otomatis mengecualikan Puskesmas, Klinik, Apotek, dan Lab.

---

## Mobility Tracking & Daily Exposure Analysis (Groq Llama-3.3-70b-versatile)

### 1. Kirim Batch Waypoint Harian
- **Method & Path:** `POST /api/v1/mobility/batch`
- **Headers:** `Content-Type: application/json`, opsional `Authorization: Bearer <idToken>`
- **Request Body JSON:**
```json
{
  "user_id": "user-123",
  "date": "2026-10-15",
  "waypoints": [
    {
      "latitude": -6.9934,
      "longitude": 110.4087,
      "timestamp": "2026-10-15T08:00:00Z",
      "duration_minutes": 180,
      "is_stay": true
    },
    {
      "latitude": -6.9893,
      "longitude": 110.4229,
      "timestamp": "2026-10-15T11:15:00Z",
      "duration_minutes": 15,
      "is_stay": false
    }
  ]
}
```
- **Response JSON:**
```json
{
  "success": true,
  "data": {
    "id": "user-123_2026-10-15",
    "user_id": "user-123",
    "date": "2026-10-15",
    "stats": {
      "total_indoor_minutes": 180.0,
      "total_outdoor_minutes": 15.0,
      "average_outdoor_aqi": 132.0,
      "average_outdoor_pm25": 48.5,
      "max_aqi": 132,
      "worst_area_name": "Simpang Lima Semarang",
      "worst_area_aqi": 132,
      "waypoints_processed": 2
    },
    "analysis": {
      "impact_score": 45.2,
      "risk_level": "Sedang",
      "summary": "Paparan polusi Anda hari ini berada pada tingkat sedang setelah beraktivitas luar ruangan selama 15 menit di sekitar Simpang Lima Semarang.",
      "doctor_diagnosis": "Terdeteksi akumulasi partikulat halus (PM2.5 rata-rata 48.5 µg/m³) akibat perjalanan luar ruangan...",
      "recommendations": [
        "Gunakan masker standar medis atau KF94 saat bepergian melintasi jalan padat kendaraan.",
        "Bilas wajah dan bersihkan rongga hidung dengan larutan saline steril setelah sampai di rumah."
      ]
    }
  }
}
```

### 2. Ambil Hasil Analisis Harian
- **Method & Path:** `GET /api/v1/mobility/daily-analysis?date=YYYY-MM-DD`
- **Query Params:**
  - `date`: `YYYY-MM-DD` (optional, default hari ini)
  - `user_id`: ID pengguna (optional jika menggunakan token `Authorization: Bearer <token>`)


