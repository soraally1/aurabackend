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
| Protected routes | `Authorization: Bearer <idToken>` checked with Admin SDK `VerifyIDTokenAndCheckRevoked` |
| Refresh | Firebase secure token REST endpoint |
| Logout | Admin SDK `RevokeRefreshTokens` (signs the user out on all devices) |

Passwords are stored only in Firebase Auth, never in Firestore.

## Firebase setup

1. **Authentication → Sign-in method** → enable **Email/Password**.
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
{ "name": "Daffa", "email": "daffa@example.com", "password": "supersecret" }
```
`201` → `{ user, tokens: { idToken, refreshToken, expiresIn } }`
Errors: `422 VALIDATION_ERROR`, `409 EMAIL_EXISTS`

### `POST /api/auth/login`
```json
{ "email": "daffa@example.com", "password": "supersecret" }
```
`200` → `{ user, tokens }`
Errors: `401 INVALID_CREDENTIALS`, `403 USER_DISABLED`, `429 TOO_MANY_ATTEMPTS`

### `POST /api/auth/refresh`
```json
{ "refreshToken": "..." }
```
`200` → `{ tokens }`. The ID token expires after 1 hour, so call this endpoint to get a new one.

### `GET /api/auth/me`
Header `Authorization: Bearer <idToken>` → `200 { user }`

### `POST /api/auth/logout`
Header `Authorization: Bearer <idToken>` → revokes all of the user's sessions.

### `GET /api/health`
Liveness check.

### curl example

```bash
BASE=http://localhost:8080
curl -X POST $BASE/api/auth/register -H 'Content-Type: application/json' \
  -d '{"name":"Daffa","email":"daffa@example.com","password":"supersecret"}'

TOKEN=$(curl -s -X POST $BASE/api/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"daffa@example.com","password":"supersecret"}' | jq -r .data.tokens.idToken)

curl $BASE/api/auth/me -H "Authorization: Bearer $TOKEN"
```
