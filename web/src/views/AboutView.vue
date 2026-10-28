<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { components, stages, type ComponentId } from '../about/model'
import { scenarios, type Scenario } from '../about/scenarios'
import { api, errorMessage } from '../api'
import ArchitectureMap from '../components/ArchitectureMap.vue'
import PipelineTrack from '../components/PipelineTrack.vue'

const router = useRouter()
const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches

const scenario = ref<Scenario>(scenarios[0]!)
const stepIndex = ref(0)
const playing = ref(!reducedMotion)
const selected = ref<ComponentId | null>(null)
const running = ref(false)
const runError = ref('')
let timer = 0

const step = computed(() => scenario.value.steps[stepIndex.value]!)
const isLast = computed(() => stepIndex.value === scenario.value.steps.length - 1)
const tone = computed(() => step.value.tone ?? 'info')
const selectedComponent = computed(() => components.find((c) => c.id === selected.value) ?? null)

function resetTimer() {
  window.clearInterval(timer)
  timer = window.setInterval(() => {
    if (playing.value) advance()
  }, 4200)
}

function choose(s: Scenario) {
  scenario.value = s
  stepIndex.value = 0
  playing.value = !reducedMotion
  runError.value = ''
  resetTimer()
}

function advance() {
  if (!isLast.value) stepIndex.value++
  else playing.value = false
}

function stepForward() {
  playing.value = false
  advance()
}

function stepBack() {
  playing.value = false
  if (stepIndex.value > 0) stepIndex.value--
}

function toggle() {
  if (isLast.value && !playing.value) {
    stepIndex.value = 0
    playing.value = !reducedMotion
  } else {
    playing.value = !playing.value
  }
  resetTimer()
}

function onKey(event: KeyboardEvent) {
  const target = event.target as HTMLElement | null
  if (target?.closest('input, textarea, select, button, a, [role="button"]')) return
  if (event.key === 'ArrowRight') stepForward()
  else if (event.key === 'ArrowLeft') stepBack()
  else if (event.key === ' ') {
    event.preventDefault()
    toggle()
  }
}

async function runForReal() {
  running.value = true
  runError.value = ''
  try {
    for (const sample of scenario.value.requires ?? []) {
      await api.processSample(sample)
    }
    const invoice = await api.processSample(scenario.value.sample)
    await router.push(`/invoices/${invoice.id}`)
  } catch (e) {
    runError.value = errorMessage(e)
  } finally {
    running.value = false
  }
}

onMounted(() => {
  resetTimer()
  window.addEventListener('keydown', onKey)
})
onUnmounted(() => {
  window.clearInterval(timer)
  window.removeEventListener('keydown', onKey)
})

const guarantees = [
  {
    title: 'Money never touches a float',
    text: 'Amounts are int64 minor units, rates are int64 × 10⁸, percentages are basis points. The FX product is computed with math/big and rounded half up once.',
    code: '"1,250.00" → 125000',
  },
  {
    title: 'The LLM only reads',
    text: 'Claude extracts fields and drafts emails. Matching, approval, scheduling, FX and payment are deterministic Go with table tests.',
    code: 'tool: record_invoice (strict)',
  },
  {
    title: 'Idempotent by design',
    text: 'Same file → same invoice. Same payment → same Idempotency-Key. Approve twice → 409. Retries can never pay twice.',
    code: 'ON CONFLICT (file_sha256) DO NOTHING',
  },
  {
    title: 'Short transactions, visible locks',
    text: 'No transaction is open during a call to Claude, the geocoder or the provider. Locks are taken in one order: invoice → supplier → PO.',
    code: 'SELECT ... FOR UPDATE SKIP LOCKED',
  },
  {
    title: 'Ask, never guess',
    text: 'When a payment times out, the executor looks it up. If the provider is unreachable it says UNKNOWN, and the reconciler asks again later.',
    code: 'UNKNOWN → lookup → PAID | FAILED',
  },
  {
    title: 'Duplicates cannot race',
    text: 'The supplier row lock serializes duplicate checks, and a partial unique index keeps one real invoice per supplier and number.',
    code: 'WHERE duplicate_of IS NULL',
  },
]
</script>

<template>
  <section class="hero card">
    <div>
      <p class="eyebrow">How it works</p>
      <h1>An accounts-payable agent for cross-border supplier invoices</h1>
      <p class="lead">
        A finance team uploads a supplier invoice. The agent reads it, checks it against the purchase order and the goods
        receipts, flags anything wrong with a clear reason, and pays approved invoices on the best date with an FX quote into
        pesos.
      </p>
    </div>
    <div class="pillars">
      <div><strong>Claude reads</strong><span>PDF in any layout, English or Spanish, amounts copied as printed</span></div>
      <div><strong>Go decides</strong><span>3-way match, approval, schedule, FX and payment, all deterministic</span></div>
      <div><strong>Postgres remembers</strong><span>Every step in a short transaction with an audit trail</span></div>
    </div>
  </section>

  <section class="card player">
    <div class="player-head">
      <h2>Watch a scenario</h2>
      <span class="muted small">Pick a sample invoice · ← → to step · space to play or pause</span>
    </div>
    <div class="scenarios">
      <button
        v-for="s in scenarios"
        :key="s.id"
        type="button"
        class="scenario"
        :class="{ current: s.id === scenario.id }"
        @click="choose(s)"
      >
        <span class="scenario-title">{{ s.title }}</span>
        <span class="scenario-tag">{{ s.tagline }}</span>
      </button>
    </div>

    <PipelineTrack :stages="stages" :current="step.stage" :tone="tone" />

    <div class="stage-grid">
      <div class="step-card" :class="tone">
        <div class="step-meta">
          <span class="badge" :class="scenario.tone === 'ok' ? 'green' : scenario.tone === 'warn' ? 'amber' : 'red'">
            {{ scenario.outcome }}
          </span>
          <span class="muted small">step {{ stepIndex + 1 }} of {{ scenario.steps.length }}</span>
        </div>
        <div :key="`${scenario.id}-${stepIndex}`" class="step-body">
          <h3>{{ step.title }}</h3>
          <p>{{ step.text }}</p>
          <pre v-if="step.code" class="code">{{ step.code }}</pre>
        </div>
        <div class="controls">
          <button type="button" :disabled="stepIndex === 0" aria-label="Previous step" @click="stepBack">‹</button>
          <button type="button" class="primary" @click="toggle">
            {{ playing ? 'Pause' : isLast ? 'Replay' : 'Play' }}
          </button>
          <button type="button" :disabled="isLast" aria-label="Next step" @click="stepForward">›</button>
          <span class="spacer" />
          <button type="button" :disabled="running" @click="runForReal">
            {{ running ? 'Processing…' : 'Run this sample for real →' }}
          </button>
        </div>
        <p v-if="runError" class="bad small">{{ runError }}</p>
      </div>

      <div class="map-card">
        <ArchitectureMap :active="step.actors" :selected="selected" @select="selected = $event" />
        <div class="component-detail">
          <template v-if="selectedComponent">
            <strong>{{ selectedComponent.label }}</strong>
            <code class="where">{{ selectedComponent.where }}</code>
            <p>{{ selectedComponent.what }}</p>
          </template>
          <p v-else class="muted small">Highlighted boxes take part in the current step. Click any box to see what it does.</p>
        </div>
      </div>
    </div>
  </section>

  <section>
    <h2 class="section-title">What the design guarantees</h2>
    <div class="guarantees">
      <article v-for="g in guarantees" :key="g.title" class="card">
        <h3>{{ g.title }}</h3>
        <p>{{ g.text }}</p>
        <code>{{ g.code }}</code>
      </article>
    </div>
  </section>
</template>

<style scoped>
.hero {
  display: grid;
  grid-template-columns: 3fr 2fr;
  gap: 24px;
  align-items: center;
  background: linear-gradient(135deg, var(--card) 0%, var(--accent-soft) 100%);
}

.eyebrow {
  margin: 0 0 6px;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--accent);
}

.hero h1 {
  font-size: 24px;
  line-height: 1.25;
}

.lead {
  margin: 10px 0 0;
  color: var(--muted);
  font-size: 15px;
}

.pillars {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.pillars div {
  display: flex;
  flex-direction: column;
  padding: 10px 14px;
  border-radius: 8px;
  background: var(--card);
  border: 1px solid var(--border);
}

.pillars span {
  font-size: 12px;
  color: var(--muted);
}

.player-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.scenarios {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 8px;
  margin-bottom: 8px;
}

.scenario {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  padding: 8px 10px;
  text-align: left;
  border-radius: 8px;
  transition:
    border-color 0.2s,
    background 0.2s;
}

.scenario:hover {
  border-color: var(--accent);
}

.scenario.current {
  border-color: var(--accent);
  background: var(--accent-soft);
}

.scenario-title {
  font-weight: 600;
  font-size: 13px;
}

.scenario-tag {
  font-size: 11px;
  color: var(--muted);
}

.stage-grid {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(0, 3fr);
  gap: 16px;
  margin-top: 16px;
}

.step-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 16px;
  border-radius: 10px;
  border: 1px solid var(--border);
  border-top: 4px solid var(--accent);
  min-height: 320px;
  transition: border-color 0.4s;
}

.step-card.ok {
  border-top-color: var(--green);
}

.step-card.warn {
  border-top-color: var(--amber);
}

.step-card.block {
  border-top-color: var(--red);
}

.step-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.step-body {
  flex: 1;
  animation: rise 0.35s ease-out;
}

.step-body h3 {
  font-size: 17px;
  margin: 4px 0 8px;
}

.step-body p {
  margin: 0 0 10px;
}

.code {
  margin: 0;
  padding: 10px 12px;
  border-radius: 6px;
  background: var(--code-bg);
  color: var(--code-text);
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-word;
}

.controls {
  display: flex;
  align-items: center;
  gap: 8px;
}

.spacer {
  flex: 1;
}

.map-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.component-detail {
  min-height: 72px;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--bg);
}

.component-detail p {
  margin: 4px 0 0;
}

.where {
  margin-left: 8px;
  font-size: 12px;
  color: var(--muted);
}

.section-title {
  margin: 8px 0 12px;
}

.guarantees {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}

.guarantees h3 {
  font-size: 15px;
  margin: 0 0 6px;
}

.guarantees p {
  margin: 0 0 10px;
  color: var(--muted);
}

.guarantees code {
  font-size: 12px;
  padding: 3px 8px;
  border-radius: 6px;
  background: var(--accent-soft);
  color: var(--accent);
}

@keyframes rise {
  from {
    opacity: 0;
    transform: translateY(6px);
  }
}

@media (max-width: 1100px) {
  .hero,
  .stage-grid {
    grid-template-columns: 1fr;
  }

  .scenarios {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .guarantees {
    grid-template-columns: 1fr;
  }
}

@media (prefers-reduced-motion: reduce) {
  .step-body {
    animation: none;
  }
}
</style>
