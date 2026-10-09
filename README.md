# Pandu Rute: Anti Bingung

Mobile-first journey planner for Indonesian public transport, starting with Jakarta.

> Status: **repository foundation only.** There is a Go API with a health endpoint
> and project docs. Trip planning, the data layer and the mobile app are not built yet.

## Layout

```
backend/   Go REST API (module: pandu-rute/backend)
  cmd/api/            entrypoint
  internal/httpapi/   routes and handlers
mobile/    React Native + Expo (TypeScript) app — placeholder, see mobile/README.md
docs/      architecture and routing/data research
```

## Prerequisites

| Tool | Needed for | Version used at bootstrap |
|------|------------|---------------------------|
| Go | backend | 1.27.1 (`go.mod` declares `go 1.27.1`) |
| Node.js LTS + npm | mobile | **not installed yet**. Required before the Expo app can be scaffolded |
| Git | version control | 2.43 |
| PostgreSQL + PostGIS | future data layer | not used yet; see [docs/architecture.md](docs/architecture.md) |

## Backend

```bash
cd backend
go test ./...          # unit tests
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

`/healthz` only reports that the process is serving HTTP. It does not check any dependencies, because none exist yet.

## Mobile

Not scaffolded yet. Follow [mobile/README.md](mobile/README.md) once Node.js is installed.

## Docs

- [docs/architecture.md](docs/architecture.md): proposed architecture
- [docs/routing-data-evaluation.md](docs/routing-data-evaluation.md): routing engine and transit data research, with open decisions
