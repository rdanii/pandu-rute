# Architecture (proposed)

Status: **proposal.** Only the parts marked *built* exist in the repo.

## Components

```
┌──────────────┐   HTTPS/JSON   ┌──────────────────┐        ┌───────────────────────┐
│ Mobile app   │ ─────────────▶ │ Go REST API      │ ─────▶ │ Routing engine        │
│ Expo / RN /  │                │ backend/         │        │ (candidate: OTP, TBD) │
│ TypeScript   │                │                  │        └───────────────────────┘
└──────────────┘                │                  │        ┌───────────────────────┐
                                │                  │ ─────▶ │ PostgreSQL + PostGIS  │
                                └──────────────────┘        │ (future)              │
                                                            └───────────────────────┘
```

| Component | Status | Notes |
|-----------|--------|-------|
| Go REST API | *built (health only)* | `GET /healthz`. Uses the standard library `net/http` with Go 1.22+ method routing. No framework. |
| Mobile app | not scaffolded | Expo + TypeScript, created with `create-expo-app`. Blocked on Node.js being installed. |
| Routing engine | undecided | See [routing-data-evaluation.md](routing-data-evaluation.md). |
| PostgreSQL/PostGIS | future | See below. |

## Responsibilities (intended)

- **Mobile app:** UI, user location, and showing itineraries in plain Indonesian ("anti bingung" means "no confusion"). It talks only to the Go API, never directly to the routing engine.
- **Go API:** the single public interface. It validates requests and adapts the routing engine's responses into a stable app-facing contract. Later it will also handle app-specific data such as stop names, transfer hints and user feedback. This boundary lets the routing engine be swapped without changing the app.
- **Routing engine:** computes multimodal itineraries from GTFS and street-network (OSM) data.

No API contract beyond `/healthz` is defined yet. Trip-planning endpoints will be
designed once the routing engine is chosen.

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

## Deliberately out of scope for the bootstrap

Docker, CI/CD, authentication, database migrations, trip planning, and a remote Git repository (its visibility needs confirming first).
