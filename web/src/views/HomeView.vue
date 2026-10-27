<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, ApiError } from '../api'

const status = ref<'loading' | 'ok' | 'down'>('loading')
const detail = ref('')

async function check() {
  status.value = 'loading'
  try {
    const health = await api.health()
    status.value = health.status === 'ok' ? 'ok' : 'down'
    detail.value = ''
  } catch (e) {
    status.value = 'down'
    detail.value = e instanceof ApiError ? `${e.status} ${e.code}: ${e.message}` : String(e)
  }
}

onMounted(check)
</script>

<template>
  <section class="card">
    <h1>Accounts-payable agent</h1>
    <p>
      API health:
      <span class="badge" :class="status">{{ status }}</span>
      <button type="button" @click="check">Refresh</button>
    </p>
    <p v-if="detail" class="muted">{{ detail }}</p>
  </section>
</template>
