<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { api, errorMessage } from '../api'
import StatusBadge from '../components/StatusBadge.vue'
import { money } from '../format'
import { clock } from '../session'
import type { PaymentSummary } from '../types'

const payments = ref<PaymentSummary[]>([])
const days = ref(1)
const busy = ref<'' | 'run' | 'advance' | 'reset'>('')
const confirmingReset = ref(false)
const error = ref('')
const pollError = ref('')
const notice = ref('')
let timer = 0

async function load() {
  try {
    payments.value = await api.payments()
    pollError.value = ''
  } catch (e) {
    pollError.value = errorMessage(e)
  }
}

function poll() {
  if (busy.value === '') void load()
}

async function run(kind: 'run' | 'advance' | 'reset', work: () => Promise<void>) {
  busy.value = kind
  error.value = ''
  notice.value = ''
  try {
    await work()
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = ''
  }
}

function runDue() {
  void run('run', async () => {
    payments.value = await api.runDue()
    notice.value = 'Due payments processed.'
  })
}

function advance() {
  void run('advance', async () => {
    clock.value = await api.advanceDays(days.value)
    notice.value = `Moved the demo clock to ${clock.value.today}. Due payments run automatically, or click Run due payments.`
  })
}

function reset() {
  if (!confirmingReset.value) {
    confirmingReset.value = true
    return
  }
  confirmingReset.value = false
  void run('reset', async () => {
    clock.value = await api.resetDemo()
    await load()
    notice.value = 'Demo reset: invoices, payments and audit log cleared, clock back to today.'
  })
}

onMounted(() => {
  void load()
  timer = window.setInterval(poll, 3000)
})
onUnmounted(() => window.clearInterval(timer))
</script>

<template>
  <section class="card controls">
    <div>
      <h2>Demo controls</h2>
      <p class="muted small">Today on the demo clock: <strong>{{ clock?.today ?? '…' }}</strong> (+{{ clock?.offsetDays ?? 0 }} days)</p>
    </div>
    <div class="row">
      <button type="button" class="primary" :disabled="busy !== ''" @click="runDue">
        {{ busy === 'run' ? 'Paying… (slow accounts take ~15s)' : 'Run due payments' }}
      </button>
      <input v-model.number="days" type="number" min="1" max="365" class="days" :disabled="busy !== ''" />
      <button type="button" :disabled="busy !== '' || days < 1" @click="advance">Fast-forward {{ days }} day{{ days === 1 ? '' : 's' }}</button>
      <button type="button" class="danger" :disabled="busy !== ''" @click="reset" @blur="confirmingReset = false">
        {{ confirmingReset ? 'Click again to confirm' : 'Reset demo' }}
      </button>
    </div>
  </section>

  <p v-if="error" class="error-banner">{{ error }}</p>
  <p v-if="pollError" class="error-banner">Could not refresh the list: {{ pollError }}</p>
  <p v-if="notice" class="notice-banner">{{ notice }}</p>

  <section class="card">
    <h2>Payments</h2>
    <table v-if="payments.length > 0" class="list">
      <thead>
        <tr>
          <th>Invoice</th>
          <th>Supplier</th>
          <th class="num">Supplier receives</th>
          <th class="num">You pay</th>
          <th>Scheduled</th>
          <th>Status</th>
          <th>Details</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="p in payments" :key="p.id">
          <td class="nowrap"><RouterLink :to="`/invoices/${p.invoiceId}`">{{ p.invoiceNumber }}</RouterLink></td>
          <td>{{ p.supplierName }} <span class="muted small">···{{ p.destinationLast4 }}</span></td>
          <td class="num">
            {{ money(p.amountMinor, p.currency) }}
            <div v-if="p.discountMinor > 0" class="good small">saved {{ money(p.discountMinor, p.currency) }}</div>
          </td>
          <td class="num">{{ money(p.fundingAmountMinor, p.fundingCurrency) }}</td>
          <td class="nowrap">{{ p.scheduledFor }}</td>
          <td><StatusBadge :status="p.status" /></td>
          <td class="small" :class="{ bad: p.error }">
            <div class="details" :title="p.error || p.processorPaymentId">{{ p.error || p.processorPaymentId || '' }}</div>
          </td>
        </tr>
      </tbody>
    </table>
    <p v-else class="muted">No payments yet. Approved invoices get a payment scheduled here.</p>
  </section>
</template>
