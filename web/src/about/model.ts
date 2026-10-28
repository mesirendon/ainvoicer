export type StageId = 'upload' | 'extract' | 'validate' | 'geocode' | 'match' | 'decide' | 'schedule' | 'execute' | 'reconcile'

export type ComponentId =
  | 'user'
  | 'web'
  | 'httpapi'
  | 'service'
  | 'extractor'
  | 'claude'
  | 'geocoder'
  | 'drafter'
  | 'rules'
  | 'repo'
  | 'postgres'
  | 'executor'
  | 'reconciler'
  | 'gateway'
  | 'mock'

export type Tone = 'info' | 'ok' | 'warn' | 'block'

export interface Stage {
  id: StageId
  label: string
  icon: string
}

export interface ArchComponent {
  id: ComponentId
  label: string
  sub: string
  x: number
  y: number
  kind: 'actor' | 'app' | 'rules' | 'data' | 'external' | 'job'
  where: string
  what: string
}

export const stages: Stage[] = [
  { id: 'upload', label: 'Upload', icon: '⇪' },
  { id: 'extract', label: 'Extract', icon: '✦' },
  { id: 'validate', label: 'Validate', icon: '✓' },
  { id: 'geocode', label: 'Geocode', icon: '⌖' },
  { id: 'match', label: '3-way match', icon: '⇄' },
  { id: 'decide', label: 'Decide', icon: '⚖' },
  { id: 'schedule', label: 'Schedule + FX', icon: '⏱' },
  { id: 'execute', label: 'Pay', icon: '➜' },
  { id: 'reconcile', label: 'Settle', icon: '◎' },
]

export const components: ArchComponent[] = [
  { id: 'user', label: 'Finance user', sub: 'browser', x: 20, y: 40, kind: 'actor', where: 'web/src/views', what: 'Uploads supplier invoices, reviews findings, approves, rejects or overrides with a reason, and runs the demo clock.' },
  { id: 'web', label: 'Web', sub: 'Vue 3 · nginx', x: 190, y: 40, kind: 'app', where: 'web/', what: 'Three working views plus this page. Typed fetch wrapper, amounts formatted for display only. nginx serves the bundle and proxies /api.' },
  { id: 'httpapi', label: 'HTTP API', sub: 'Gin handlers', x: 360, y: 40, kind: 'app', where: 'internal/httpapi', what: 'RequestID → AccessLog → Recover middleware, one error JSON shape, size cap and PDF check, status code mapping (404, 409, 422, 502/504, 503 DATABASE_BUSY).' },
  { id: 'service', label: 'Invoice service', sub: 'orchestration', x: 530, y: 40, kind: 'app', where: 'internal/invoices', what: 'Reads like a recipe: hash → receive → extract → geocode → verdict. External calls happen between short transactions, detached from the HTTP request.' },
  { id: 'extractor', label: 'Extractor', sub: 'Claude or fixtures', x: 700, y: 40, kind: 'app', where: 'internal/extraction', what: 'Sends the PDF with a strict record_invoice tool. Amounts come back as printed text and Go parses them with money.ParseMinor. One retry with the validation errors fed back.' },
  { id: 'claude', label: 'Claude API', sub: 'reads documents', x: 860, y: 40, kind: 'external', where: 'anthropic-sdk-go', what: 'The only LLM in the system. It reads invoices and drafts supplier emails. It never decides anything that moves money.' },
  { id: 'geocoder', label: 'Geocoder', sub: 'lookup table', x: 680, y: 260, kind: 'app', where: 'internal/geocode', what: 'Postal code, then city, then a deterministic hash inside the country box. A stand-in for a real geocoding provider.' },
  { id: 'drafter', label: 'Email drafter', sub: 'Claude or template', x: 700, y: 150, kind: 'app', where: 'internal/emaildraft', what: 'Drafts a short email to the supplier from the stored blocking findings only. Spanish for Spanish-speaking suppliers; a deterministic template in fixture mode. Never stored or sent.' },
  { id: 'rules', label: 'Rules', sub: 'match · schedule · FX', x: 360, y: 150, kind: 'rules', where: 'internal/matching, payments, fx, money', what: 'Pure functions with table tests: 3-way match, discount windows and due dates, big.Int FX quote rounded half up once. No I/O, no floats.' },
  { id: 'repo', label: 'Repository', sub: 'all SQL', x: 530, y: 260, kind: 'data', where: 'internal/repo', what: 'One method per transaction. Takes the locks (invoice → supplier → PO), loads the facts, calls the pure rules, writes results and the audit log.' },
  { id: 'postgres', label: 'Postgres', sub: 'PostGIS container', x: 190, y: 260, kind: 'data', where: 'migrations/', what: 'bigint money, rate_e8, basis points, geography(Point) with a GiST index, ST_Distance for origin checks, partial unique index for duplicates.' },
  { id: 'executor', label: 'Executor', sub: 'every 5s', x: 360, y: 360, kind: 'job', where: 'internal/payments', what: 'Claims one due payment with FOR UPDATE SKIP LOCKED, pays outside any transaction (3 attempts, 3s each, never retries a decline), looks up anything unresolved, then settles.' },
  { id: 'reconciler', label: 'Reconciler', sub: 'every 10s', x: 530, y: 470, kind: 'job', where: 'internal/payments', what: 'Resolves UNKNOWN and stale PROCESSING payments by asking the provider. NOT_FOUND after UNKNOWN means FAILED; after a crash it means safe to retry.' },
  { id: 'gateway', label: 'Gateway', sub: 'HTTP client', x: 700, y: 360, kind: 'app', where: 'internal/payments/http_gateway.go', what: 'Maps provider answers: 200 succeeded, 402 declined, 5xx or network transient. Sends the payment id as the Idempotency-Key.' },
  { id: 'mock', label: 'Payment provider', sub: 'mock service', x: 860, y: 360, kind: 'external', where: 'cmd/payment-mock', what: 'Separate service with its own schema. Account prefix drives behaviour: 4242 approves, 0000 declines, 1111 records then hangs, 2222 hangs without recording.' },
]

export const edges: [ComponentId, ComponentId][] = [
  ['user', 'web'],
  ['web', 'httpapi'],
  ['httpapi', 'service'],
  ['service', 'extractor'],
  ['extractor', 'claude'],
  ['service', 'geocoder'],
  ['service', 'drafter'],
  ['drafter', 'claude'],
  ['service', 'repo'],
  ['repo', 'rules'],
  ['repo', 'postgres'],
  ['executor', 'repo'],
  ['reconciler', 'repo'],
  ['executor', 'gateway'],
  ['reconciler', 'gateway'],
  ['gateway', 'mock'],
]

export const boxWidth = 130
export const boxHeight = 58
