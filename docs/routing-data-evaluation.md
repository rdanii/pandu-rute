# Routing engine & transit data: initial evaluation

Status: **research notes. Nothing here is a final decision.**
Last reviewed: 2026-10-09.

## 1. Routing engine

### Candidate: OpenTripPlanner (OTP)

OTP is a candidate, **not** a final choice.

Why it is on the list:
- It is an open-source multimodal trip planner that reads GTFS (transit) and OpenStreetMap (walking legs and the street network).
- It would avoid writing our own transit routing algorithm.

Unverified and still to evaluate:
- [ ] Which OTP major version to target, and its current API style.
- [ ] How well it handles Jakarta-specific patterns: BRT corridors with in-station transfers, feeder routes, and informal or unscheduled services (angkot, Mikrotrans) that may lack timetables.
- [ ] Memory and CPU cost of building and serving a Jabodetabek-scale graph, and the hosting cost.
- [ ] Operations: graph rebuild time, and how feed updates get deployed.
- [ ] Licence of OTP itself and whether it suits how we plan to distribute the app.

Alternatives not yet assessed: other open-source GTFS routers, or a custom
RAPTOR-style implementation in Go. They should be compared against the same
criteria before deciding.

## 2. Transit data

### TransJakarta GTFS

| Field | Value | Verified? |
|-------|-------|-----------|
| Publisher | Transjakarta | Listed as an "Official Feed" on Mobility Database |
| Feed URL (producer) | `https://gtfs.transjakarta.co.id/files/file_gtfs.zip` | Found via Mobility Database; the URL was not downloaded |
| Catalogue entry | https://mobilitydatabase.org/feeds/gtfs/mdb-1909 | Viewed 2026-10-09 |
| Coverage per catalogue | 240 bus routes; service dates 2004-01-15 to 2027-12-31 | From the catalogue page; feed contents not inspected |
| Licence | **CC BY 4.0, as stated in the project brief** | ⚠️ **Not verified.** See below |

#### ⚠️ Licence status

The project brief says the feed is licensed **CC BY 4.0**. On 2026-10-09 I could **not** find
that licence published anywhere:

- The Mobility Database entry shows no licence.
- `https://gtfs.transjakarta.co.id` shows only a login page ("Upload File GTFS"), with no licence or terms.
- An OpenStreetMap community thread
  (https://community.openstreetmap.org/t/transjakarta-gtfs-data/143620) reports that the ZIP
  contains no licence file and that Transjakarta's website has a copyright notice. Community
  members were contacting Transjakarta for permission.

**Until the licence is confirmed in writing by Transjakarta, or found in an official
publication, treat the feed's terms as unknown.** Don't redistribute it, commit it to this repo, or ship it in a public
release. If CC BY 4.0 is confirmed, record where it came from here, along with the
attribution wording it requires.

#### Coverage of other Jakarta modes: unverified

The TransJakarta feed is assumed to cover TransJakarta services only. Whether
it includes any of the following has **not** been checked. Each needs its own
source and licence:

- [ ] TransJakarta feeder and Mikrotrans/JakLingko microbus routes. Check whether they are in this feed.
- [ ] MRT Jakarta
- [ ] LRT Jakarta
- [ ] LRT Jabodebek
- [ ] KRL Commuter Line (KAI Commuter)
- [ ] Soekarno–Hatta Airport rail link
- [ ] Angkot and other informal services. There may be no GTFS at all.

#### Street network

OpenStreetMap is the expected source for walking legs, under the ODbL licence, which brings
attribution and share-alike obligations. Not yet evaluated.

## 3. Open decisions

| # | Decision | Blocking? | Next action |
|---|----------|-----------|-------------|
| D1 | Routing engine (OTP or an alternative) | Blocks trip planning | Build a spike: load the TransJakarta feed into OTP locally and measure graph size and query quality |
| D2 | TransJakarta GTFS licence | Blocks any redistribution or public release | Get written confirmation from Transjakarta |
| D3 | Sources for non-TransJakarta modes | Blocks multimodal routing | Survey the operators listed in §2 |
| D4 | Whether PostgreSQL/PostGIS is needed for MVP | Not blocking | Revisit after D1 |
| D5 | App-facing trip-planning API contract | Blocks mobile integration | Design after D1 |
