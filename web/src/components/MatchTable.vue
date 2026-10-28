<script setup lang="ts">
import { computed } from 'vue'
import { money } from '../format'
import type { Finding, InvoiceDetail } from '../types'

const props = defineProps<{ invoice: InvoiceDetail }>()

interface Row {
  lineNo: number
  sku: string
  description: string
  poQuantity: number | null
  poPrice: number | null
  received: number | null
  quantity: number
  price: number
  rowTone: string
  priceTone: string
  quantityTone: string
}

function severityTone(findings: Finding[]): string {
  if (findings.some((f) => f.severity === 'BLOCK')) return 'cell-red'
  if (findings.length > 0) return 'cell-amber'
  return ''
}

const rows = computed<Row[]>(() => {
  const po = props.invoice.purchaseOrder
  return props.invoice.lines.map((line) => {
    const poLine = po?.lines.find((p) => p.lineNo === line.poLineNo) ?? null
    const received = poLine
      ? (po?.receipts ?? []).flatMap((r) => r.lines).filter((r) => r.sku === poLine.sku).reduce((sum, r) => sum + r.quantity, 0)
      : null
    const lineFindings = props.invoice.findings.filter((f) => f.lineNo === line.lineNo)
    return {
      lineNo: line.lineNo,
      sku: line.sku || poLine?.sku || '',
      description: line.description,
      poQuantity: poLine?.quantity ?? null,
      poPrice: poLine?.unitPriceMinor ?? null,
      received,
      quantity: line.quantity,
      price: line.unitPriceMinor,
      rowTone: severityTone(lineFindings.filter((f) => f.code === 'LINE_NOT_ON_PO')),
      priceTone: severityTone(lineFindings.filter((f) => f.code.startsWith('PRICE_'))),
      quantityTone: severityTone(lineFindings.filter((f) => f.code === 'QTY_EXCEEDS_RECEIVED')),
    }
  })
})
</script>

<template>
  <table class="match-table">
    <thead>
      <tr>
        <th>#</th>
        <th>Item</th>
        <th class="num">PO qty</th>
        <th class="num">PO price</th>
        <th class="num">Received</th>
        <th class="num">Invoiced qty</th>
        <th class="num">Invoiced price</th>
      </tr>
    </thead>
    <tbody>
      <tr v-for="row in rows" :key="row.lineNo" :class="row.rowTone">
        <td>{{ row.lineNo }}</td>
        <td>
          <div>{{ row.description }}</div>
          <div class="muted small">{{ row.sku }}</div>
        </td>
        <td class="num">{{ row.poQuantity ?? '—' }}</td>
        <td class="num">{{ row.poPrice === null ? '—' : money(row.poPrice, invoice.purchaseOrder?.currency ?? invoice.currency) }}</td>
        <td class="num" :class="row.quantityTone">{{ row.received ?? '—' }}</td>
        <td class="num" :class="row.quantityTone">{{ row.quantity }}</td>
        <td class="num" :class="row.priceTone">{{ money(row.price, invoice.currency) }}</td>
      </tr>
    </tbody>
  </table>
  <p v-if="invoice.purchaseOrder" class="muted small">
    {{ invoice.purchaseOrder.number }} · receipts:
    <span v-for="(r, i) in invoice.purchaseOrder.receipts" :key="r.id">
      {{ i > 0 ? '; ' : '' }}{{ r.receivedOn }} at {{ r.warehouseName }} ({{ r.warehouseCity }})
    </span>
  </p>
</template>
