# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A take-home demo: an AI accounts-payable agent for cross-border supplier invoices. `instructions.md` is the authoritative spec (schema, matching rules, scheduling rules, API, UI, samples, phases). Read the relevant section before changing behaviour; it overrides anything here.

Work proceeds in the 8 phases listed at the end of `instructions.md`. After each phase: `make lint` and `make test` must be clean, `go vet ./...` and `go build ./...` must pass, run the stack and verify it, then stop and ask the user for review. The user makes the commits; do not commit.

## Commands

```sh
make up          # docker compose up --build -d --wait (postgres -> migrate -> seed -> api -> web)
make down        # tear down including volumes
make lint        # golangci-lint run (must be 0 issues; fix code instead of adding //nolint)
make fmt         # golangci-lint fmt
make test        # go test -race ./...
make mocks       # regenerate mockery mocks from .mockery.yml
make dev-api     # go run ./cmd/api against localhost:5432
make dev-web     # vite dev server on :5173, proxies /api to :3000

go test -race ./internal/money -run TestParseMinor/three_decimals   # single test / subtest
docker compose run --rm migrate    # re-run migrations (idempotent)
docker compose run --rm seed       # re-run seed (idempotent)
cd web && npm run build            # vue-tsc type check + vite build
go run ./cmd/api extract-check samples/01-clean-usd.pdf   # print extraction output (Claude if ANTHROPIC_API_KEY is set, else fixture)
go run ./cmd/genfixtures [-today YYYY-MM-DD]               # regenerate samples/*.pdf + .expected.json (dates relative to -today)
```

Ports: web 8080, api 3000, payment-mock 4000, postgres 5432. `postgis/postgis` runs as `linux/amd64` (emulated on Apple Silicon). Tool versions: Go 1.27, golangci-lint v2, mockery v3.

## Core principle

The LLM only reads documents (extraction) and optionally drafts text. Every decision that moves money (matching, approval, scheduling, FX, payment) is deterministic Go code.

## Hard code rules (from the spec, easy to violate)

- **No comments** unless explaining a non-obvious "why". No doc comments, banners, or SQL comments. Explanations go in README.md / DESIGN.md.
- **No floats for money, rates or percentages.** Amounts are `int64` minor units (all currencies treated as 2 decimals), percentages and spreads are `int` basis points, FX rates are `int64` scaled by 1e8 (`rate_e8`). SQL money columns are `bigint`. Use `internal/money` for parsing, formatting, overflow-checked arithmetic, and `MulDivHalfUpBig` for the FX product. Floats are only allowed for lat/lng and km distances.
- **All SQL lives in `internal/repo`**, raw SQL with pgx, no ORM. Locks, transactions and conditional updates (`UPDATE ... WHERE id=$1 AND status IN (...)` → 409 on zero rows) must be visible there. Repo functions take `db.Querier` so they work with a pool or a tx. Exceptions: `cmd/seed`, the migration runner in `internal/db`, and `cmd/payment-mock` (a stand-in for an external provider with its own schema) keep their own SQL.
- **Store interfaces have one method per DB transaction** (e.g. `SaveReceivedInvoice`, `ApplyVerdict`, `ClaimDuePayments`, `SettlePayment`). `db.WithTx` is called inside the repo method, never in services or handlers.
- **External calls (Claude, geocoder, payment provider) never run inside a transaction.** Use short transactions before and after, and write state before each side effect so the reconciler can recover.
- **Background jobs** are `Tick(ctx) error` functions plus a ticker loop, tracked by a `sync.WaitGroup` for graceful shutdown. Each tick is idempotent, claims rows with `FOR UPDATE SKIP LOCKED`, and uses a lease on `updated_at` (database `now()`) for `PROCESSING` rows.
- **Geography:** never scan geography columns into Go. Insert with `ST_SetSRID(ST_MakePoint($lng,$lat),4326)::geography`. Compute distances in SQL with `ST_Distance` and pass km into the pure matcher.
- **Time:** no Clock interface and no injected `now`. Production code calls `time.Now()`. Only a package with time-dependent tests declares `var now = time.Now` and overrides it in-package (those tests do not use `t.Parallel()`). The demo clock (`internal/clock`, offset stored in `demo_clock`) is a product feature, not a test seam.
- **Gin:** `gin.New()` in release mode. The middleware order is RequestID → AccessLog → Recover (`internal/httpapi/middleware.go`). Errors use one JSON shape, `{"error":{"code","message","details?"}}`, via `writeError`. DB lock or statement timeouts (SQLSTATE 55P03 / 57014, `db.IsBusy`) map to `503 DATABASE_BUSY` with `Retry-After: 1`. Never log file contents, API keys, or more than the last 4 digits of a bank account.

## Architecture

- `cmd/api`: HTTP server plus in-process background jobs (executor, reconciler). It also has a `healthcheck` subcommand used by the compose healthcheck, because the runtime images are distroless and have no shell.
- `cmd/migrate`: applies embedded `migrations/*.sql` in filename order, each file in its own transaction, under `pg_advisory_lock`, tracked in `schema_migrations`.
- `cmd/seed`: a single transaction; every insert uses `ON CONFLICT DO NOTHING`; never deletes. Fixed readable UUIDs: `11111111-…` suppliers, `22222222-…` warehouses, `33333333-…` POs, `44444444-…` receipts. Each sample-invoice scenario has its own PO (or its own PO lines) so scenarios don't affect each other's invoiced quantities. Supplier bank-account prefixes drive the payment-mock behaviour: `4242` approved, `0000` declined, `1111` records then times out, `2222` times out without recording.
- `cmd/payment-mock`: a separate Gin service that owns the `payment_mock` schema; the API never reads that schema. It stores the full destination account (the provider is the regulated party that holds payment details), but logs only the last 4 digits. While a `2222` call is sleeping, the mock keeps an in-memory (unrecorded) in-flight marker, so a lookup for that key also hangs. The client sees the lookup as unreachable and marks the payment UNKNOWN. After the sleep ends, a lookup returns `NOT_FOUND`, which the reconciler resolves to FAILED.
- Pure packages, table-tested, with no DB and no I/O: `internal/money`, `internal/matching`, `internal/fx`, and the scheduling rules in `internal/payments`.
- Orchestration: `internal/invoices` (upload → extract → geocode → match + decide + schedule) and `internal/payments` (executor and reconciler). Each declares the small interfaces it consumes (`Extractor`, `Geocoder`, `Store`, `Gateway`).
- **Invoice pipeline** (`invoices.Service.Process`):
  1. `SaveReceivedInvoice` runs `INSERT ... ON CONFLICT (file_sha256) DO NOTHING`. If the file already exists and is `EXTRACTION_FAILED`, or is stuck in `RECEIVED` past `LEASE_SECONDS`, it is reclaimed and processed again. Otherwise the existing invoice is returned.
  2. Extract, then geocode the ship-from address. Both run outside any transaction.
  3. `ApplyVerdict` runs one transaction. It locks the invoice row, then the supplier row, then the PO row, always in that order (the supplier lock is what serializes duplicate-number checks). It builds the `matching.Input` and calls `rules.Decide`, then `rules.PlanPayment` on the auto-approval path. `invoices.Rules` is a pure value passed in by the service; it combines matching, `payments.ScheduleFor` and `fx.Convert`. The repo only loads data (supplier terms, the FX rate, and today's date from `demo_clock`) and writes the results.
- **Human actions:** approve, override and reject are conditional `UPDATE ... WHERE status = ANY(...)` (`repo.transition`). Zero rows means `ErrNotFound` if the invoice doesn't exist (404) and `ErrConflict` otherwise (409). Approve and override schedule the payment in the same transaction through `planPayment` and `schedulePayment`, the same functions the auto-approval path uses.
- Duplicate invoice numbers are stored as BLOCKED with `duplicate_of` pointing at the original. The unique index on (supplier, invoice number) excludes rows with `duplicate_of` set, so the database still guarantees one real invoice per supplier and number. Each `invoice_lines.po_line_no` records the PO line it matched, and the "already invoiced" sums use it.
- A geocoding failure doesn't stop matching: the origin check becomes a WARNING `ORIGIN_UNVERIFIED`. Distance comes from `ST_Distance` to the nearest receiving warehouse of the PO.
- `internal/extraction`: `ClaudeExtractor` and `FixtureExtractor`; the fixture one is used when `ANTHROPIC_API_KEY` is empty and loads `samples/<name>.expected.json`. Both go through `extraction.Parse` (`RawInvoice` → `ExtractedInvoice` plus validation). Validation failures wrap `extraction.ErrInvalid`; any other error is an upstream failure. Claude is asked to call the `record_invoice` tool with `strict: true` and `tool_choice: auto`, because `claude-sonnet-5-5` rejects a forced `tool_choice` with a 400. A missing tool call or an invalid result gets one retry, with the error sent back as a `tool_result` marked `is_error`.
- `cmd/genfixtures`: generates the sample PDFs (fpdf, three layouts: classic, modern, Spanish factura) and their `.expected.json` from the same data, so the two always agree. The dates are relative to the run day, so regenerate the samples before a demo. The output is deterministic for a given `-today`.
- `web/`: Vue 3 + strict TypeScript + vue-router; no UI library and no Pinia. `src/api.ts` is the typed fetch wrapper. JSON carries amounts as integer minor units, and the UI formats them only for display. In compose, nginx serves the bundle and proxies `/api/` to the api service. `/health` is also served at `/api/health` for that reason.

## Testing and mocks

- testify (`require`, `assert`, `mock`) plus mockery v3 using the `testify` template. To add a mock, list the interface under `packages:` in `.mockery.yml`, then run `make mocks`. Mocks are generated as `mock_<name>_test.go` next to the interface, in the same package. Tests are in-package (`package invoices`, not `invoices_test`). Never hand-edit generated files.
- Use the `NewMockX(t)` constructors with `EXPECT()`. Test names read like `TestProcess_BlockedOnPriceVariance`. No sleeps: retry backoff is a struct field set small in tests.
- Repositories are not unit tested; their SQL is exercised by `scripts/demo.sh` and the duplicate-upload race check.
