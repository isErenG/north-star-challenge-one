# Real-world verification pipeline

Premise: the uploaded KBO export is out of date. Every field, including the
registration status, is a claim. External sources supply evidence. Output per
row is a verdict on whether the business still exists as registered plus a
suggestion per field where the evidence disagrees with the registry. Nothing is
applied without a reviewer accepting it.

## Data model additions (`internal/model`)

```go
type Evidence struct {
    Source string `json:"source"`          // google | website | openai | databe | goldenpages | trendstop | vkbo
    URL    string `json:"url,omitempty"`
    Note   string `json:"note,omitempty"`  // short human-readable justification
}

type Suggestion struct {
    Field      string     `json:"field"`       // name | address | phone | email | website | activity | status
    Current    string     `json:"current"`     // value in the record when the suggestion was made
    Value      string     `json:"value"`       // proposed value
    Kind       string     `json:"kind"`        // confirmed | different | new
    Confidence float64    `json:"confidence"`  // 0..1
    Evidence   []Evidence `json:"evidence"`
    Accepted   bool       `json:"accepted,omitempty"`
}

type Verification struct {
    Verdict   string   `json:"verdict"`   // likely_active | likely_ceased | unclear | skipped | not_run
    Reason    string   `json:"reason"`    // one sentence for the reviewer
    Sources   []string `json:"sources"`   // sources that actually ran
    Skipped   string   `json:"skipped,omitempty"` // budget | not_configured | non_commercial | ceased_early
    Cost      float64  `json:"cost"`      // estimated EUR spent on this row
    CheckedAt string   `json:"checkedAt"`
}

// Record gains:
Verification *Verification `json:"verification,omitempty"`
Suggestions  []Suggestion  `json:"suggestions,omitempty"`
// Google *Place stays for backward compatibility; the google source fills it.

// Job gains:
Enrichment *Enrichment `json:"enrichment,omitempty"`

type Enrichment struct {
    Sources     []string `json:"sources"`      // requested, in run order
    BudgetEUR   float64  `json:"budgetEur"`
    SpentEUR    float64  `json:"spentEur"`
    Verified    int      `json:"verified"`     // rows with a verdict
    Skipped     int      `json:"skipped"`      // rows skipped (budget, non-commercial)
    Concurrency int      `json:"concurrency"`
}
```

`Job.Enrich bool` stays and means "enrichment requested".

## HTTP API changes (`internal/httpapi`)

`GET /api/config` →

```json
{
  "google": true, "email": false, "mapbox": "pk...",
  "sources": {
    "google": true, "website": true, "openai": false,
    "databe": false, "goldenpages": true, "trendstop": true, "vkbo": true
  },
  "defaultBudgetEur": 5
}
```

`POST /api/jobs` multipart fields: `file`, `email`, `enrich` (true|false),
`budget` (EUR, float, default from config), `sources` (comma list; default
`google,website,openai`). Unknown or not-ready sources are dropped with a
job-level note, never an error.

`PATCH /api/jobs/{id}` body is exactly one of:

- `{"email": "..."}` unchanged
- `{"record": {...}}` unchanged
- `{"accept": {"id": "12", "field": "phone"}}` → copies the suggestion value
  into the record field, marks the suggestion accepted, appends
  `"<field> accepted from <source> on <date>"` to notes, revalidates.
- `{"verify": {"id": "12"}}` → re-runs the pipeline for one record even if it
  was skipped; respects the job budget. Returns 202 and the job; the row's
  verification shows `verdict: "not_run"` with `reason: "queued"` until done.

Errors keep the existing shape `{"error": "..."}`.

## Enrichment package (`internal/enrich`)

```go
// Row is the working state for one record during a pipeline run.
type Row struct {
    Record   model.Record
    Signals  []Signal    // existence evidence
    Findings []Finding   // field-level evidence
    Pages    []Page      // fetched text for the LLM
    Place    *model.Place
    Cost     float64
    Sources  []string
}

type Signal struct {
    Source    string
    Existence string // alive | closed | unknown
    Weight    float64 // 0..1
    Note, URL string
}

type Finding struct {
    Source, Field, Value, URL, Note string
    Confidence float64
}

type Page struct{ URL, Title, Text string } // text truncated to 8 kB

// Source is one external provider. Run appends to the Row; it never mutates
// the Record.
type Source interface {
    Name() string
    Ready() bool
    CostEUR() float64                        // per call, for the budget
    Run(ctx context.Context, row *Row) error // errors become a Signal note, never fatal
}

// Judge is the LLM step. Both methods are skipped when not Ready.
type Judge interface {
    Ready() bool
    CostEUR() float64
    Verdict(ctx context.Context, row *Row) (Signal, error)          // for unclear rows
    Extract(ctx context.Context, row *Row, want []string) ([]Finding, error) // gaps + activity
}

type Options struct {
    Sources     []string
    BudgetEUR   float64
    Concurrency int // default 4
}

type Pipeline struct { /* sources, judge, budget, cache, limiters */ }

func New(cfg config.Config, cache Cache) *Pipeline
func (p *Pipeline) Available() map[string]bool
func (p *Pipeline) Run(ctx context.Context, rec model.Record, opts Options, budget *Budget) Result

type Result struct {
    Verification model.Verification
    Suggestions  []model.Suggestion
    Place        *model.Place
}
```

Run order inside `Run`:

1. Skip rules: legal form "Vereniging van Mede-eigenaars" → `skipped`
   (`non_commercial`). No name → `skipped`.
2. Existence sources: `google`, `website`, then directories if requested.
   Each is rate-limited, cached by `enterpriseNumber|source`, and metered.
3. `verdict(row)` by rules:
   - any signal `closed` with weight ≥ 0.8 → `likely_ceased`
   - sum of `alive` weights ≥ 1.0 → `likely_active`
   - otherwise `unclear`; if judge ready → `judge.Verdict`, else stays unclear.
   `likely_ceased` stops here unless `opts.Force` (used by `verify`).
4. Gap detection: fields wanted = phone, email, website, address, activity.
   Missing or conflicting → `judge.Extract` over `row.Pages`.
5. `reconcile(row)`: per field group findings, pick highest confidence value
   with support count; produce `confirmed` when equal to the record value
   (after normalisation: digits for phone, lower-case for email/website,
   whitespace-collapsed for address), `different` when not, `new` when the
   record is empty.

### Sources

- `website` (no key). Candidates: record website, Google website. Fetches `/`,
  `/contact`, `/contact-us`, `/over-ons`, `/about`, `/impressum` with a 6 s
  timeout each, 512 kB cap, custom User-Agent. Extracts `mailto:`, emails,
  Belgian phone patterns, `BE 0xxx.xxx.xxx` VAT numbers, and postal address
  lines. Finding the record's enterprise number on the site gives an `alive`
  signal with weight 0.9. Parking pages ("domain is for sale", registrar
  templates), DNS failure or 4xx/5xx give `closed` 0.4 (weak: sites die
  while businesses live). Closure phrases ("permanently closed", "definitief
  gesloten", "stopgezet", "heeft zijn deuren gesloten") give `closed` 0.7.
- `google` (key). Text search, field mask adds `location`, `types`,
  `businessStatus`, `currentOpeningHours`. `OPERATIONAL` → `alive` 0.7,
  `CLOSED_PERMANENTLY` → `closed` 0.9, `CLOSED_TEMPORARILY` → `alive` 0.3 with
  note. Not found → `unknown`. Phone, website, address become findings 0.7.
  Name similarity below 0.5 (token Jaccard) lowers all weights to 0.3 with a
  "name mismatch" note. Fills `Record.Google` for the existing UI.
- `databe` (token, `DATABE_TOKEN`). Official API. Contact fields → findings
  0.8, "active" flag → `alive` 0.6 (it mirrors KBO, so low weight).
- `goldenpages`, `trendstop` (no key, off by default, 1 req/s, best effort).
  HTML search by name + municipality; phone/website findings 0.5; presence
  → `alive` 0.3. Any block or layout change → skipped with note.
- `vkbo` (no key, off by default). Live registry re-query; produces only a
  note "registry changed since export" as a Finding on `status`. Never a
  Signal.

### Judge (`internal/enrich/openai.go`)

Chat Completions with `response_format: json_schema`, `OPENAI_MODEL`
(default `gpt-4o-mini`, override in `.env`), temperature 0, 1 retry. Input is
the record baseline, signals, findings and page texts (max 24 kB total). Cost
estimate `OPENAI_COST_EUR` per call (default 0.002).

### Budget, cache, limits

- `Budget{Limit, Spent float64; mu}`: `Reserve(cost) bool`. A row that cannot
  reserve its next call stops and is marked `skipped: budget`.
- `Cache`: `DATA_DIR/cache/<source>/<sha256(key)>.json`, TTL 30 days.
- Limiters: `golang.org/x/time/rate`, google 10/s, website 4/s (per host
  1/s), directories 1/s, openai 5/s.
- Job concurrency: 4 workers per job.

## Config

| Variable | Purpose |
|---|---|
| `OPENAI_API_KEY`, `OPENAI_MODEL`, `OPENAI_COST_EUR` | judge |
| `GOOGLE_MAPS_API_KEY`, `GOOGLE_COST_EUR` (default 0.03) | places |
| `DATABE_TOKEN` | data.be API |
| `ENRICH_DIRECTORIES=goldenpages,trendstop` | opt-in scraping |
| `ENRICH_DEFAULT_BUDGET_EUR` (default 5) | upload default |

## Frontend

- Upload: "Verify against real-world sources" checkbox replaces the Google
  checkbox; expands to source toggles (only ready sources enabled) and a budget
  field. Processing screen shows spent / budget.
- List: fourth badge "Likely ceased"; filter tab "Likely ceased"; map colour
  `#8a4a3a`.
- Drawer: verdict banner, then suggestions table (field, current, suggested,
  confidence bar, source link, Accept). "Verify this business" button
  triggers `verify`.
- Exports: `verification` and `suggestions` ride along in JSON/GeoJSON; CSV
  adds `review_verdict`, `review_verdict_reason`, and one
  `review_suggested_<field>` column per field.

## Tests

- Go: website source against `httptest` pages (contact page, parking page,
  closure notice, VAT number match); google against fake transport; judge
  with fake transport returning schema JSON; reconcile table tests; budget
  and cache; pipeline end-to-end with fake sources; httpapi accept/verify.
- Playwright: upload with enrichment off stays green; a fixture job JSON with
  suggestions is loaded through the store to exercise the drawer and accept.

## Build order

1. model + enrich core (row, budget, cache, limiter, reconcile, verdict rules)
2. website source
3. google source (replaces `internal/google`)
4. openai judge
5. jobs integration (worker pool, budget, accept, verify) + httpapi
6. frontend
7. databe, goldenpages, trendstop, vkbo
8. exports + docs
