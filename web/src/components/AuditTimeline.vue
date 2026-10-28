<script setup lang="ts">
import { dateTime, humanize } from '../format'
import type { AuditEntry } from '../types'

defineProps<{ entries: AuditEntry[] }>()

function summary(detail: Record<string, unknown>): string {
  for (const key of ['reason', 'error', 'verdict']) {
    const value = detail[key]
    if (typeof value === 'string' && value !== '') return value
  }
  if (typeof detail['scheduledFor'] === 'string') return `for ${detail['scheduledFor']}`
  return ''
}
</script>

<template>
  <ol class="timeline">
    <li v-for="(e, i) in entries" :key="i">
      <span class="muted small">{{ dateTime(e.createdAt) }}</span>
      <div>
        <strong>{{ humanize(e.action) }}</strong> <span class="muted">by {{ e.actor }}</span>
      </div>
      <div v-if="summary(e.detail)" class="small">{{ summary(e.detail) }}</div>
    </li>
  </ol>
</template>
