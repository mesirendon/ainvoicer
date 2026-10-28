<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, ApiError, errorMessage } from '../api'
import StatusBadge from '../components/StatusBadge.vue'
import { dateTime, money } from '../format'
import type { InvoiceDetail, InvoiceSummary, Sample } from '../types'

const router = useRouter()
const invoices = ref<InvoiceSummary[]>([])
const samples = ref<Sample[]>([])
const selectedSample = ref('')
const busy = ref(false)
const dragging = ref(false)
const error = ref('')
const pollError = ref('')
let timer = 0

async function load() {
  try {
    invoices.value = await api.invoices()
    pollError.value = ''
  } catch (e) {
    pollError.value = errorMessage(e)
  }
}

async function submit(work: () => Promise<InvoiceDetail>) {
  busy.value = true
  error.value = ''
  try {
    const invoice = await work()
    await router.push(`/invoices/${invoice.id}`)
  } catch (e) {
    const id = e instanceof ApiError ? e.details['invoiceId'] : undefined
    if (typeof id === 'string') {
      await router.push(`/invoices/${id}`)
      return
    }
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

function uploadFiles(files: FileList | null | undefined) {
  const file = files?.[0]
  if (file && !busy.value) void submit(() => api.upload(file))
}

function onPick(event: Event) {
  const input = event.target as HTMLInputElement
  uploadFiles(input.files)
  input.value = ''
}

function onDrop(event: DragEvent) {
  dragging.value = false
  uploadFiles(event.dataTransfer?.files)
}

function processSample() {
  if (selectedSample.value) void submit(() => api.processSample(selectedSample.value))
}

onMounted(async () => {
  timer = window.setInterval(load, 5000)
  await load()
  try {
    samples.value = await api.samples()
    selectedSample.value = samples.value[0]?.name ?? ''
  } catch (e) {
    error.value = errorMessage(e)
  }
})
onUnmounted(() => window.clearInterval(timer))
</script>

<template>
  <div class="grid-2">
    <label
      class="card dropzone"
      :class="{ dragging, busy }"
      @dragover.prevent="dragging = true"
      @dragleave="dragging = false"
      @drop.prevent="onDrop"
    >
      <input type="file" accept="application/pdf" hidden :disabled="busy" @change="onPick" />
      <strong>{{ busy ? 'Reading the invoice…' : 'Upload invoice' }}</strong>
      <span class="muted">{{ busy ? 'Extracting, matching and deciding' : 'Drop a PDF here or click to choose one' }}</span>
      <span v-if="busy" class="spinner" />
    </label>
    <section class="card">
      <h2>Try a sample</h2>
      <p class="muted small">Committed PDFs covering every scenario. They go through the same path as an upload.</p>
      <div class="row">
        <select v-model="selectedSample" :disabled="busy">
          <option v-for="s in samples" :key="s.name" :value="s.name">{{ s.name }}</option>
        </select>
        <button type="button" class="primary" :disabled="busy || !selectedSample" @click="processSample">Process</button>
      </div>
    </section>
  </div>

  <p v-if="error" class="error-banner">{{ error }}</p>
  <p v-if="pollError" class="error-banner">Could not refresh the list: {{ pollError }}</p>

  <section class="card">
    <h2>Invoices</h2>
    <table v-if="invoices.length > 0" class="list">
      <thead>
        <tr>
          <th>Supplier</th>
          <th>Invoice</th>
          <th class="num">Amount</th>
          <th>Status</th>
          <th>Received</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="inv in invoices" :key="inv.id" class="clickable" @click="router.push(`/invoices/${inv.id}`)">
          <td>{{ inv.supplierName || inv.fileName }}</td>
          <td>{{ inv.invoiceNumber || '—' }}</td>
          <td class="num">{{ inv.currency ? money(inv.totalMinor, inv.currency) : '—' }}</td>
          <td><StatusBadge :status="inv.status" /></td>
          <td class="muted">{{ dateTime(inv.createdAt) }}</td>
        </tr>
      </tbody>
    </table>
    <p v-else class="muted">No invoices yet. Upload one or try a sample.</p>
  </section>
</template>
