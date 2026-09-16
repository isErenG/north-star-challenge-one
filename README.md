# KBO Review

A three-screen workspace for organising Belgian KBO business exports: upload → processing → review and export. Built with Svelte 5, SvelteKit, Tailwind CSS, daisyUI and a Go service.

## Run locally

Requires Node 22.12+ (Node 24 recommended) and Go 1.24+. If Go is not installed, `npm run setup:go` downloads the current official toolchain into the gitignored `.context/toolchain` directory and checks its SHA-256 hash.

```sh
npm install
npm run setup:go # only when Go is not already installed
npm run dev
```

Open the URL printed by Vite (normally http://127.0.0.1:5173; it selects another port if occupied). `npm run dev` starts both SvelteKit and the Go API. Both bind to loopback. Optional configuration is loaded from `.env`; copy `.env.example` to get started. Do not commit credentials.

## Supported files

- CSV: UTF-8 (with or without BOM), comma, semicolon or tab separated; proper quoted fields and multiline values. Maximum 10 MB and 10,000 records.
- JSON: an array of objects, or an object containing `records` or `data`.
- GeoJSON: a FeatureCollection with business information in each feature's `properties`. Geometries are preserved; missing geometry stays null rather than being invented.

The initial mapping uses the **header and first three records only** of the supplied Schoten CSV. The full attachment was not read or bundled. `static/example.csv` is synthetic.

The importer recognises `Ondernemingsnr`, `Maatschappelijke_naam`, `Commerciele_naam`, `Ondernemingsnr_maatsch_zetel`, `Rechtstoestand`, KBO address columns, `longitude` and `latitude`, plus the documented English equivalents in `server/main.go`. It keeps establishment identifiers and parent enterprise numbers distinct. CSV identifiers remain strings so leading zeroes survive. Unrecognised source columns remain attached to every record.

Checks cover the Belgian modulo-97 identifier checksum, duplicate identifiers, missing business names/addresses/coordinates, email syntax and selected source-status terms. These are data-quality checks, **not live KBO registration verification**. Duplicate records are flagged and retained, not silently merged. Phone and email completeness has its own filter; absence is never filled with fabricated data.

The review drawer edits names, addresses, phone, email, website and notes. Source registration status and original source fields stay intact. Marking a row reviewed records human review without erasing its issues. JSON is the primary export; CSV adds prefixed review columns, and GeoJSON preserves geometry with reviewed properties. CSV formula-like values are escaped for spreadsheet safety; use JSON for exact textual fidelity. JSON and GeoJSON exports can be reimported while retaining source fields and review changes. Import enterprise identifiers as **text** when opening CSV in Excel.

## Optional connections

**Google Places (New):** set `GOOGLE_MAPS_API_KEY` with Places API access. The checkbox is disabled until configured. Each opted-in row with a name and address makes one Text Search call (billable); a maximum of two files process concurrently. The top candidate is explicitly unconfirmed, and names/addresses/status/phone/websites are never silently applied. Per-record API errors are visible. External closure status never replaces official registration status. Google provides no email-address field. Live integration has not been tested with credentials. Confirm the applicable Google Maps licensing, attribution, retention and export terms before enabling it for an operational dataset; this local prototype currently persists candidate responses with the job.

**Completion email:** set `RESEND_API_KEY`, `NOTIFY_FROM` (a verified sender) and optionally `PUBLIC_APP_URL` (HTTPS). Users can request one notice during upload/processing or after completion. Jobs use a stable provider idempotency key. Provider failures are surfaced rather than reported as sent. Notifications are not enabled by default; no email is sent during tests. A notice without `PUBLIC_APP_URL` tells the recipient to return to their existing tab. Provider acceptance is not proof of inbox delivery.

## Storage and deployment boundary

The Go service writes jobs atomically to `server/data/` with private file permissions (or `DATA_DIR`). Refreshing a task URL reloads saved results. An interrupted processing job is marked failed on restart, not silently resumed; re-upload to retry. Completion messages interrupted during delivery are marked `delivery unknown` rather than sent again automatically.

This is a local working application, not an authenticated government service. A 48-character random task URL grants access to its job. Treat it as a private link. Before any public/multi-user deployment, add identity/access controls, per-tenant ownership, retention/deletion policy, quotas, and a durable queue with notification retry reconciliation. There is no automatic expiry yet. Job data is not tracked in Git. No live KBO requests are made.

For a production build, run `npm run build` and build the Go service with `cd server && go build -o kbo-review .`. Run both `node build` and the Go executable behind the same private deployment; the Go process binds only to 127.0.0.1. Set SvelteKit's `ORIGIN` to the web origin and configure environment variables in both processes. `npm run preview` only previews the web build; the Go service must also be running.

## Validation

```sh
npm run check
npm run test           # Go parsing, checks, persistence; includes race detection
npm run build
TEST_URL=http://127.0.0.1:5173 npx playwright test # with npm run dev active
```

Browser tests use synthetic inputs to cover CSV upload, JSON/GeoJSON drag-and-drop, malformed files, mobile overflow, record edits, refresh persistence, search, all export formats and CSV formula escaping. Screenshots are written to `.context/`.

## Source references

- [FPS Economy: CBE data available for reuse](https://economie.fgov.be/en/themes/enterprises/crossroads-bank-enterprises/services-everyone/public-data-available-reuse/cbe-file-containing-all-public)
- [FPS Economy: Public Search](https://economie.fgov.be/en/themes/enterprises/crossroads-bank-enterprises/services-everyone/consultation-and-research-data/cbe-public-search)
- [Google Places data fields](https://developers.google.com/maps/documentation/places/web-service/data-fields)
- [daisyUI with SvelteKit](https://daisyui.com/docs/install/sveltekit/)
