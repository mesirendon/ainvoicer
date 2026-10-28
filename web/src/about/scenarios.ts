import type { ComponentId, StageId, Tone } from './model'

export interface Step {
  stage: StageId
  actors: ComponentId[]
  title: string
  text: string
  code?: string
  tone?: Tone
}

export interface Scenario {
  id: string
  sample: string
  title: string
  tagline: string
  outcome: string
  tone: Tone
  requires?: string[]
  steps: Step[]
}

const receive: Step = {
  stage: 'upload',
  actors: ['user', 'web', 'httpapi', 'service', 'repo', 'postgres'],
  title: 'Upload and dedupe',
  text: 'The PDF is hashed with SHA-256 and inserted as RECEIVED. Uploading the same file again returns the same invoice: no SELECT-then-INSERT race.',
  code: 'INSERT INTO invoices ... ON CONFLICT (file_sha256) DO NOTHING RETURNING id',
}

function extract(text: string, printed: string): Step {
  return {
    stage: 'extract',
    actors: ['service', 'extractor', 'claude'],
    title: 'Claude reads the invoice',
    text: `${text} The model calls a strict record_invoice tool and copies every amount exactly as printed, as text. No transaction is open while it works.`,
    code: printed,
  }
}

function validate(printed: string, minor: string): Step {
  return {
    stage: 'validate',
    actors: ['extractor', 'service'],
    title: 'Go validates and converts',
    text: 'Deterministic code parses amounts into int64 minor units, checks ISO codes and dates, and rejects 3+ decimals. An invalid result gets exactly one retry with the errors fed back.',
    code: `money.ParseMinor("${printed}") → ${minor}`,
  }
}

function geocode(where: string, point: string): Step {
  return {
    stage: 'geocode',
    actors: ['service', 'geocoder'],
    title: 'Locate the ship-from address',
    text: `${where} is turned into a point, still outside any transaction. A failure here is not fatal: the origin check would become ORIGIN_UNVERIFIED.`,
    code: point,
  }
}

const matchActors: ComponentId[] = ['service', 'repo', 'rules', 'postgres']
const decideByAgent: ComponentId[] = ['repo', 'rules', 'postgres']
const decideByHuman: ComponentId[] = ['user', 'web', 'httpapi', 'service', 'repo', 'postgres']
const scheduleActors: ComponentId[] = ['repo', 'rules', 'postgres']
const payActors: ComponentId[] = ['executor', 'gateway', 'mock', 'repo', 'postgres']
const reconcileActors: ComponentId[] = ['reconciler', 'gateway', 'mock', 'repo', 'postgres']

function paid(amount: string, last4: string): Step {
  return {
    stage: 'execute',
    actors: payActors,
    title: 'The executor pays',
    text: `When the demo clock reaches the date, the executor claims the payment with FOR UPDATE SKIP LOCKED, calls the provider outside any transaction, and gets 200 SUCCEEDED for account ···${last4}.`,
    code: `POST /payments  Idempotency-Key: <payment id>  amount ${amount}`,
    tone: 'ok',
  }
}

function settled(status: string, tone: Tone, text: string): Step {
  return {
    stage: 'reconcile',
    actors: ['executor', 'repo', 'postgres'],
    title: `Settled: ${status}`,
    text,
    code: "UPDATE payments SET status = $2 WHERE id = $1 AND status = 'PROCESSING'",
    tone,
  }
}

export const scenarios: Scenario[] = [
  {
    id: 'clean',
    sample: '01-clean-usd',
    title: 'Clean invoice, paid on the due date',
    tagline: 'US supplier, USD, everything matches',
    outcome: 'Auto-approved → PAID',
    tone: 'ok',
    steps: [
      receive,
      extract('Acme Industrial Supply sends a classic English invoice for PO-1001.', '"total": "5,582.50"'),
      validate('5,582.50', '558250'),
      geocode('Houston 77002, US', '(29.7604, -95.3698)'),
      {
        stage: 'match',
        actors: matchActors,
        title: '3-way match: all green',
        text: 'Invoice lines, PO-1001 and the goods receipt agree on SKU, price and quantity. The ship-from point is 0 km from the receiving warehouse (PostGIS ST_Distance).',
        code: 'invoice = PO = receipt  →  MATCHED',
        tone: 'ok',
      },
      {
        stage: 'decide',
        actors: decideByAgent,
        title: 'The agent approves',
        text: 'A clean match with AUTO_APPROVE_CLEAN on is approved by the agent inside the same transaction that stored the verdict, under locks on invoice → supplier → PO.',
        tone: 'ok',
      },
      {
        stage: 'schedule',
        actors: scheduleActors,
        title: 'Schedule and quote FX',
        text: 'No early-payment discount, so it pays on the due date. The FX quote is computed with math/big and rounded half up once, then stored on the payment so it is auditable.',
        code: '5,582.50 USD × 4,000.00 × 1.0100 = 22,553,300.00 COP',
      },
      paid('5,582.50 USD', '3456'),
      settled('PAID', 'ok', 'A second short transaction marks the payment PAID and the invoice PAID, and writes the audit log.'),
    ],
  },
  {
    id: 'discount',
    sample: '02-factura-mxn-discount',
    title: 'Spanish factura, early-payment discount',
    tagline: 'Mexican supplier, MXN, 2/10 net 30',
    outcome: 'Paid early, 3,967.20 MXN saved',
    tone: 'ok',
    steps: [
      receive,
      extract('Textiles del Norte sends a Spanish factura: Folio, Subtotal, IVA 16%, dates as dd/mm/yyyy.', '"total": "$198,360.00"  "tax": "$27,360.00"'),
      validate('$198,360.00', '19836000'),
      geocode('Monterrey 64000, MX', '(25.6866, -100.3161)'),
      {
        stage: 'match',
        actors: matchActors,
        title: '3-way match: all green',
        text: 'Both lines match PO-2001 and the Monterrey receipt. Line totals plus IVA add up to the total, within 1 minor unit per line.',
        tone: 'ok',
      },
      { stage: 'decide', actors: decideByAgent, title: 'The agent approves', text: 'Clean match, approved automatically in the same transaction.', tone: 'ok' },
      {
        stage: 'schedule',
        actors: scheduleActors,
        title: 'Take the discount',
        text: 'The supplier offers 2/10 net 30 and the window is still open on the demo clock, so it pays on the last day of the window with 2% off, in integer basis points.',
        code: 'discount = round_half_up(19836000 × 200 / 10000) = 396720\n194,392.80 MXN × 220.00 × 1.0150 = 43,407,912.24 COP',
        tone: 'ok',
      },
      paid('194,392.80 MXN', '7890'),
      settled('PAID', 'ok', 'Paid on the discount date: the buyer kept 3,967.20 MXN.'),
    ],
  },
  {
    id: 'price',
    sample: '03-price-variance',
    title: 'Price 8% above the PO',
    tagline: 'Blocked, a human decides',
    outcome: 'BLOCKED → reject or override',
    tone: 'block',
    steps: [
      receive,
      extract('Acme bills 500 steel brackets at $4.32 each.', '"unit_price": "$4.32"'),
      validate('$4.32', '432'),
      geocode('Dallas 75201, US', '(32.7767, -96.7970)'),
      {
        stage: 'match',
        actors: matchActors,
        title: 'PRICE_VARIANCE',
        text: 'The PO says 4.00. The tolerance check is pure integer math: the variance is within tolerance only if |invoice − PO| × 10000 ≤ PO × 200.',
        code: '|432 − 400| × 10000 = 320000  >  400 × 200 = 80000\n→ 8.00% above, beyond the 2.00% tolerance: BLOCK',
        tone: 'block',
      },
      {
        stage: 'decide',
        actors: decideByHuman,
        title: 'Waits for a human',
        text: 'A reviewer rejects with a reason, or overrides with a mandatory reason stored in the audit log. Both are conditional updates: a second click gets 409.',
        code: "UPDATE invoices SET status = 'REJECTED' WHERE id = $1 AND status = ANY($2)",
        tone: 'warn',
      },
      {
        stage: 'decide',
        actors: ['user', 'web', 'httpapi', 'service', 'drafter', 'claude'],
        title: 'Draft an email to the supplier',
        text: 'Claude drafts a short email from the stored findings only. It is labelled as a draft and nothing is sent automatically.',
      },
    ],
  },
  {
    id: 'qty',
    sample: '04-qty-exceeds-received',
    title: 'Billing more than was received',
    tagline: 'Quantity check against goods receipts',
    outcome: 'BLOCKED → override with reason',
    tone: 'block',
    steps: [
      receive,
      extract('Acme bills 80 pneumatic valves.', '"quantity": 80'),
      validate('$5,200.00', '520000'),
      {
        stage: 'match',
        actors: matchActors,
        title: 'QTY_EXCEEDS_RECEIVED',
        text: 'Only 50 valves were received. Already-invoiced quantities from other invoices on the PO are added before comparing, so partial billing works and over-billing does not.',
        code: 'ordered 80, received 50, already invoiced 0, this invoice 80\n→ exceeds the received quantity by 30',
        tone: 'block',
      },
      {
        stage: 'decide',
        actors: decideByHuman,
        title: 'Override with a reason',
        text: 'The reviewer knows the rest arrived and overrides with a mandatory reason. Approval and scheduling happen in the same transaction.',
        code: '{"reason": "Remaining 30 valves arrived; receipt not yet posted in the ERP"}',
        tone: 'warn',
      },
      { stage: 'schedule', actors: scheduleActors, title: 'Scheduled', text: 'Pays on the due date with an FX quote into COP, like any approved invoice.' },
    ],
  },
  {
    id: 'duplicate',
    sample: '05-duplicate-number',
    title: 'Same invoice number, different file',
    tagline: 'Duplicate detection that cannot race',
    outcome: 'BLOCKED → reject only',
    tone: 'block',
    requires: ['01-clean-usd'],
    steps: [
      { ...receive, text: 'It is a different PDF, so its hash is new and a new row is created. File-level dedupe does not catch it.' },
      extract('The supplier re-sends ACM-2026-0412 in a different layout.', '"invoice_number": "ACM-2026-0412"'),
      {
        stage: 'match',
        actors: matchActors,
        title: 'DUPLICATE_INVOICE',
        text: 'The supplier row is locked FOR UPDATE, so two uploads of the same number are serialized. The second one is stored as BLOCKED with duplicate_of pointing at the original, and the partial unique index still guarantees one real invoice per supplier and number.',
        code: 'UNIQUE (supplier_id, invoice_number) WHERE duplicate_of IS NULL',
        tone: 'block',
      },
      {
        stage: 'decide',
        actors: decideByHuman,
        title: 'Override is refused',
        text: 'Overriding a duplicate would pay the supplier twice, so the API answers 409 and the UI only offers Reject.',
        code: '409 INVALID_STATE: a duplicate invoice can only be rejected',
        tone: 'block',
      },
    ],
  },
  {
    id: 'warning',
    sample: '06-price-within-tolerance',
    title: '1% over, then a provider timeout',
    tagline: 'Human approval and idempotent retries',
    outcome: 'Approved → timeout → lookup → PAID',
    tone: 'ok',
    steps: [
      receive,
      extract('Pacific Coast Electronics bills LED drivers at 18.18.', '"unit_price": "18.18"'),
      {
        stage: 'match',
        actors: matchActors,
        title: 'Within tolerance: a warning',
        text: 'The PO price is 18.00, so 1.00% above: inside the 2% tolerance. It is a WARNING, not a block, and the invoice waits for a human.',
        code: '|1818 − 1800| × 10000 = 180000  ≤  1800 × 200 = 360000',
        tone: 'warn',
      },
      { stage: 'decide', actors: decideByHuman, title: 'A human approves', text: 'Approve moves MATCHED_WITH_WARNINGS → APPROVED → PAYMENT_SCHEDULED in one transaction.', tone: 'ok' },
      { stage: 'schedule', actors: scheduleActors, title: 'Scheduled', text: 'Due date, FX quote stored on the payment.', code: '3,118.00 USD × 4,000.00 × 1.0100 = 12,596,720.00 COP' },
      {
        stage: 'execute',
        actors: payActors,
        title: 'The provider hangs',
        text: 'Account 1111 records the payment, then never answers in time. Each call has its own 3s timeout; the executor retries only transient errors, three attempts with backoff, always with the same Idempotency-Key.',
        code: 'attempt 1: timeout · attempt 2: timeout · attempt 3: timeout',
        tone: 'warn',
      },
      {
        stage: 'execute',
        actors: payActors,
        title: 'Ask instead of guessing',
        text: 'After the retries the executor looks the payment up. The provider has it as SUCCEEDED, and the idempotency key means the retries never charged twice.',
        code: 'GET /payments/<payment id>  →  200 SUCCEEDED',
        tone: 'ok',
      },
      settled('PAID', 'ok', 'Settled as PAID. Every attempt and the lookup are in the audit trail.'),
    ],
  },
  {
    id: 'partial',
    sample: '07-eur-partial-receipt',
    title: 'Partial receipts, euros',
    tagline: 'German supplier billing the first delivery',
    outcome: 'Auto-approved → PAID',
    tone: 'ok',
    steps: [
      receive,
      extract('Hanse Präzisionsteile bills 60 shafts and 60 bearing sets.', '"total": "14,070.00"'),
      geocode('Hamburg 20095, DE', '(53.5511, 9.9937)'),
      {
        stage: 'match',
        actors: matchActors,
        title: 'Two receipts, summed',
        text: 'PO-3001 was delivered in two partial receipts (Hamburg, then Munich). Received quantities are summed per SKU, and this invoice only bills what has arrived.',
        code: 'HPT-1: received 60 + 40 = 100 ≥ billed 60  ✓',
        tone: 'ok',
      },
      { stage: 'decide', actors: decideByAgent, title: 'The agent approves', text: 'Clean match, approved automatically.', tone: 'ok' },
      { stage: 'schedule', actors: scheduleActors, title: 'Net 45 and FX', text: 'Pays on the due date.', code: '14,070.00 EUR × 4,350.00 × 1.0120 = 61,938,954.00 COP' },
      paid('14,070.00 EUR', '3000'),
    ],
  },
  {
    id: 'origin',
    sample: '08-origin-mismatch',
    title: 'Shipped from the wrong place',
    tagline: 'PostGIS distance check',
    outcome: 'Warning → human approves',
    tone: 'warn',
    steps: [
      receive,
      extract('Acme bills anchor bolts, shipped from New York.', '"ship_from": { "city": "New York", "postal_code": "10118" }'),
      geocode('New York 10118, US', '(40.7484, -73.9857)'),
      {
        stage: 'match',
        actors: matchActors,
        title: 'ORIGIN_MISMATCH',
        text: 'The goods were received from the Dallas warehouse. Postgres computes the distance with ST_Distance on geography points and passes kilometres to the pure matcher: same country, so a warning, not a block.',
        code: 'ST_Distance(ship_from, warehouse) / 1000 ≈ 2,212 km  >  100 km',
        tone: 'warn',
      },
      { stage: 'decide', actors: decideByHuman, title: 'A human approves', text: 'The reviewer accepts the explanation and approves; the payment is scheduled in the same transaction.', tone: 'ok' },
    ],
  },
  {
    id: 'unknown',
    sample: '09-payment-unknown',
    title: 'The provider never answers',
    tagline: 'UNKNOWN, resolved by the reconciler',
    outcome: 'UNKNOWN → reconciler → FAILED',
    tone: 'block',
    steps: [
      receive,
      { stage: 'match', actors: matchActors, title: 'Clean match', text: 'Lakeshore Packaging, everything matches.', tone: 'ok' },
      { stage: 'decide', actors: decideByAgent, title: 'Auto-approved', text: 'Approved by the agent and scheduled for its due date.', tone: 'ok' },
      {
        stage: 'execute',
        actors: payActors,
        title: 'Timeouts, and the lookup hangs too',
        text: 'Account 2222 never answers and records nothing. After three attempts the lookup also times out, so the system cannot know whether money moved. It says so instead of guessing.',
        code: 'payment → UNKNOWN   invoice → PAYMENT_UNKNOWN',
        tone: 'warn',
      },
      {
        stage: 'reconcile',
        actors: reconcileActors,
        title: 'The reconciler asks again',
        text: 'After the lease, the reconciler claims the UNKNOWN payment with SKIP LOCKED and looks it up again. The provider now answers NOT_FOUND: it was sent but never happened.',
        code: 'GET /payments/<payment id>  →  404 NOT_FOUND  →  FAILED',
        tone: 'block',
      },
    ],
  },
  {
    id: 'declined',
    sample: '10-payment-declined',
    title: 'The bank declines',
    tagline: 'A decline is final, never retried',
    outcome: 'Declined → FAILED',
    tone: 'block',
    steps: [
      receive,
      { stage: 'match', actors: matchActors, title: 'Clean match', text: 'Gulf Marine Parts, everything matches.', tone: 'ok' },
      { stage: 'decide', actors: decideByAgent, title: 'Auto-approved', text: 'Approved and scheduled.', tone: 'ok' },
      {
        stage: 'execute',
        actors: payActors,
        title: '402 DECLINED',
        text: 'Account 0000 is declined by the destination bank. Only transient errors are retried; a decline is an answer, so the payment is FAILED immediately and a human follows up.',
        code: 'POST /payments  →  402 PAYMENT_DECLINED',
        tone: 'block',
      },
      settled('FAILED', 'block', 'Payment FAILED, invoice PAYMENT_FAILED, with the reason in the audit log.'),
    ],
  },
]
