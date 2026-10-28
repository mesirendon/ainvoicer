<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { api, errorMessage } from '../api'
import AuditTimeline from '../components/AuditTimeline.vue'
import MatchTable from '../components/MatchTable.vue'
import PaymentCard from '../components/PaymentCard.vue'
import ReasonModal from '../components/ReasonModal.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { humanize, money } from '../format'
import type { InvoiceDetail } from '../types'

const route = useRoute()
const invoice = ref<InvoiceDetail | null>(null)
const error = ref('')
const busy = ref(false)
const modal = ref<'override' | 'reject' | null>(null)
let timer = 0

const id = computed(() => String(route.params['id']))
const moving = ['RECEIVED', 'PAYMENT_SCHEDULED', 'PAYMENT_UNKNOWN']

const canApprove = computed(() => ['MATCHED', 'MATCHED_WITH_WARNINGS'].includes(invoice.value?.status ?? ''))
const canOverride = computed(() => invoice.value?.status === 'BLOCKED' && !invoice.value.duplicateOf)
const canReject = computed(() =>
  ['MATCHED', 'MATCHED_WITH_WARNINGS', 'BLOCKED', 'EXTRACTION_FAILED'].includes(invoice.value?.status ?? ''),
)

async function load() {
  const requested = id.value
  try {
    const loaded = await api.invoice(requested)
    if (requested === id.value) invoice.value = loaded
  } catch (e) {
    if (requested === id.value) error.value = errorMessage(e)
  }
}

async function act(work: () => Promise<InvoiceDetail>) {
  busy.value = true
  try {
    invoice.value = await work()
    modal.value = null
    error.value = ''
  } catch (e) {
    const message = errorMessage(e)
    modal.value = null
    await load()
    error.value = message
  } finally {
    busy.value = false
  }
}

function approve() {
  void act(() => api.approve(id.value))
}

function confirmReason(reason: string) {
  const action = modal.value
  if (action === 'override') void act(() => api.override(id.value, reason))
  if (action === 'reject') void act(() => api.reject(id.value, reason))
}

watch(id, () => {
  invoice.value = null
  modal.value = null
  error.value = ''
  void load()
})
onMounted(() => {
  void load()
  timer = window.setInterval(() => {
    if (moving.includes(invoice.value?.status ?? '')) void load()
  }, 4000)
})
onUnmounted(() => window.clearInterval(timer))
</script>

<template>
  <p><RouterLink to="/">← Invoices</RouterLink></p>
  <p v-if="error" class="error-banner">{{ error }}</p>

  <template v-if="invoice">
    <div class="detail-header">
      <h1>{{ invoice.supplierName || invoice.fileName }} · {{ invoice.invoiceNumber || 'no number' }}</h1>
      <StatusBadge :status="invoice.status" />
    </div>

    <div class="detail">
      <div class="column">
        <iframe class="pdf" :src="api.invoiceFileUrl(invoice.id)" :title="invoice.fileName" />
        <section class="card">
          <h2>Extracted fields</h2>
          <p v-if="invoice.extractionError" class="bad">{{ invoice.extractionError }}</p>
          <dl class="fields">
            <dt>Supplier</dt>
            <dd>{{ invoice.supplierName || '—' }} <span v-if="!invoice.supplierId && invoice.supplierName" class="muted">(not in master data)</span></dd>
            <dt>Invoice number</dt>
            <dd>{{ invoice.invoiceNumber || '—' }}</dd>
            <dt>Purchase order</dt>
            <dd>{{ invoice.poNumber || '—' }}</dd>
            <dt>Issue / due</dt>
            <dd>{{ invoice.issueDate || '—' }} / {{ invoice.dueDate || '—' }}</dd>
            <dt>Ship from</dt>
            <dd>{{ [invoice.shipFromCity, invoice.shipFromCountry].filter(Boolean).join(', ') || '—' }}</dd>
            <template v-if="invoice.currency">
              <dt>Subtotal</dt>
              <dd>{{ money(invoice.subtotalMinor, invoice.currency) }}</dd>
              <dt>Tax</dt>
              <dd>{{ money(invoice.taxMinor, invoice.currency) }}</dd>
              <dt>Total</dt>
              <dd><strong>{{ money(invoice.totalMinor, invoice.currency) }}</strong></dd>
            </template>
            <template v-if="invoice.duplicateOf">
              <dt>Duplicate of</dt>
              <dd><RouterLink :to="`/invoices/${invoice.duplicateOf}`">original invoice</RouterLink></dd>
            </template>
          </dl>
        </section>
      </div>

      <div class="column">
        <section v-if="canApprove || canOverride || canReject" class="card actions">
          <span class="muted">Waiting for a decision</span>
          <button v-if="canApprove" type="button" class="primary" :disabled="busy" @click="approve">Approve</button>
          <button v-if="canOverride" type="button" :disabled="busy" @click="modal = 'override'">Override…</button>
          <button v-if="canReject" type="button" class="danger" :disabled="busy" @click="modal = 'reject'">Reject…</button>
        </section>

        <section class="card">
          <h2>Findings</h2>
          <ul v-if="invoice.findings.length > 0" class="findings">
            <li v-for="f in invoice.findings" :key="f.id" :class="f.severity === 'BLOCK' ? 'finding-block' : 'finding-warning'">
              <strong>{{ humanize(f.code) }}</strong>
              <span v-if="f.lineNo" class="muted"> · line {{ f.lineNo }}</span>
              <div>{{ f.message }}</div>
            </li>
          </ul>
          <p v-else-if="invoice.status !== 'RECEIVED' && invoice.status !== 'EXTRACTION_FAILED'" class="good">
            Clean 3-way match: invoice, purchase order and goods receipts agree.
          </p>
        </section>

        <section v-if="invoice.lines.length > 0" class="card">
          <h2>3-way match</h2>
          <MatchTable :invoice="invoice" />
        </section>

        <PaymentCard v-if="invoice.payment" :payment="invoice.payment" />

        <section class="card">
          <h2>Audit trail</h2>
          <AuditTimeline :entries="invoice.audit" />
        </section>
      </div>
    </div>

    <ReasonModal
      v-if="modal"
      :title="modal === 'override' ? 'Override the block and approve' : 'Reject the invoice'"
      :confirm-label="modal === 'override' ? 'Override and approve' : 'Reject'"
      :busy="busy"
      @confirm="confirmReason"
      @cancel="modal = null"
    />
  </template>
</template>
