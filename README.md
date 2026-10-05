# PulsePoll

PulsePoll is a live polling app built for the GUVI/HCL developer internship task. A poll creator signs in, publishes a question, shares one link, and watches votes arrive in real time. Voters do not need an account and can vote once per browser.

## What is included

- React + Vite frontend with a responsive creator dashboard and public voting page.
- Go + Gin backend with secure cookie sessions, validation, rate limits, and a small JSON API.
- MongoDB for users, sessions, polls, vote totals, and per-browser ballot records.
- Redis for Pub/Sub live notifications and rate-limit counters. The browser consumes the stream through Server-Sent Events (SSE).
- A multi-stage Dockerfile for a Go-friendly host such as Render, Railway, Fly.io, or a VPS.

## Run locally

You need Node.js 20+, Go 1.23+, MongoDB 7/8+, and Redis 7+.

### 1. Start MongoDB and Redis

Use local services or hosted equivalents. For a quick local setup, MongoDB listens on `mongodb://127.0.0.1:27017` and Redis listens on `redis://127.0.0.1:6379/0`.

### 2. Build the frontend

```bash
cd frontend
npm install
npm run build
```

### 3. Start the Go server

Run this from the `backend` directory. The same process serves the compiled React files from `../frontend/dist`.

```bash
cd ../backend
export APP_ORIGIN=http://localhost:8080
export COOKIE_SECRET='replace-with-a-random-string-at-least-32-characters-long'
export MONGODB_URI='mongodb://127.0.0.1:27017'
export MONGODB_DB='pulsepoll'
export REDIS_URL='redis://127.0.0.1:6379/0'
export STATIC_DIR='../frontend/dist'
go mod tidy
go run .
```

Open <http://localhost:8080>. For PowerShell, use `$env:NAME = 'value'` instead of `export NAME=value`.

### Frontend development mode

When editing React, run Vite in a second terminal:

```bash
cd frontend
npm run dev
```

Set `APP_ORIGIN=http://localhost:5173` for that session. Vite proxies `/api` to the Go server, so the browser still uses one origin.

## API and realtime flow

1. `POST /api/auth/signup` or `/api/auth/login` creates a seven-day HttpOnly session cookie.
2. `POST /api/polls` validates and stores a poll in MongoDB.
3. `GET /api/polls/:id` creates a signed anonymous visitor cookie and returns the current snapshot.
4. `POST /api/polls/:id/vote` atomically increments the selected option and records the browser ballot in MongoDB. It then publishes `pulsepoll:poll:<id>` through Redis.
5. `GET /api/polls/:id/events` subscribes to that Redis channel and emits SSE `results` events. A ten-second MongoDB reconciliation also protects against a missed Pub/Sub message.

The vote update is conditional on `open: true`, the selected option, the capacity limit, and a missing ballot for the current visitor. That makes duplicate clicks and network retries safe.

## Deploy

Create a MongoDB Atlas database and a Redis Cloud/Upstash Redis database, then deploy this repository as a Docker web service. The container listens on the host-provided `PORT` and serves `frontend/dist`.

Set these production environment variables:

```text
APP_ORIGIN=https://your-public-host.example
COOKIE_SECRET=<long-random-secret>
MONGODB_URI=<mongodb-atlas-connection-string>
MONGODB_DB=pulsepoll
REDIS_URL=<redis-connection-string>
STATIC_DIR=/app/frontend/dist
```

After deployment, verify `GET /healthz` returns `{"status":"ok"}` and test one creator browser plus two private/incognito voter windows. Keep the public link and a short screen recording for the internship submission.

## Security notes

- Passwords use bcrypt; raw passwords are never stored.
- Sessions and visitor identifiers are random, hashed server-side, and held in HttpOnly/SameSite cookies.
- Mutating API calls require the app request header, a matching Origin, JSON content, and a small body limit.
- MongoDB indexes cover unique emails, session expiry, and creator poll listings.
- Redis-backed rate limits protect authentication, poll creation, and voting endpoints.
