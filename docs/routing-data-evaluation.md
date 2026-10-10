# Routing engine & transit data: evaluation

Status: **OpenTripPlanner 2.10.0 is the routing engine used by the backend adapter; it
has only been run locally.** Production hosting, data refresh and real-world accuracy
are open questions (§4).

Last reviewed: 2026-10-10.

> **About the evidence in this document.** Findings marked *(recorded 2026-10-09/10)* come
> from web research and a local feasibility experiment done on those dates. They were
> **not re-verified** when this document was last edited. Experiment files (downloaded
> feeds, OSM extract, built graph, validator reports, notes) are kept in the git-ignored
> `data/` folder and are not part of the repository.

## 1. Routing engine: OpenTripPlanner (OTP)

### What is decided and built

- The backend talks to OTP **2.10** through `backend/internal/otp`, which implements the
  app-owned `tripplan.Planner` interface. The mobile app never sees OTP data structures.
- The adapter uses OTP's GTFS GraphQL API at `/otp/gtfs/v1` and its `planConnection` field.
- OTP's licence is LGPL-3.0-or-later *(recorded 2026-10-09/10, from the project's LICENSE
  file)*. The API calls OTP over HTTP as a separate process; whether that fits the final
  distribution model has not been reviewed legally.

### OTP 2.10 API behaviour relied on by the adapter

Checked by GraphQL introspection and test queries against a local OTP 2.10.0 instance
*(recorded 2026-10-10)*:

- The schema has `planConnection` and **no** legacy `plan` field. OTP's documentation states
  that the REST API was removed in 2025.
- `planConnection(origin, destination, dateTime: {earliestDeparture}, first)` returns
  `routingErrors` and `edges { node { … } }`.
- Leg times are `LegTime` objects; the adapter uses `scheduledTime` (ISO-8601 with offset).
- Stop, route and trip ids carry an OTP feed prefix (`1:` in the local build).
- `RoutingError` has a structured `inputField` (`FROM`, `TO`, `DATE_TIME`, `VIA`), so
  origin/destination errors can be told apart without parsing descriptions. Its
  `RoutingErrorCode` enum has 8 values: `LOCATION_NOT_FOUND`, `NO_DIRECT_MODE_CONNECTION`,
  `NO_STOPS_IN_RANGE`, `NO_TRANSIT_CONNECTION`, `NO_TRANSIT_CONNECTION_IN_SEARCH_WINDOW`,
  `OUTSIDE_BOUNDS`, `OUTSIDE_SERVICE_PERIOD`, `WALKING_BETTER_THAN_TRANSIT`.
- OTP can return walk-only itineraries **together with** a routing error (seen with
  `OUTSIDE_SERVICE_PERIOD` and `WALKING_BETTER_THAN_TRANSIT`). The adapter therefore lets any
  routing error take precedence.
- No field reports whether a trip is frequency-based, so every itinerary is labelled
  `frequency_estimate`.

### Local feasibility experiment *(recorded 2026-10-10, not re-verified)*

- Software: Temurin JDK 25 and the `otp-shaded-2.10.0.jar` release from Maven Central.
- Inputs: the TransJakarta GTFS Static feed (§2) and an OpenStreetMap extract of DKI Jakarta
  from a third-party extract site (geo2day), under ODbL.
- Graph build and server start succeeded on a machine with about 8 GB RAM, using `-Xmx3G`.
  The build took well under a minute; peak memory was roughly 1.8 GB for the build and
  1.4 GB for serving.
- Sample journeys returned legs whose trips and stop times matched the same GTFS feed.
  **This only shows internal consistency with the feed, not real-world accuracy**; no
  official TransJakarta journey planner was used for comparison.
- Near-identical itineraries departing a minute apart were returned for the same request.
  The cause is not proven; the backend now merges structurally identical itineraries.
- An origin outside the DKI-only OSM extract returned `LOCATION_NOT_FOUND`.

## 2. Transit data

### TransJakarta GTFS Static

| Field | Value |
|-------|-------|
| Publisher | PT Transportasi Jakarta (Mobility Database lists it as an official feed) |
| Licence | **CC BY 4.0**, stated on the official PPID page *(recorded 2026-10-09/10)* |
| Licence source | https://ppid.transjakarta.co.id/informasi/berkala (row H): "GTFS Static Transjakarta © 2026 oleh PT Transportasi Jakarta dilisensikan di bawah CC BY 4.0." |
| Download | The PPID "Unduh" link redirects to `https://gtfs.transjakarta.co.id/files/file_gtfs.zip` |
| Catalogue | https://mobilitydatabase.org/feeds/gtfs/mdb-1909 (shows no licence field) |

Notes:

- An older PPID URL (`/pusat-data/data-terbuka/transjakarta-gtfs-feed`) redirected to the
  PPID home page when checked, and catalogues such as Mobility Database show no licence.
  The licence statement above is the authoritative source found so far.
- CC BY 4.0 allows reuse and redistribution with attribution. The backend sends the
  attribution text from `backend/internal/otp/attribution.go` with every successful trip plan.
  The final wording still needs owner approval.
- The licence applies to TransJakarta's static GTFS only. It says nothing about other operators.
- The feed itself must still not be committed; `.gitignore` excludes it.

Feed contents *(recorded 2026-10-10 from one downloaded version, not re-verified)*:

- MobilityData's GTFS validator 8.0.1 reported 0 errors and about 4,000 warnings, mostly
  missing `timepoint` values, plus two expired calendar entries.
- 240 routes, all GTFS `route_type` 3 (bus). Route descriptions include BRT, Mikrotrans,
  Transjabodetabek, Royaltrans and others, so Mikrotrans routes are in this feed.
- Service is almost entirely frequency-based (`frequencies.txt` with `exact_times=0`), so
  departure times are approximate.
- `transfers.txt` is very small, so interchange modelling relies mostly on OTP's own street
  transfers.
- Some stops lie outside DKI Jakarta (Bekasi, Tangerang, Bogor) and cannot be routed with a
  DKI-only OSM extract.

### Other Jakarta operators *(recorded 2026-10-09/10)*

The recorded research found no official, openly licensed GTFS feed and no open licence
for the data of MRT Jakarta, KRL Commuter Line (KAI Commuter), LRT Jakarta or LRT
Jabodebek. That does not prove no licence exists. LRT Jakarta and LRT Jabodebek have
schedule pages on their websites. The MRT Jakarta website could not be inspected
automatically because it is rendered with JavaScript, and the KAI Commuter website
returned HTTP 403 to automated access. The Mobility Database catalogue listed
TransJakarta as the only active Jakarta feed. **None of these operators may be used until
a licence or written permission is obtained.** Unofficial scraped datasets must not be used.

### Street network

OpenStreetMap data is used for walking legs under the ODbL, which requires attribution
("© OpenStreetMap contributors") and share-alike for derived databases. The extract used
so far covers DKI Jakarta only.

## 3. Decisions

| # | Decision | Status |
|---|----------|--------|
| D1 | Routing engine | OTP 2.10 adapter built and tested offline; locally feasible. Production hosting still open (§4). |
| D2 | TransJakarta GTFS licence | CC BY 4.0 per official PPID statement. Attribution wording needs owner approval. |
| D3 | Sources for non-TransJakarta modes | Open: no licensed feeds found; permission requests needed. |
| D4 | PostgreSQL/PostGIS for MVP | Open, not blocking. |
| D5 | App-facing trip-planning API contract | `POST /v1/trip-plans` implemented ([OpenAPI](api/openapi.yaml)). Open items: walk-only results and more specific error codes. |

## 4. Open production and data-quality questions

- Hosting: memory and CPU for OTP in production, and where it runs.
- Data refresh: how and how often the feed and OSM extract are re-downloaded, the graph
  rebuilt and deployed atomically, and how the API learns the data version.
- Coverage: an OSM extract covering Jabodetabek so stops outside DKI can be routed.
- Accuracy: comparing itineraries with official TransJakarta information; timetable
  precision of frequency-based service; quality of transfers.
- Configuration: OTP's feed id must match the adapter's expectation (`1` today).
- Product: whether and when to show walk-only journeys; stop search and geocoding.
