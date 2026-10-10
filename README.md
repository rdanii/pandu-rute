# Pandu Rute: Anti Bingung

Mobile-first journey planner for Indonesian public transport, starting with Jakarta.

> Status: **early development, not production-ready.** The Go API serves `GET /healthz`
> and `POST /v1/trip-plans`, which asks a separately run OpenTripPlanner (OTP) 2.10
> instance for TransJakarta + walking itineraries. The mobile app has a home screen but
> does not call the API yet. Timetable accuracy has not been verified against real-world
> service.

## Layout

```
backend/   Go REST API (module: pandu-rute/backend)
  cmd/api/              entrypoint and configuration
  internal/httpapi/     HTTP routes, request validation, error envelope
  internal/tripplan/    app-owned trip-planning model and service
  internal/otp/         OpenTripPlanner 2.10 adapter
mobile/    React Native + Expo (TypeScript) app, see mobile/README.md
docs/      architecture, API contract (docs/api/openapi.yaml), routing/data research
```

## Prerequisites

| Tool | Needed for | Version used so far |
|------|------------|---------------------|
| Go | backend | 1.27.1 (`go.mod` declares `go 1.27.1`) |
| Node.js LTS + npm | mobile | Node v24.21.0, npm 11.19.0 (installed via nvm) |
| Java (JDK) + OTP 2.10.0 | trip planning | JDK 25 and `otp-shaded-2.10.0.jar` in a local experiment; not part of this repo |
| Git | version control | 2.43 |
| PostgreSQL + PostGIS | future data layer | not used yet; see [docs/architecture.md](docs/architecture.md) |

## Backend

```bash
cd backend
go test ./...          # unit tests (offline; no OTP or data needed)
go vet ./...           # static checks
go build ./...         # compile check
go run ./cmd/api       # serves on :8080 (override with PORT=xxxx)
```

Check that it is running:

```bash
curl -i http://localhost:8080/healthz
# HTTP/1.1 200 OK
# Content-Type: application/json
# {"status":"ok"}
```

`/healthz` only reports that the process is serving HTTP. It does not check OpenTripPlanner.

### Trip planning (OpenTripPlanner)

`POST /v1/trip-plans` takes origin and destination **coordinates** and returns
app-owned itineraries (contract: [docs/api/openapi.yaml](docs/api/openapi.yaml)).
It only works when a correctly configured OTP 2.10 instance is reachable:

- OTP must serve a graph built from the TransJakarta GTFS Static feed plus an
  OpenStreetMap extract. Building that graph is a manual, local step; graphs and the
  data they are built from live in the git-ignored `data/` folder and must not be committed.
- The API reaches OTP at `OTP_BASE_URL` (default `http://localhost:8082`). OTP's own
  default port is 8080, the same as the API's, so start OTP on 8082.
- The adapter expects OTP's feed id `1` for the TransJakarta feed (the id OTP assigned
  in the local build). A graph built with a different feed id is rejected as an invalid
  upstream response.

```bash
java -Xmx3G -jar otp-shaded-2.10.0.jar --load --port 8082 <graph-dir>
OTP_BASE_URL=http://localhost:8082 go run ./cmd/api
```

If OTP is not running, the API still starts and trip plans return `503 ROUTING_UNAVAILABLE`.
Every successful response carries TransJakarta (CC BY 4.0) and OpenStreetMap (ODbL)
attributions; `dataVersion` is always `null` for now.

### Not available yet

- stop search and geocoding (routing input is coordinates only; `label` is display text)
- real-time data
- transit operators other than TransJakarta
- service-area checks, `/readyz`, route geometry and deployment configuration

See [docs/architecture.md](docs/architecture.md) for the current limitations in detail.

## Mobile

```bash
cd mobile
npm install
npx expo-doctor        # project health checks
npx tsc --noEmit       # type check
npx expo start         # dev server; on WSL2 a phone may need --tunnel
```

See [mobile/README.md](mobile/README.md) for versions, the current home screen and its limitations.

## Docs

- [docs/architecture.md](docs/architecture.md): current architecture, behaviour and limitations
- [docs/api/openapi.yaml](docs/api/openapi.yaml): HTTP API contract
- [docs/routing-data-evaluation.md](docs/routing-data-evaluation.md): routing engine and transit data findings, with open questions
