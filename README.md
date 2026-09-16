# KBO Review

A three-screen workspace for organising Belgian KBO business exports: upload → processing → review and export. Built with Svelte 5, SvelteKit, Tailwind CSS, daisyUI and a Go service using Gin. In production the Go binary serves both the API and the prebuilt frontend from a single container.

## Run locally

Requires Node 22.12+ (Node 24 recommended) and Go 1.25+. If Go is not installed, `npm run setup:go` downloads the current official toolchain into the gitignored `.context/toolchain` directory and checks its SHA-256 hash.

```sh
npm install
npm run setup:go # only when Go is not already installed
npm run dev
```

Open the URL printed by Vite (normally http://127.0.0.1:5173; it selects another port if occupied). `npm run dev` starts the Vite dev server and the Go API; Vite proxies `/api` to the Go process. Both bind to loopback. Optional configuration is loaded from `.env`; copy `.env.example` to get started. Do not commit credentials.

## Project layout

```
cmd/kbo-review/      main.go — calls app.Run()
internal/app         wiring: config → store → services → HTTP server; `-check` health probe
internal/config      environment variables → Config
internal/model       Record, Place, Job (JSON tags are the on-disk and wire format)
internal/parse       CSV / JSON / GeoJSON → records, source columns preserved
internal/validate    identifier checksum, duplicates, missing fields, email syntax, status terms
internal/store       Store interface; atomic JSON-file implementation; in-memory test double
internal/google      Places Text Search client (unconfirmed candidates only)
internal/notify      Resend completion email with idempotency key
internal/jobs        job registry, locking, processing pipeline, notification state machine
internal/httpapi     Gin routes, same-origin and body-size middleware, security headers, static site
web/                 embed.go embeds web/dist, the SvelteKit static build (gitignored)
src/                 SvelteKit frontend
```

Dependencies point inward: `httpapi → jobs → {store, parse, validate, google, notify} → model`. Only `httpapi` knows about HTTP.

## Supported files

- CSV: UTF-8 (with or without BOM), comma, semicolon or tab separated; proper quoted fields and multiline values. Maximum 10 MB and 10,000 records.
- JSON: an array of objects, or an object containing `records` or `data`.
- GeoJSON: a FeatureCollection with business information in each feature's `properties`. Geometries are preserved; missing geometry stays null rather than being invented.

The initial mapping uses the **header and first three records only** of the supplied Schoten CSV. The full attachment was not read or bundled. `static/example.csv` is synthetic.

The importer recognises `Ondernemingsnr`, `Maatschappelijke_naam`, `Commerciele_naam`, `Ondernemingsnr_maatsch_zetel`, `Rechtstoestand`, KBO address columns, `longitude` and `latitude`, plus the documented English equivalents in `internal/parse/parse.go`. It keeps establishment identifiers and parent enterprise numbers distinct. CSV identifiers remain strings so leading zeroes survive. Unrecognised source columns remain attached to every record.

Checks cover the Belgian modulo-97 identifier checksum, duplicate identifiers, missing business names/addresses/coordinates, email syntax and selected source-status terms. These are data-quality checks, **not live KBO registration verification**. Duplicate records are flagged and retained, not silently merged. Phone and email completeness has its own filter; absence is never filled with fabricated data.

The review drawer edits names, addresses, phone, email, website and notes. Source registration status and original source fields stay intact. Marking a row reviewed records human review without erasing its issues. JSON is the primary export; CSV adds prefixed review columns, and GeoJSON preserves geometry with reviewed properties. CSV formula-like values are escaped for spreadsheet safety; use JSON for exact textual fidelity. JSON and GeoJSON exports can be reimported while retaining source fields and review changes. Import enterprise identifiers as **text** when opening CSV in Excel.

## Optional connections

**Google Places (New):** set `GOOGLE_MAPS_API_KEY` with Places API access. The checkbox is disabled until configured. Each opted-in row with a name and address makes one Text Search call (billable); a maximum of two files process concurrently. The top candidate is explicitly unconfirmed, and names/addresses/status/phone/websites are never silently applied. Per-record API errors are visible. External closure status never replaces official registration status. Google provides no email-address field. Live integration has not been tested with credentials. Confirm the applicable Google Maps licensing, attribution, retention and export terms before enabling it for an operational dataset; this local prototype currently persists candidate responses with the job.

**Map view (Mapbox):** set `MAPBOX_ACCESS_TOKEN` to a Mapbox _public_ token (`pk.…`) with Styles and Tiles scopes. The results screen then offers a List / Map toggle; the Map button stays disabled until a token is configured. The map plots every filtered record that has coordinates as a circle coloured by review state, shows a hover popup, and opens the review drawer on click. Records without coordinates are counted above the map and remain in the list; no position is ever invented. Only public tokens are exposed to the browser; the server ignores secret (`sk.`) tokens. Restrict the token by URL in your Mapbox account. Opening the map loads Mapbox GL JS on demand and fetches styles and vector tiles from `api.mapbox.com`, which is an outbound browser connection subject to Mapbox's billing and terms. Map attribution must remain visible.

**Completion email:** set `RESEND_API_KEY`, `NOTIFY_FROM` (a verified sender) and optionally `PUBLIC_APP_URL` (HTTPS). Users can request one notice during upload/processing or after completion. Jobs use a stable provider idempotency key. Provider failures are surfaced rather than reported as sent. Notifications are not enabled by default; no email is sent during tests. A notice without `PUBLIC_APP_URL` tells the recipient to return to their existing tab. Provider acceptance is not proof of inbox delivery.

## Storage and deployment boundary

The Go service writes jobs atomically to `data/` with private file permissions (or `DATA_DIR`). Refreshing a task URL reloads saved results. An interrupted processing job is marked failed on restart, not silently resumed; re-upload to retry. Completion messages interrupted during delivery are marked `delivery unknown` rather than sent again automatically.

This is a local working application, not an authenticated government service. A 48-character random task URL grants access to its job. Treat it as a private link. Before any public/multi-user deployment, add identity/access controls, per-tenant ownership, retention/deletion policy, quotas, and a durable queue with notification retry reconciliation. There is no automatic expiry yet. Job data is not tracked in Git. No live KBO requests are made.

For a production build, run `npm run build:all`. This writes the static frontend to `web/dist` and compiles `bin/kbo-review` with that frontend embedded. The single binary serves the site and the API on `ADDR` (default `127.0.0.1:8787`). It sends a Content Security Policy that allows only its own assets, Google Fonts and Mapbox; extend it in `internal/httpapi/middleware.go` if you add another external service. Precompressed `.br`/`.gz` assets are served when the browser accepts them, and hashed `_app/immutable` files get a one-year cache header.

## Docker

```sh
docker compose up --build
```

The image is a three-stage build: Node builds the frontend, Go compiles a static binary with it embedded, and a distroless non-root image runs it (about 25 MB). Compose publishes port 8787 on `127.0.0.1` only, loads `.env` if present, and stores job data in the `kbo-data` named volume mounted at `/data`. The container's healthcheck runs `kbo-review -check`, which probes `/healthz`. Job data survives container restarts; remove the volume to wipe it.

## Validation

```sh
npm run check
npm run test           # go test -race ./... : parsing, checks, persistence, jobs, HTTP
npm run build:all
TEST_URL=http://127.0.0.1:5173 npx playwright test # with npm run dev active
TEST_URL=http://127.0.0.1:8787 npx playwright test # against bin/kbo-review or the container
```

Browser tests use synthetic inputs to cover CSV upload, JSON/GeoJSON drag-and-drop, malformed files, mobile overflow, record edits, refresh persistence, search, all export formats, CSV formula escaping and the map view toggle (the map itself renders only when `MAPBOX_ACCESS_TOKEN` is set; otherwise the test asserts the disabled state). Screenshots are written to `.context/`.

## Source references

- [FPS Economy: CBE data available for reuse](https://economie.fgov.be/en/themes/enterprises/crossroads-bank-enterprises/services-everyone/public-data-available-reuse/cbe-file-containing-all-public)
- [FPS Economy: Public Search](https://economie.fgov.be/en/themes/enterprises/crossroads-bank-enterprises/services-everyone/consultation-and-research-data/cbe-public-search)
- [Google Places data fields](https://developers.google.com/maps/documentation/places/web-service/data-fields)
- [daisyUI with SvelteKit](https://daisyui.com/docs/install/sveltekit/)
