# Design

This document has three parts:
- **The pipeline in text:** what happens to an invoice from upload to payment.
- **Transaction boundaries:** every transaction, with what it locks and what it writes.
- **The failure matrix:** what happens when each piece fails, and how it recovers.

The diagrams in [docs/](docs) show the same flows visually:
- [sequence-invoice-pipeline.png](docs/sequence-invoice-pipeline.png)
- [sequence-payment-execution.png](docs/sequence-payment-execution.png)

## 1. Pipeline flow

**Upload** (`POST /api/invoices`, or `POST /api/samples/:name/process` for a committed sample)

1. **Check the file.** The handler caps the request size (`EXTRACT_MAX_FILE_MB`), reads the multipart `file` and checks the `%PDF-` header.
2. **Hash it.** `invoices.Service.Process` hashes the bytes with SHA-256.
3. **Receive** (transaction 1). `SaveReceivedInvoice` inserts the invoice as `RECEIVED` with `ON CONFLICT (file_sha256) DO NOTHING RETURNING id`.
   - If the file already exists and failed extraction, or has been stuck in `RECEIVED` longer than the extraction timeout plus `LEASE_SECONDS`, it is reclaimed and processed again.
   - Otherwise the existing invoice is returned with 200, and nothing else runs.
4. **Extract** (no transaction). From here on, the work is detached from the HTTP request, so a closed browser tab doesn't abort it. Each step has its own timeout.
   - `ClaudeExtractor` sends the PDF with the strict `record_invoice` tool and allows one retry on a validation failure.
   - `FixtureExtractor` loads the committed `.expected.json` instead.
   - Both end in `extraction.Parse`: amounts become `int64` minor units and dates become UTC days, and the result is validated.
   - **On failure,** `MarkExtractionFailed` (a short transaction) stores the error and sets `EXTRACTION_FAILED`. The API answers 422 (invalid), 504 (timeout) or 502 (upstream), with the invoice id in the error details.
5. **Geocode** (no transaction). The ship-from address goes through the table geocoder with `GEOCODER_TIMEOUT_MS`. A failure is not fatal: the match records `ORIGIN_UNVERIFIED`.
6. **Decide** (transaction 2). `ApplyVerdict` locks the invoice, supplier and PO, loads everything matching needs, and calls `Rules.Decide`. It writes the header, lines, findings and verdict.
   - If the verdict is `MATCHED` and `AUTO_APPROVE_CLEAN` is on, it also plans the payment (`Rules.PlanPayment`: scheduling + FX), approves as `agent`, inserts the payment and moves the invoice to `PAYMENT_SCHEDULED`, all in the same transaction.
   - If the payment can't be planned (no FX rate, nothing left to pay), the invoice stays `MATCHED` with an `auto_approval_skipped` audit entry.
7. **Respond.** The detail is read in one read-only `REPEATABLE READ` snapshot and returned with 201.

**Supplier email draft** (`POST /api/invoices/:id/supplier-email`) is a read plus an LLM call, with no transaction open during the call. It reads the invoice (409 unless it is BLOCKED), keeps only the BLOCK findings (422 if there are none), and asks the `Drafter` for a subject and body: Claude with a strict tool and one retry if it doesn't call the tool, or the template in fixture mode. It has its own 20s timeout. Nothing is written; the draft is only returned.

**Human decisions** (`/approve`, `/override`, `/reject`) are one transaction each. A conditional status update guards the transition. Approve and override schedule the payment inside the same transaction, through the same `planPayment` and `schedulePayment` functions the agent uses.

**Payments** run in two background loops inside the API process, plus `POST /api/payments/run-due` for the demo:

- **Executor** (every `EXECUTE_INTERVAL_MS`):
  1. Claim one due payment (transaction).
  2. Call the provider outside any transaction: 3s per call, up to 3 attempts, retrying only transient errors (timeout, 5xx, network) and never a decline.
  3. If the outcome is unresolved, look the payment up.
  4. Settle it (transaction).
  5. Repeat, up to `BatchSize`.
- **Reconciler** (every `RECONCILE_INTERVAL_MS`): claims `UNKNOWN` and `PROCESSING` payments whose `updated_at` is older than `LEASE_SECONDS`. For UNKNOWN payments the lease acts as a backoff between provider lookups; with the defaults an UNKNOWN payment is re-checked about 30–40s after it was marked. It looks each one up and settles it.

## 2. Transaction boundaries

Every API connection sets three limits: `lock_timeout` (5s) < `statement_timeout` (10s) < `idle_in_transaction_session_timeout` (30s). A lock or statement timeout rolls the transaction back and maps to `503 DATABASE_BUSY` with `Retry-After: 1`. No transaction is ever open while calling Claude, the geocoder or the payment provider. **Lock order is always invoice → supplier → PO.**

| Transaction | Repo method | Locks | Writes |
|---|---|---|---|
| Receive upload | `SaveReceivedInvoice` | The unique index on `file_sha256`: a concurrent insert of the same file waits, then does nothing. The reclaim is a conditional `UPDATE` that row-locks the invoice. | `invoices` (bytes + `RECEIVED`) or a reclaim to `RECEIVED`; `audit_log` |
| Record extraction failure | `MarkExtractionFailed` | Row lock through the conditional `UPDATE ... WHERE status='RECEIVED'` | `invoices.status = EXTRACTION_FAILED`, `extraction_error`; `audit_log` |
| Verdict (and auto-approval) | `ApplyVerdict` | `SELECT ... FOR UPDATE` on the invoice, the supplier (serializes duplicate-number checks) and the PO (serializes "already invoiced" quantities) | Invoice header (supplier, PO, `duplicate_of`, amounts, dates, `ship_from_location`, raw extraction), `invoice_lines`, `findings`, status = verdict, `audit_log`. When auto-approved: status `APPROVED` → `payments` row (`SCHEDULED`, FX rate, spread, funding amount) → `PAYMENT_SCHEDULED`, with an audit row for each step |
| Approve | `Approve` | Conditional `UPDATE ... WHERE status = ANY('{MATCHED,MATCHED_WITH_WARNINGS}')` | `APPROVED`, `payments`, `PAYMENT_SCHEDULED`, `audit_log` |
| Override | `Override` | `SELECT duplicate_of ... FOR UPDATE` (a duplicate is refused with 409), then a conditional update from `BLOCKED` | Same as Approve, with the reason in the audit row |
| Reject | `Reject` | Conditional update from `MATCHED`, `MATCHED_WITH_WARNINGS`, `BLOCKED` or `EXTRACTION_FAILED` | `REJECTED`, `audit_log` with the reason |
| Read invoice detail | `GetInvoice` | None: read-only `REPEATABLE READ` snapshot | — |
| Claim a due payment | `ClaimDuePayments` | `SELECT ... WHERE status='SCHEDULED' AND scheduled_for <= demo today ... FOR UPDATE SKIP LOCKED LIMIT 1` | `payments.status = PROCESSING`, `updated_at = now()` (starts the lease); `audit_log` |
| Claim stale payments | `ClaimStalePayments` | `SELECT ... WHERE status IN ('UNKNOWN','PROCESSING') AND updated_at < now() - lease FOR UPDATE SKIP LOCKED` | `PROCESSING`, `updated_at = now()` |
| Settle a payment | `SettlePayment` | Conditional `UPDATE payments ... WHERE status='PROCESSING'`, then a conditional `UPDATE invoices ... WHERE status IN ('PAYMENT_SCHEDULED','PAYMENT_UNKNOWN')`. Zero rows is a conflict and rolls back. | Payment status, provider id, error; invoice `PAID` / `PAYMENT_FAILED` / `PAYMENT_UNKNOWN` / `PAYMENT_SCHEDULED`; `audit_log` |
| Advance the demo clock | `AdvanceDemoClock` | Row lock on `demo_clock` | `offset_days` |
| Reset the demo | `ResetDemo` | `TRUNCATE` (ACCESS EXCLUSIVE on the invoice and payment tables) | Truncates `audit_log`, `findings`, `invoice_lines`, `payments`, `invoices`; `offset_days = 0` |
| Migrations | `db.Migrate` | `pg_advisory_lock` for the whole run | One transaction per file: the file's SQL plus its `schema_migrations` row |
| Seed | `cmd/seed` | — | One transaction; every insert is `ON CONFLICT DO NOTHING` |

## 3. Failure matrix

| Failure | Where it happens | Effect on invoice and payment | How it recovers |
|---|---|---|---|
| Claude returns 5xx or is unreachable | Extraction, no transaction open | Invoice `EXTRACTION_FAILED` with the error stored. API 502. No payment. | Re-upload the same file: the reclaim retries extraction. The SDK already retried 429/5xx twice. |
| Claude does not answer within `EXTRACT_TIMEOUT_MS` | Extraction | `EXTRACTION_FAILED`, API 504 | Same: re-upload. |
| Model output fails validation twice (bad currency, unparseable amount, 3 decimals, due before issue …) | `extraction.Parse` after one retry with the errors fed back | `EXTRACTION_FAILED`, API 422 with both attempts' errors. No payment. | A human rejects it, or re-uploads once the cause is fixed. Nothing is guessed. |
| Model refuses, or the response hits `max_tokens` | `ClaudeExtractor` | `EXTRACTION_FAILED` (422), no retry | Same as above. |
| Browser closed during processing | Request context | None: processing is detached once the row exists | The result appears in the list. |
| API crashes after receive, before the verdict commits | Between transactions 1 and 2, or inside 2 (rolled back) | Invoice stuck in `RECEIVED`, no lines, no payment | Re-uploading after the reclaim window (extraction timeout + lease) processes it again. |
| Geocoder fails or times out | Geocode step | Match continues. WARNING `ORIGIN_UNVERIFIED`, so the invoice waits for a human. | A reviewer approves or rejects. |
| Same file uploaded concurrently | Transaction 1 | Exactly one invoice (`ON CONFLICT`); the others get it with 200 | Nothing to do (checked by `make demo`: 10 parallel uploads). |
| Same supplier and invoice number, different files, concurrently | `ApplyVerdict` | The supplier row lock serializes them. The second is BLOCKED `DUPLICATE_INVOICE` with `duplicate_of`; the unique index is the backstop. | Reject the duplicate. Override is refused, because it would pay twice. |
| Claude fails or times out while drafting a supplier email | `ClaudeDrafter`, no transaction open | None: nothing is stored | API 502 / 504. Click **Draft again**. |
| Lock or statement timeout | Any transaction | Transaction rolled back, state unchanged | API 503 `DATABASE_BUSY` + `Retry-After: 1`; the client retries. |
| No FX rate, or nothing left to pay after discount or rounding | `PlanPayment` | Auto path: stays `MATCHED` + `auto_approval_skipped` audit. Human approve: 422 `CANNOT_SCHEDULE`, nothing written. | Add the rate (or fix the terms), then approve. |
| Two reviewers act at once | Approve / override / reject | First wins (conditional update); the second gets 409 `INVALID_STATE` with the current status | The UI refetches and shows the new state. |
| Provider declines (402) | Executor | Payment `FAILED` ("declined by the destination bank"), invoice `PAYMENT_FAILED`. Never retried. | Human follow-up with the supplier. No re-pay action in the demo. |
| Provider times out or returns 5xx | Executor, no transaction open | Up to 3 attempts with backoff, then a lookup: `SUCCEEDED` → `PAID`, `NOT_FOUND` → `FAILED` | Automatic. The same idempotency key means no double payment (the `1111` sample). |
| Provider lookup also unreachable | Executor | Payment `UNKNOWN`, invoice `PAYMENT_UNKNOWN` | The reconciler re-checks after the lease: `PAID`, `FAILED`, or `UNKNOWN` again (the `2222` sample). |
| Ambiguous provider answer (200 that doesn't decode, unexpected status, 4xx after an earlier timeout) | Executor | Not settled blindly: always resolved through the lookup | `PAID` if the provider has it, `FAILED` with the original error if not, `UNKNOWN` while pending. |
| API crashes after claiming, before calling the provider | Between claim and Pay | Payment stuck in `PROCESSING` | After the lease, the reconciler's lookup gets `NOT_FOUND`, so it goes back to `SCHEDULED` and is retried with the same key. |
| API crashes after the provider paid, before settling | Between Pay and settle | Payment stuck in `PROCESSING` | After the lease, the lookup gets `SUCCEEDED`, so it becomes `PAID`. The idempotency key prevents a second charge. |
| Shutdown (SIGTERM) during a batch | Executor | The current payment finishes, uncancelled; no further claims. Unclaimed payments stay `SCHEDULED`. | The next start picks them up. `stop_grace_period: 30s` covers the slowest payment (about 13.5s). |
| Two API instances running the jobs | Claims | `SKIP LOCKED` and the lease prevent double work, as long as the lease outlasts the slowest execution (about 13.5s); config rejects `LEASE_SECONDS` below 20 | Nothing to do. |
| Settle finds the row already changed (reset, other worker) | `SettlePayment` | Conditional update matches zero rows, so it rolls back with a conflict | Logged. The reconciler or the current state wins. |
| Payment mock down | Gateway | Calls become transient errors, then lookup fails: `UNKNOWN` | The reconciler resolves it once the mock is back. |
| Postgres down | Everywhere | `/health` returns 503, so compose marks api and payment-mock unhealthy. Requests fail, and job ticks log errors. | Each tick retries on its own schedule. Nothing is lost, because every change is transactional. |
