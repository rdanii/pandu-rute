# Pandu Rute: Anti Bingung

Mobile-first journey planner for Indonesian public transport, starting with Jakarta.

> Status: **repository foundation only.** There is a Go API with a health endpoint
> plus an unmodified Expo app template, and project docs. Trip planning, the data layer
> and mobile features are not built yet.

## Layout

```
backend/   Go REST API (module: pandu-rute/backend)
  cmd/api/            entrypoint
  internal/httpapi/   routes and handlers
mobile/    React Native + Expo (TypeScript) app — template only, see mobile/README.md
docs/      architecture and routing/data research
```

## Prerequisites

| Tool | Needed for | Version used at bootstrap |
|------|------------|---------------------------|
| Go | backend | 1.27.1 (`go.mod` declares `go 1.27.1`) |
| Node.js LTS + npm | mobile | Node v24.21.0, npm 11.19.0 (installed via nvm) |
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

```bash
cd mobile
npm install
npx expo-doctor        # project health checks
npx tsc --noEmit       # type check
npx expo start         # dev server (not yet verified on a device)
```

See [mobile/README.md](mobile/README.md) for versions, more checks and WSL2 notes.

## Docs

- [docs/architecture.md](docs/architecture.md): proposed architecture
- [docs/routing-data-evaluation.md](docs/routing-data-evaluation.md): routing engine and transit data research, with open decisions
