<script setup lang="ts">
import { humanize, tone } from '../format'

defineProps<{ status: string }>()
</script>

<template>
  <span class="badge" :class="tone(status)">{{ humanize(status) }}</span>
</template>
