Build a take-home demo: an AI accounts-payable agent for cross-border supplier
invoices. Read the whole brief before writing code, then build in the phases
listed at the end. Once you finish each phase, ask me for review and I'll make
the commit. Once we commit, I'll tell you so we can continue. At the end, create
a `docs/` folder with PlantUML diagrams: components and connectors, sequences,
deployment, and a class diagram for the database (details in the "Engineering
patterns" section).

## Context

This is a technical challenge for a Forward Deployed Engineer role at a
cross-border B2B payments and spend-management company whose customers are LatAm
businesses paying international suppliers. I have two days. The demo must be
functional, easy to explain line by line, and impressive without being complex.
It is NOT a production system.

This brief is self-contained. Start from an empty directory and build everything
from this file alone. Do not look for, clone or depend on any other repository,
template or earlier project. Every pattern I expect (migrations runner, seed,
transaction helper, background jobs, table geocoder, payment mock, testing and
mocks, docs) is specified in the "Engineering patterns" section below. Where
something is not specified, choose the simplest option I could explain in an
interview and note the choice in the README.

## The product

A finance team uploads a supplier invoice (PDF). The agent:

1. Extracts structured data from it with Claude (any layout, English or
   Spanish). 2. Does a 3-way match against the purchase order and the goods
   receipts, with plain deterministic code. 3. Flags discrepancies with a clear
   reason per line. 4. Clean invoices are approved automatically. Anything flagged
   waits for a human to approve, reject or override with a reason. 5. Approved
   invoices get a payment scheduled: pick the date from the payment terms
   (early-payment discount vs due date), show an FX quote, and execute the payment
   through a mocked payment provider when the date arrives.

Principle to preserve everywhere: the LLM only reads documents (and optionally
drafts text). Every decision that moves money is deterministic Go code that I
can explain.

## Hard rules for the code

- Do NOT write comments unless strictly necessary to explain a non-obvious
  "why". No doc comments, no section banners, no commented-out code, no comments
  in SQL files, no comments restating what the code does. - I must be able to
  explain every line in a live interview. Plain functions, small files, no
  frameworks beyond the ones listed, no ORM, no DI container, no clever generics,
  no reflection tricks. - Raw SQL with pgx. Every locking, transaction and
  conditional-update statement must be visible in the code. - Put explanations in
  README.md / DESIGN.md, not in code. - Idiomatic Go: gofmt, errors wrapped with
  `%w`, `context.Context` first, small interfaces defined where they are consumed.
  Unit tests follow the "Testing and mocks" section: table-driven tests for the
  pure packages (`matching`, the payment scheduling rules, `fx`, `money`) and
  mockery-based tests for the orchestration code. Nothing else needs automated
  tests. - Money never touches a float. No `float32` or `float64` anywhere in
  code that holds, parses, converts, compares or prints an amount, a price, a rate
  or a percentage. Amounts are `int64` minor units (cents), rates are scaled
  integers, percentages are basis points. Details in "Money". The only floats
  allowed in the project are geographic coordinates (latitude, longitude) and
  distances in km. - `golangci-lint run` must finish with zero issues, and `go
vet ./...` and `go build ./...` must be clean, at the end of every phase. Fix
  the code instead of adding `//nolint`; a `//nolint` is only acceptable with a
  one-line justification and I will ask about each one.

## Stack

- Toolchain (my machine, develop against these): Go 1.27.3 (`go.mod` says `go
1.27`; module name `ap-agent` unless I say otherwise) and golangci-lint 2.15.0
  (v2 configuration format). If the sandbox you run in has an older Go, let
  `GOTOOLCHAIN=auto` fetch 1.27.x; if it cannot, tell me and do not downgrade
  silently. Do not rely on any library or language feature you have not confirmed
  exists in this Go version. - Backend: Go, `github.com/gin-gonic/gin` as the
  HTTP server and router (the payment mock uses Gin too), `log/slog` (JSON),
  `pgx/v5` with `pgxpool`, `embed` for SQL migrations. Run Gin with
  `gin.SetMode(gin.ReleaseMode)` and `gin.New()` (not `gin.Default()`), so logging
  and recovery are my own small middleware. Use `http.Server{Handler: router}` so
  graceful `Shutdown(ctx)` works. Request bodies are decoded with
  `c.ShouldBindJSON` into plain structs; validation rules that are business rules
  live in Go code, not in struct-tag magic (simple `binding:"required"` tags are
  fine). Multipart upload via `c.FormFile` with `router.MaxMultipartMemory` and a
  body size cap (`EXTRACT_MAX_FILE_MB`, default 10). - Database: PostgreSQL via
  the `postgis/postgis` image. Keep PostGIS: `geography(Point,4326)`, GiST index,
  `ST_Distance`. Never scan geography columns into Go; insert with
  `ST_SetSRID(ST_MakePoint($lng,$lat),4326)::geography` and read back with
  `ST_X(location::geometry)` / `ST_Y(...)` if needed. - LLM: Anthropic API.
  Prefer the official Go SDK (`github.com/anthropics/anthropic-sdk-go`); if it
  gets awkward, call the Messages API with plain `net/http` + `encoding/json`.
  PDFs are sent as base64 `document` content blocks. Structured output via a
  forced tool call with a JSON schema. Model name from env `ANTHROPIC_MODEL`
  (default `claude-sonnet-5-5`). - Frontend: Vue 3 + TypeScript (strict) + Vite +
  vue-router. No UI framework, no Pinia: a small typed `api.ts` wrapper over
  `fetch`, plain CSS, light theme. Three views only (see UI). - Money (package
  `internal/money`, pure, table-tested):
  - Every amount is an `int64` in minor units (cents) plus a 3-letter currency
    code. Every currency is treated as 2 decimals (say so in the README as a
    simplification: no JPY or KWD handling). - Percentages and spreads are
    integer basis points (1% = 100 bps). Tolerances and discounts are configured
    and stored as bps. - FX rates are stored as `int64` scaled by 1e8 (`rate_e8`,
    units of quote currency per 1 unit of base currency). Both currencies have 2
    decimals, so minor units convert with the same scale. - Where a product can
    exceed `int64` (the FX quote: amount x rate_e8 x (10000 + spread_bps)),
    compute the intermediate with `math/big.Int`, round half up once at the end,
    and return an error if the result does not fit `int64`. Everywhere else use
    plain `int64` arithmetic with half-up rounding helpers, with an overflow check
    where inputs are user-controlled. - Parse and format with strings only:
    `ParseMinor("1,250.00")` and `FormatMinor(125000)`. `ParseMinor` accepts an
    optional currency symbol, thousands separators and exactly 0 to 2 decimals,
    and rejects anything else (including 3 or more decimals, which become a clear
    extraction error and a README limitation). JSON between the API and the web
    carries amounts as integers in minor units; the web formats them for display
    with `Intl.NumberFormat` on `amount / 100`, never storing the result. - SQL
    columns for money are `bigint`. No `numeric` or `double precision` for money,
    rates or percentages.
- Config only via environment variables, validated at startup. `GET /health`.
  Graceful shutdown (signal -> stop accepting -> finish in-flight -> stop jobs).

## Docker: layered strategy so anyone who clones the repo can run it

- One `Dockerfile` for the Go services, multi-stage: (1) `deps` stage copies
  only `go.mod`/`go.sum` and runs `go mod download` so dependency layers cache;
  (2) `build` stage copies source and builds all binaries with `CGO_ENABLED=0
-trimpath -ldflags="-s -w"`; the Go builder image is `golang:1.27` (pin to
  `1.27.3` if that tag exists, and make the version a build `ARG` shared by both
  stages); (3) one tiny runtime stage per binary (`api`, `migrate`, `seed`,
  `payment-mock`) based on `gcr.io/distroless/static` or `alpine` (alpine only if
  a healthcheck needs a shell/wget). Select with `target:` in compose. -
  `web/Dockerfile`: `deps` (`npm ci` on package files only) -> `build` (`vite
build`) -> `nginx:alpine` serving the static bundle and proxying `/api/` to the
  api service. - `docker-compose.yml`: `postgres` (healthcheck) -> `migrate` ->
  `seed` -> `api`; `payment-mock` depends only on postgres; `web` depends on api
  healthy. Every long-running service has a healthcheck, and `docker compose up
--build -d --wait` must return when all are healthy. `postgis/postgis` is amd64
  only: set `platform: linux/amd64` and note it in the README for Apple Silicon.
- `.env.example` documents every variable. The app must work with NO API key:
  see extraction below. `ANTHROPIC_API_KEY` is optional and read from the host env
  or `.env`. - Ports: web 8080, api 3000, payment-mock 4000, postgres 5432. - A
  `Makefile` with: `up`, `down` (with `-v`), `logs`, `demo`, `dev-api`, `dev-web`,
  `fmt` (`golangci-lint fmt`), `lint` (`golangci-lint run`), `test` (`go test
-race ./...`), `mocks` (regenerates the mockery mocks). Nothing else. -
  `.golangci.yml` in the repo root using the v2 format (`version: "2"`). Start
  from the default linters and add: `bodyclose`, `contextcheck`, `errorlint`,
  `gocritic`, `gosec`, `misspell`, `nilerr`, `noctx`, `rowserrcheck`,
  `sqlclosecheck`, `unconvert`, `unparam`. Formatters: `gofmt` and `goimports`.
  Keep the config short and without per-file exclusions; if a linter produces
  noise that is not worth fixing, say so and I will decide. Linting runs on my
  machine, not inside Docker. Mockery output carries the standard "generated"
  header, which golangci-lint skips; do not add exclusions for it.

## Engineering patterns (specified here so nothing external is needed)

- Migrations runner (`cmd/migrate`): `migrations/*.sql` embedded with `embed`,
  applied in filename order. Tracking table `schema_migrations(version text
primary key, applied_at timestamptz not null default now())`. Each pending file
  runs in its own transaction together with the insert of its version. The runner
  takes `pg_advisory_lock` for its whole run so two concurrent runs cannot
  collide. Running it again applies nothing. - Seed (`cmd/seed`): one
  transaction, every insert uses `ON CONFLICT DO NOTHING` on the primary or unique
  key, fixed UUIDs. Running it twice changes nothing. It never deletes. -
  Transaction helper: `WithTx(ctx, pool, fn func(pgx.Tx) error) error` begins,
  defers rollback, calls `fn`, commits. No retries and no nesting. Repos take a
  small `Querier` interface (satisfied by both `pgxpool.Pool` and `pgx.Tx`) so the
  same query function works inside or outside a transaction. Savepoints are
  allowed only where a statement must be undone without losing the outer
  transaction, and must be visible in the code. - Database session limits on the
  API pool, set per connection: `lock_timeout`, `statement_timeout`,
  `idle_in_transaction_session_timeout`, from `DB_LOCK_TIMEOUT_MS` (5000),
  `DB_STATEMENT_TIMEOUT_MS` (10000), `DB_IDLE_IN_TX_TIMEOUT_MS` (30000). Startup
  fails unless lock < statement < idle. Lock timeout (SQLSTATE 55P03) and
  statement timeout (57014) map to `503 DATABASE_BUSY` with `Retry-After: 1`. -
  Background jobs: each job is a plain function `Tick(ctx) error` plus a loop `for
{ select { case <-ctx.Done(): return; case <-ticker.C: Tick } }`. A
  `sync.WaitGroup` lets graceful shutdown wait for the in-flight tick. Every tick
  is idempotent and claims rows with `FOR UPDATE SKIP LOCKED` in a short
  transaction. A claimed row in a transitional state (`PROCESSING`) carries a
  lease: it is only re-claimed when `updated_at < now() - LEASE_SECONDS` (default
  30). Several API instances may run the jobs at once without doing the same work
  twice. A tick never holds a transaction open across an HTTP call. - External
  calls (Claude, geocoder, payment provider) never run inside a database
  transaction. Short transactions before and after, with state written before each
  side effect so a crash in between is recoverable by the reconciler. - Table
  geocoder (`internal/geocode`, type `TableGeocoder`; it is a deterministic
  stand-in for a real geocoding provider, and the name avoids confusion with
  mockery mocks): interface `Geocode(ctx, Address) (Point, error)` with
  `Address{Line1, City, PostalCode, Country}` and `Point{Lat, Lng float64}`;
  errors of type `GeocodingError`. Resolution order: (1) exact match on
  `country|postalCode` in a small built-in table; (2) case-insensitive match on
  `country|city`; (3) deterministic fallback: FNV-1a hash of
  `country|postalCode|city` mapped into a per-country bounding box (US, MX, DE,
  CO, plus a default box for any other country); (4) if both postal code and city
  are blank, return `GeocodingError`. A per-call timeout comes from
  `GEOCODER_TIMEOUT_MS` (default 2000) via the context. The built-in table must
  include every warehouse city and every sample-invoice ship-from city, with real
  coordinates, for example: Miami (25.7617, -80.1918), Houston (29.7604,
  -95.3698), Dallas (32.7767, -96.7970), Chicago (41.8781, -87.6298), Los Angeles
  (34.0522, -118.2437), New York (40.7128, -74.0060), Hamburg (53.5511, 9.9937),
  Munich (48.1351, 11.5820), Monterrey (25.6866, -100.3161), Guadalajara (20.6597,
  -103.3496), Mexico City (19.4326, -99.1332), Bogotá (4.7110, -74.0721). -
  Distance to the receiving warehouse is computed in SQL with
  `ST_Distance(a::geography, b::geography)` (meters, divided by 1000 for km) and
  passed into the pure matcher as a plain number. - Gin middleware, three small
  functions in `internal/httpapi`, registered in this order on both the API and
  the payment mock: request id (reads or generates `X-Request-Id`, sets it on the
  context and response), access log (`slog` JSON to stdout, one line per request
  with request id, method, route pattern, status, duration), recover (logs the
  panic with the request id and returns the standard error JSON with 500). Never
  log file contents, full bank account numbers (last 4 only) or API keys. -
  Health: `GET /health` on the api and on the payment mock returns
  `{"status":"ok"}` only if the database answers, otherwise 503. Compose
  healthchecks call it. - Docs (`docs/`): PlantUML sources (`.puml`) with a
  rendered PNG next to each, rendered with the `plantuml/plantuml` Docker image
  (`docker run --rm -v "$PWD/docs:/data" plantuml/plantuml -tpng "/data/*.puml"`);
  if rendering is impossible in the environment, commit the `.puml` files and say
  so. Five diagrams: `components.puml` (components and connectors: web, api
  layers, extractor, matcher, scheduler/executor, payment mock, postgres, Claude
  API), `sequence-invoice-pipeline.puml` (upload to verdict to scheduled payment,
  with transaction boundaries), `sequence-payment-execution.puml` (happy path,
  decline, timeout with lookup, UNKNOWN resolved by the reconciler),
  `deployment.puml` (compose services, ports, startup order, healthchecks) and
  `database.puml` (class diagram of the tables with keys and relations). Keep each
  one readable.

## Testing and mocks

- Stack: the standard `testing` package plus `github.com/stretchr/testify`
  (`require`, `assert`, `mock`). Mocks are generated with mockery v3
  (`github.com/vektra/mockery/v3`), configured in `.mockery.yml` with the
  `testify` template. Check `mockery version` and follow the config format of the
  version installed on my machine (install it with `go install` if missing and
  tell me which version you used). Mocks are committed so a clone builds and tests
  without mockery; `make mocks` regenerates them. Generated files are never edited
  by hand. - Interfaces live in the package that consumes them and stay small.
  Mocks are generated next to the interface, in the same package, as `_test.go`
  files, and tests are in-package (`package invoices`, not `package
invoices_test`). This avoids import cycles and keeps mock code out of the
  binaries. Use the generated `NewMockX(t)` constructors so unexpected or missing
  calls fail the test automatically. - Interfaces to mock: `Extractor` and
  `Geocoder` and the invoice `Store` (declared in `invoices`); `Gateway` and the
  payments `Store` (declared in `payments`); the service interfaces the HTTP
  handlers call (declared in `httpapi`). - Store interfaces have one method per
  database transaction, for example `SaveReceivedInvoice`, `ApplyVerdict`,
  `ClaimDuePayments`, `SettlePayment`. The transaction itself (`WithTx`, locks,
  conditional updates, plain SQL) lives inside the `internal/repo` method, never
  in services or handlers. This keeps the services as a readable sequence (claim,
  call the gateway, settle) that can be unit-tested with mocks, while every
  statement stays visible in `internal/repo`. - What gets unit tests: (a) the
  pure packages, table-driven, covering every rule and boundary in "3-way matching
  rules", "Decision and scheduling", "Money" and the FX quote; (b) the invoice
  pipeline in `invoices` with mocked `Extractor`, `Geocoder` and `Store`: clean
  invoice auto-approved and scheduled, blocked invoice waits for a human,
  duplicate file returns the existing invoice, extraction failure after the retry,
  geocoding failure; (c) the payment executor and reconciler with mocked `Gateway`
  and `Store`: approved, declined, timeout then lookup SUCCEEDED, timeout then
  lookup NOT_FOUND, lookup unreachable becomes `UNKNOWN`, no retry on a decline,
  reconciler resolving `UNKNOWN` and stale `PROCESSING`; (d) HTTP handlers through
  `httptest` and the real Gin router with mocked services, verifying status codes
  and the error JSON shape. Repositories are not unit tested; their SQL is
  exercised by `scripts/demo.sh` and the duplicate-upload race check. - Test
  style: table-driven where cases share a shape, each case sets its mock
  expectations explicitly, test names read like
  `TestProcess_BlockedOnPriceVariance`. No test sleeps; the HTTP retry backoff is
  a field with a small value set in the test, not an interface. - Time:
  production code calls `time.Now()` directly. There is no `Clock` interface, no
  `now func() time.Time` constructor argument or struct field, and no time value
  threaded through constructors for testing. A package that needs a fixed time in
  a test declares one unexported package variable in a non-test file, `var now =
time.Now`, and calls `now()`; a test in the same package sets `now = func()
time.Time { return fixed }` and restores the original with `t.Cleanup`. Only
  packages with time-dependent tests get this variable, the others call
  `time.Now()` straight, and tests of packages that override `now` do not use
  `t.Parallel()`. Pure functions may still take a `today` date as a business input
  (the scheduling rules need to compare the issue date with "today"), because that
  date comes from the demo clock, not from a testing concern. Leases and
  `expires_at` use the database `now()` and are covered by the race and demo
  scripts, not by Go unit tests.

## Layout

````go.mod, go.sum, .golangci.yml, .mockery.yml, Makefile, Dockerfile,
docker-compose.yml, .env.example cmd/api/              main for the HTTP server
+ background jobs cmd/migrate/          runs embedded migrations cmd/seed/
idempotent seed cmd/payment-mock/     separate service, own schema payment_mock
cmd/genfixtures/      one-off generator for the sample invoice PDFs (output is
committed) internal/config/ internal/db/          pool, WithTx, migration
runner, pg error helpers internal/httpapi/     Gin router, handlers, JSON
helpers, error mapping, middleware (request id, logging, recover)
internal/money/       PURE: ParseMinor, FormatMinor, rounding and overflow-safe
helpers internal/invoices/    orchestration: upload -> extract -> match ->
decide -> schedule internal/extraction/  ClaudeExtractor, FixtureExtractor
(implement the `Extractor` interface declared by the consumer, `invoices`)
internal/matching/    PURE functions: 3-way match rules, tolerances. No DB, no
I/O internal/payments/    `Gateway` interface (consumed here) + HTTP
implementation, scheduling rules (pure), executor, reconciler internal/fx/
quote calculation (pure, integer math, see "Money") + rates lookup
internal/geocode/     TableGeocoder (lookup-table implementation of the
`Geocoder` interface declared by `invoices`; spec in "Engineering patterns")
internal/repo/        ALL SQL lives here internal/clock/       demo clock: a
product feature (day offset stored in the database), not a test seam migrations/
numbered .sql files, embedded samples/              committed PDFs +
<name>.expected.json web/                  Vue app docs/ scripts/
demo.sh ```

## Extraction (the LLM part)

- `Extractor` interface: `Extract(ctx, fileName, pdfBytes) (ExtractedInvoice,
error)`.  - `ClaudeExtractor`: sends the PDF, forces a tool call whose schema is
the `RawInvoice` shape (below), then converts `RawInvoice` to `ExtractedInvoice`
and validates in Go (required fields present, currency is ISO 4217 3 letters,
every amount passes `money.ParseMinor`, dates parse as `YYYY-MM-DD`, quantities
are positive integers, line totals present). The prompt tells the model to copy
amounts exactly as printed on the invoice, as text, and never to convert, round
or compute them. One retry with the validation error fed back; if it still
fails, the invoice goes to status `EXTRACTION_FAILED` with the error stored,
visible in the UI.  - `FixtureExtractor`: used when `ANTHROPIC_API_KEY` is
empty. Loads `samples/<file name without extension>.expected.json`, which has
exactly the `RawInvoice` shape, so the fixture path goes through the same
conversion and validation as the real model output. A visible banner in the UI
says "Fixture mode: no API key, using pre-extracted data". This is what lets
anyone who clones the repo see the whole flow without a key.  - `RawInvoice`
(what the model returns and what `.expected.json` holds): supplier name,
supplier tax id (optional), invoice number, PO number (optional), issue date,
due date (optional), currency, subtotal, tax, total, payment terms text
(optional), ship-from address (optional: line1, city, postal code, country),
lines [{ sku (optional), description, quantity (integer), unit price, line total
}]. Every monetary field is a string exactly as printed, for example
`"1,250.00"`.  - `ExtractedInvoice`: the same fields after conversion, with
every monetary field an `int64` in minor units, dates as `time.Time` at midnight
UTC, and nothing optional left ambiguous (empty string means absent).  - Store
the raw model output JSON on the invoice (`extraction jsonb`) and the file bytes
in a `bytea` column (demo only; say in README that production uses object
storage).

## Schema (migrations)

All money is `bigint` minor units, rates are `bigint` scaled by 1e8, percentages
are `int` basis points. No `numeric` or floating-point columns for any of them.
Use timestamptz for instants and date for business dates. No comments in SQL.

- suppliers(id uuid, name, tax_id, country, currency, bank_account,
payment_net_days int, early_discount_bps int, early_discount_days int) -
warehouses(id uuid, supplier_id FK, name, city, country, location
geography(Point,4326) NOT NULL) + GiST index. These are the supplier dispatch
warehouses that goods are sent from.  - purchase_orders(id uuid, po_number
unique, supplier_id FK, currency, status OPEN|CLOSED, created_at) -
po_lines(po_id FK, line_no, sku, description, quantity, unit_price_minor CHECK >
0, PK(po_id, line_no)) - goods_receipts(id uuid, po_id FK, warehouse_id FK,
received_on date) - receipt_lines(receipt_id FK, sku, quantity_received CHECK >
0, PK(receipt_id, sku)) - invoices(id uuid, file_name, file_sha256 UNIQUE, file
bytea, supplier_id nullable FK, po_id nullable FK, invoice_number, currency,
subtotal_minor, tax_minor, total_minor, issue_date, due_date, ship_from_city,
ship_from_country, ship_from_location geography nullable, extraction jsonb,
status, created_at, updated_at) + UNIQUE(supplier_id, invoice_number) WHERE
supplier_id IS NOT NULL - invoice_lines(invoice_id FK, line_no, sku,
description, quantity, unit_price_minor, line_total_minor, PK(invoice_id,
line_no)) - findings(id, invoice_id FK, line_no nullable, code, severity
WARNING|BLOCK, expected text, actual text, message text) - payments(id uuid,
invoice_id UNIQUE FK, status SCHEDULED|PROCESSING|PAID|FAILED|UNKNOWN,
amount_minor, currency, discount_minor, scheduled_for date, funding_currency,
fx_rate_e8 bigint, fx_spread_bps int, funding_amount_minor,
processor_payment_id, error, updated_at) - fx_rates(base, quote, rate_e8 bigint
CHECK > 0, spread_bps int, PK(base, quote)) -- rate_e8 = units of quote per 1
base, times 1e8 - audit_log(id, invoice_id FK, actor, action, detail jsonb,
created_at) + index (invoice_id, created_at) - demo_clock(id int PK CHECK id =
1, offset_days int)

Invoice statuses: RECEIVED, EXTRACTION_FAILED, MATCHED, MATCHED_WITH_WARNINGS,
BLOCKED, APPROVED, REJECTED, PAYMENT_SCHEDULED, PAID, PAYMENT_FAILED,
PAYMENT_UNKNOWN.

## 3-way matching rules (internal/matching, pure, table-tested)

Input: extracted invoice, resolved supplier, PO with lines, all receipts for the
PO with lines, quantities already invoiced on this PO by other non-rejected
invoices, distance in km between invoice ship-from and the receipt warehouse
(nullable). Output: list of findings and a verdict.

- Supplier not found -> BLOCK `UNKNOWN_SUPPLIER`. PO number missing or not found
-> BLOCK `NO_PO`. PO belongs to another supplier -> BLOCK
`PO_SUPPLIER_MISMATCH`.  - Same supplier + invoice number already exists, or
same file hash -> BLOCK `DUPLICATE_INVOICE`.  - Currency differs from the PO ->
BLOCK `CURRENCY_MISMATCH`.  - Each invoice line matches a PO line by SKU
(fallback: normalized description equality). No match -> BLOCK `LINE_NOT_ON_PO`.
- Price: unit price equal -> ok; within `PRICE_TOLERANCE_BPS` (default 200,
meaning 2%) -> WARNING `PRICE_VARIANCE_WITHIN_TOLERANCE`; above -> BLOCK
`PRICE_VARIANCE`. Compare in integers: variance is within tolerance when
`abs(invoice - po) * 10000 <= po * toleranceBps`.  - Quantity: previously
invoiced + this invoice > total received -> BLOCK `QTY_EXCEEDS_RECEIVED`
(message states ordered / received / already invoiced / this invoice). Invoice
quantity below received is fine (partial billing).  - Arithmetic: sum of line
totals + tax != total (tolerance 1 minor unit per line) -> WARNING
`TOTAL_MISMATCH`.  - Origin: invoice ship-from farther than
`ORIGIN_TOLERANCE_KM` (default 100) from the receipt's warehouse -> WARNING
`ORIGIN_MISMATCH`; different country -> BLOCK `ORIGIN_COUNTRY_MISMATCH`. Skip
when ship-from is absent. The distance comes from PostGIS (`ST_Distance` between
the geocoded ship-from point and the warehouse location) computed in the repo
layer and passed in.  - Verdict: any BLOCK -> BLOCKED; else any WARNING ->
MATCHED_WITH_WARNINGS; else MATCHED.

Every finding has a deterministic English `message` written in Go. Claude is
never asked to decide anything here.

## Decision and scheduling (internal/payments, pure, table-tested)

- MATCHED and `AUTO_APPROVE_CLEAN=true` (default) -> APPROVED automatically,
actor `agent`, then payment scheduled in the same transaction.  -
MATCHED_WITH_WARNINGS -> waits; a human approves. BLOCKED -> waits; a human
rejects, or overrides with a mandatory reason (stored in audit_log).
Approve/override/reject are conditional updates: `UPDATE invoices SET status=...
WHERE id=$1 AND status IN (...)`, 409 if zero rows.  - Schedule date: if the
supplier offers an early discount and the discount window (issue_date +
early_discount_days) is still open on the demo-clock "today", pay on the last
day of that window with the discount applied; otherwise pay on due_date (or
issue_date + payment_net_days when due_date is missing), and today if already
overdue. Record `discount_minor`, computed as `round_half_up(amount_minor *
early_discount_bps / 10000)` in integers; the payment amount is `amount_minor -
discount_minor`.  - FX quote: the buyer funds from a base currency
(`BUYER_BASE_CURRENCY`, default COP). Funding amount is
`round_half_up(amount_minor * rate_e8 * (10000 + spread_bps) / (100000000 *
10000))`, computed with `math/big.Int` and checked to fit `int64` (see "Money").
The rate here is the rate from the supplier's currency to the buyer's funding
currency. Store `fx_rate_e8`, `fx_spread_bps` and `funding_amount_minor` on the
payment so the quote is auditable. Same currency -> rate 1 (`rate_e8 =
100000000`), spread 0.

## Payment mock (separate service, own schema `payment_mock`)

- `POST /payments` with `Idempotency-Key` header and body `{ destinationAccount,
amountMinor, currency, description }`. Same key returns the stored result
without paying twice.  - `GET /payments/:key` -> SUCCEEDED | NOT_FOUND |
DECLINED.  - Storage: table `payment_mock.payments(key text primary key,
destination_last4 text, amount_minor bigint, currency text, status text,
created_at timestamptz)`, created by the service itself on startup with `CREATE
SCHEMA IF NOT EXISTS` / `CREATE TABLE IF NOT EXISTS`. The API never reads this
schema. `GET /health` returns 200 when the database answers.  - Behaviour by
destination account prefix: `4242` approved; `0000` declined (402); `1111`
records SUCCEEDED then sleeps 10 seconds on every call (longer than the 3s
client timeout); `2222` sleeps 10 seconds WITHOUT recording anything. Any other
prefix approved. Seed suppliers so each behaviour is reachable from at least one
sample invoice. Never log full account numbers (last 4 only).

## Payment execution and background jobs

In-process goroutines with `time.Ticker`, each tick idempotent, graceful stop
via context.

- Executor (every `EXECUTE_INTERVAL_MS`, default 5000): claim due payments in a
short transaction with `SELECT ... WHERE status='SCHEDULED' AND scheduled_for <=
$today ORDER BY scheduled_for FOR UPDATE SKIP LOCKED LIMIT n`, set `PROCESSING`;
call the gateway OUTSIDE any transaction with idempotency key = payment id
(per-call timeout 3s, max 3 attempts with backoff, retry only
timeouts/5xx/network, never a decline); then a second short transaction marks
`PAID` (invoice -> PAID) or `FAILED` (invoice -> PAYMENT_FAILED). After retries
exhausted, call `GET /payments/:key`: SUCCEEDED -> PAID, NOT_FOUND -> FAILED,
lookup unreachable -> `UNKNOWN`.  - Reconciler (every `RECONCILE_INTERVAL_MS`,
default 10000): resolves `UNKNOWN` and stale `PROCESSING` payments (lease on
`updated_at`) via the lookup, with `SKIP LOCKED`.  - Every state change writes
an audit_log row in the same transaction.

## API (prefix /api)

- `POST /api/invoices` multipart `file`. Computes sha256, `INSERT ... ON
CONFLICT (file_sha256) DO NOTHING RETURNING`; if it conflicts, return the
existing invoice with 200 (idempotent re-upload; never SELECT-then-INSERT). New
file: extract (outside any transaction, timeout `EXTRACT_TIMEOUT_MS` default
60000), geocode ship-from (outside any transaction), then match + decide +
schedule in one transaction. Responds 201 with the full invoice. Processing is
synchronous; the UI shows a spinner.  - Route parameters use Gin syntax (`:id`,
`:name`); the paths below are written that way.  - `GET /api/invoices`, `GET
/api/invoices/:id` (invoice, lines, findings, PO + receipts side by side,
payment, audit log), `GET /api/invoices/:id/file` (the PDF).  - `POST
/api/invoices/:id/approve`, `/reject` `{reason}`, `/override` `{reason}` (409 if
the status does not allow it).  - `GET /api/payments`, `POST
/api/payments/run-due` (runs one executor tick now), `GET /api/purchase-orders`
(with receipts).  - `GET /api/samples`, `POST /api/samples/:name/process` (loads
a committed sample PDF through the same path as an upload, so a reviewer can
click through all scenarios without finding files).  - `POST
/api/demo/advance-days` `{days}` (moves the demo clock so scheduled payments
come due), `POST /api/demo/reset` (truncate invoices, findings, payments, audit;
reset clock; keep master data).  - `POST /api/invoices/:id/supplier-email`
(stretch, last): Claude drafts a short email to the supplier explaining the
blocking findings, from the stored findings only. In fixture mode return a
deterministic template. Mark clearly in the UI that it is a draft.  - Errors
share one shape `{ "error": { "code", "message", "details"? } }`. Status codes:
200/201, 400 bad request, 404, 409 invalid state transition, 422 validation /
unreadable file, 502/504 upstream (Claude or payment provider).

## UI (Vue 3 + TS, three views, light and clean)

1. Invoices: table with supplier, invoice number, amount + currency, status
badge (green matched, amber warnings, red blocked, blue scheduled, grey paid),
created time. An "Upload invoice" drop zone and a "Try a sample" dropdown
listing `/api/samples`. Banner when running in fixture mode.  2. Invoice detail:
left, the PDF preview (`<iframe>` or `<embed>`) next to the extracted fields;
right, the 3-way match table (one row per line: PO qty/price, received qty,
invoiced qty/price, cell colouring per finding), the findings list with
plain-language messages, the action bar (Approve / Reject / Override with a
required reason modal, only the actions valid for the status), the payment card
(scheduled date, discount saved, FX quote showing "supplier receives X USD, you
pay Y COP at rate R"), and the audit timeline. Poll or refetch after each
action.  3. Payments and demo controls: scheduled and executed payments, a "Run
due payments" button, "Fast-forward N days", and "Reset demo".

TypeScript types mirror the API responses. No component library. Responsive down
to a laptop screen is enough.

## Seed and samples

- Buyer is configured by env (`BUYER_NAME`, `BUYER_COUNTRY=CO`,
`BUYER_BASE_CURRENCY=COP`).  - 5 suppliers in different countries and currencies
(USD US, EUR DE, MXN MX, USD US with bank account prefix `1111`, USD with prefix
`2222`), each with 1-2 warehouses at real coordinates, realistic payment terms
(one with `2/10 net 30`).  - 5-6 POs with 2-4 lines each, goods receipts
including one PO with two partial receipts.  - `fx_rates` for USD/COP, EUR/COP,
MXN/COP with plausible static values stored as `rate_e8` (for example a USD/COP
rate of 4,000.00 is `400000000000`) and a spread of 80-150 bps. The early
discount of `2/10 net 30` is stored as `early_discount_bps = 200`,
`early_discount_days = 10`, `payment_net_days = 30`.  - Stable, readable UUIDs
so the README and curl examples can reference them:
`11111111-0000-4000-8000-0000000000NN` for suppliers, `22222222-...` for
warehouses, `33333333-...` for purchase orders, `44444444-...` for goods
receipts, with NN a two-digit counter. The seed is idempotent (see "Engineering
patterns").  - 8 sample invoices as PDFs in `samples/`, each with an
`.expected.json`. `cmd/genfixtures` generates them (use
`github.com/go-pdf/fpdf`; handle Spanish accents with the cp1252 translator)
with at least 3 visibly different layouts, including a Spanish one (Factura /
Subtotal / IVA). Commit the PDFs so nobody needs to run the generator.
Scenarios:
  1. Clean, USD, everything matches -> auto-approved and scheduled.  2. Clean,
  Spanish, MXN supplier, early-payment discount window open -> auto-approved,
  paid early with discount.  3. Unit price 8% above PO -> BLOCKED
  `PRICE_VARIANCE`.  4. Quantity billed exceeds quantity received -> BLOCKED
  `QTY_EXCEEDS_RECEIVED`.  5. Same invoice number as sample 1 sent again as a
  different file -> BLOCKED `DUPLICATE_INVOICE`.  6. Price 1% above PO ->
  MATCHED_WITH_WARNINGS, human approves.  7. EUR German supplier billing only
  what the first of two partial receipts covered -> MATCHED.  8. Ship-from city
  far from the receiving warehouse -> MATCHED_WITH_WARNINGS `ORIGIN_MISMATCH`;
  plus an invoice for the `2222` supplier that is otherwise clean, so approving
  it demonstrates the payment UNKNOWN -> reconciler path.  If more than 8 files
  are needed to cover the payment failure paths (`0000`, `1111`, `2222`), add
  them; keep the list small.

## Decisions already made (do not revisit)

- Go 1.27 + Gin + pgx, Vue 3 + TS, Postgres + PostGIS, no ORM, no auth, no
queues, no cloud deployment.  - Money is `int64` cents, rates are scaled
integers, percentages are basis points. No floats anywhere near money.  - The
LLM extracts; deterministic code matches, decides, schedules and pays.  -
Synchronous processing on upload; payments are the only asynchronous part.  -
Demo clock instead of waiting for real dates.  - Unit tests use testify and
mockery mocks of small consumer-side interfaces. Time is never injected: code
calls `time.Now()` directly, and only where a test needs a fixed time a
package-level `var now = time.Now` is replaced in-package.  - Mocked: geocoder,
FX provider, payment provider, ERP. The README says how each would be integrated
in a real customer deployment (ERP sync of POs and receipts, bank/FX provider,
object storage for files, queue for extraction).

## Phases

1. Repo skeleton, `go.mod`, `.golangci.yml`, Makefile, Dockerfiles (layered),
compose, config, db helpers, migrations, seed, `internal/money` with its table
tests, `.mockery.yml` and `make mocks` wired up (the first mocks appear in phase
4), Gin router with the three middleware and `GET /health`, web scaffold that
shows API health.  2. Payment mock service.  3. Extraction: `ExtractedInvoice`,
FixtureExtractor, ClaudeExtractor, sample generator + the 8 PDFs + expected
JSON. A small `go run ./cmd/api extract-check` style command (or a `make`
target) that prints extraction output for a sample.  4. Matching package + table
tests, repo SQL, `POST /api/invoices` end to end up to a verdict and findings,
`GET` endpoints, samples endpoints, mockery mocks for `Extractor`, `Geocoder`
and the invoice `Store`, the pipeline unit tests and the handler tests for these
endpoints.  5. Decision, approve/reject/override, schedule + FX quote, audit
log, with table tests for the scheduling rules and the FX quote and handler
tests for the new endpoints.  6. Executor, reconciler, demo clock, run-due,
reset, mocks for `Gateway` and the payments `Store`, and the executor and
reconciler unit tests. `scripts/demo.sh` (curl through every scenario) and a
duplicate-upload race check (10 parallel uploads of the same file -> exactly one
invoice).  7. Vue UI: the three views.  8. README (run in 2 commands, how to run
lint and tests and regenerate mocks, sample walkthrough in the order a reviewer
should click, architecture, state machine, design decisions and trade-offs, what
was cut, what a real customer deployment would integrate), DESIGN.md (pipeline
flow in text, transaction boundaries listing what each transaction locks and
writes, and a failure matrix table with columns: failure, where it happens,
effect on invoice and payment, how it recovers), the five docs/ diagrams as
PlantUML + rendered PNGs. Stretch only if time remains: supplier-email draft.

After each phase, run `make lint` and `make test` (both must be clean), then run
the project and verify it works before continuing. Tell me briefly what you
built and anything you were unsure about.
````
