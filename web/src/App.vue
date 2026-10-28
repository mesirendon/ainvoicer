<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import { errorMessage } from './api'
import { clock, loadMeta, meta, refreshClock } from './session'

const route = useRoute()
const error = ref('')

watch(() => route.fullPath, refreshClock)
onMounted(async () => {
  void refreshClock()
  try {
    await loadMeta()
  } catch (e) {
    error.value = `The API is not reachable: ${errorMessage(e)}`
  }
})
</script>

<template>
  <header class="topbar">
    <RouterLink to="/" class="brand">AP Agent</RouterLink>
    <nav>
      <RouterLink to="/">Invoices</RouterLink>
      <RouterLink to="/payments">Payments &amp; demo</RouterLink>
    </nav>
    <span class="spacer" />
    <span v-if="meta" class="muted small">{{ meta.buyerName }} · pays in {{ meta.fundingCurrency }}</span>
    <span v-if="clock" class="clock">Demo date {{ clock.today }}</span>
  </header>
  <p v-if="meta?.fixtureMode" class="fixture-banner">Fixture mode: no API key, using pre-extracted data</p>
  <p v-if="error" class="error-banner container-banner">{{ error }}</p>
  <main class="container">
    <RouterView />
  </main>
</template>
