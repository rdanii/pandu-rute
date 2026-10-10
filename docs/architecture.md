# Architecture

Status: parts marked *built* exist in the repo and are covered by tests;
everything else is a plan.

## Components

```
┌──────────────┐   HTTPS/JSON   ┌──────────────────┐  GraphQL   ┌──────────────────────────┐
│ Mobile app   │ ─ ─ ─ ─ ─ ─ ─▶ │ Go REST API      │ ─────────▶ │ OpenTripPlanner 2.10     │
│ Expo / RN /  │  (not wired    │ backend/         │            │ TransJakarta GTFS + OSM  │
│ TypeScript   │   yet)         │                  │            └──────────────────────────┘
└──────────────┘                │                  │            ┌──────────────────────────┐
                                │                  │ ─ ─ ─ ─ ─▶ │ PostgreSQL + PostGIS     │
                                └──────────────────┘            │ (future)                 │
                                                                └──────────────────────────┘
```

| Component | Status | Notes |
|-----------|--------|-------|
| Go REST API | *built* | `GET /healthz`, `POST /v1/trip-plans` ([OpenAPI](api/openapi.yaml)). Standard library `net/http` only. |
| OTP adapter | *built* | `backend/internal/otp`, OTP 2.10 GTFS GraphQL API (`/otp/gtfs/v1`, `planConnection`). |
| Mobile app | *home screen only* | Indonesian home screen with a free-text destination field. It does not call the API yet. |
| Routing engine | OTP 2.10.0, local only | Feasibility run on a developer machine; no deployment. See [routing-data-evaluation.md](routing-data-evaluation.md). |
| PostgreSQL/PostGIS | future | See below. |

## Go API layers (built)

- `internal/httpapi`: HTTP transport. Strict JSON decoding (4 KiB limit, unknown
  fields rejected), app-owned response and error envelope with fixed Indonesian
  messages and a request ID. Imports neither OTP types nor OTP errors.
- `internal/tripplan`: app-owned model and `Planner` interface. The service
  validates queries, rejects planner output without attribution or with invalid
  fields, drops walk-only itineraries (see limitations), merges structurally
  identical itineraries, and truncates to `maxItineraries`.
- `internal/otp`: implements `tripplan.Planner`. GraphQL query and wire structs are
  private. Upstream error text is never forwarded or logged. Configured with
  `OTP_BASE_URL` (default `http://localhost:8082`); startup does not contact OTP.

Behaviour:

- `/healthz` is liveness only and does not depend on OTP.
- OTP unreachable or answering 5xx/429 → `503 ROUTING_UNAVAILABLE`; upstream deadline
  (6 s inside an 8 s request budget, below the 10 s server write timeout) →
  `504 ROUTING_TIMEOUT`; malformed or unexpected upstream data → `502 UPSTREAM_INVALID_RESPONSE`.
- Any OTP routing error takes precedence over itineraries returned with it.
  `LOCATION_NOT_FOUND` / `OUTSIDE_BOUNDS` → `422 LOCATION_NOT_ROUTABLE` (side from the
  structured `inputField`); all other codes, including unknown ones → `422 NO_ROUTE`.
- Logs record request ID, status, error code, itinerary count and duration only.

## Current limitations

- **No geocoding.** Routing input is coordinates only; `label` is echoed, never resolved.
- **No service-area check.** `OUT_OF_COVERAGE` is never produced; locations OTP cannot
  link (e.g. outside the DKI-only OSM extract) appear as `LOCATION_NOT_ROUTABLE`.
- **Less specific errors.** "Date outside the loaded service period" and "walking is
  better" both surface as `NO_ROUTE`; the contract has no codes for them.
- **Walk-only journeys are not returned.** No maximum walking distance is approved, so
  the default policy drops every itinerary without a transit leg. Walking access,
  egress and transfer legs of transit itineraries are kept.
- **Timetable accuracy is unverified.** All itinerary times are labelled
  `frequency_estimate`, because OTP 2.10 does not expose whether a trip is
  frequency-based. A successful response does not prove real-world accuracy.
- **Duplicates are merged by structure, not by root cause.** OTP returned
  near-identical itineraries a minute apart in local tests; the cause is not proven.
  Fewer than `maxItineraries` results may remain because extra candidates are not requested.
- **No data-version metadata.** `dataVersion` is always `null`; dataset checksums and
  freshness are not loaded at runtime.
- **No `/readyz`, route geometry, other operators, realtime data or deployment.**

## PostgreSQL / PostGIS (future capability)

Not implemented: no driver, schema, migrations or connection config yet.

Expected uses, all of which need confirming once trip planning is designed:

- Storing app-specific stop, station and landmark data with spatial indexes, e.g. "nearest halte".
- Possibly storing imported GTFS data for search and display. The routing engine would still read GTFS directly.

When it is added, the expected approach is:

1. Connection settings come from environment variables, e.g. `DATABASE_URL`.
2. Pick a migration tool at that time. There is no migration tool yet.
3. Run PostGIS locally. Docker Compose is one option, but it was left out of the bootstrap on purpose.

The `psql` client (16.x) is installed on the bootstrap machine. A running server with PostGIS has not been checked.

## Not yet in place

Docker, CI/CD, authentication and database migrations.
