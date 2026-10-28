<script setup lang="ts">
import { money, percent, rate } from '../format'
import type { Payment } from '../types'
import StatusBadge from './StatusBadge.vue'

defineProps<{ payment: Payment }>()
</script>

<template>
  <section class="card">
    <h2>Payment <StatusBadge :status="payment.status" /></h2>
    <dl class="fields">
      <dt>Scheduled for</dt>
      <dd>{{ payment.scheduledFor }}</dd>
      <dt v-if="payment.discountMinor > 0">Discount saved</dt>
      <dd v-if="payment.discountMinor > 0" class="good">{{ money(payment.discountMinor, payment.currency) }}</dd>
      <dt>Provider reference</dt>
      <dd>{{ payment.processorPaymentId || '—' }}</dd>
      <dt v-if="payment.error">Last error</dt>
      <dd v-if="payment.error" class="bad">{{ payment.error }}</dd>
    </dl>
    <p class="fx-quote">
      Supplier receives <strong>{{ money(payment.amountMinor, payment.currency) }}</strong>, you pay
      <strong>{{ money(payment.fundingAmountMinor, payment.fundingCurrency) }}</strong>
      <template v-if="payment.currency !== payment.fundingCurrency">
        at rate {{ rate(payment.fxRateE8) }} {{ payment.fundingCurrency }} per {{ payment.currency }} plus a
        {{ percent(payment.fxSpreadBps) }} spread.
      </template>
    </p>
  </section>
</template>
