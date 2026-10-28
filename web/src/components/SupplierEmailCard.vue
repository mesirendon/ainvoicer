<script setup lang="ts">
import { ref, watch } from 'vue'
import { api, errorMessage } from '../api'
import type { EmailDraft } from '../types'

const props = defineProps<{ invoiceId: string }>()

const draft = ref<EmailDraft | null>(null)
const text = ref('')
const busy = ref(false)
const error = ref('')
const copied = ref(false)

async function generate() {
  busy.value = true
  error.value = ''
  copied.value = false
  try {
    draft.value = await api.supplierEmail(props.invoiceId)
    text.value = `Subject: ${draft.value.subject}\n\n${draft.value.body}`
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

async function copy() {
  try {
    await navigator.clipboard.writeText(text.value)
    copied.value = true
  } catch (e) {
    error.value = `Could not copy: ${errorMessage(e)}`
  }
}

watch(
  () => props.invoiceId,
  () => {
    draft.value = null
    text.value = ''
    error.value = ''
  },
)
</script>

<template>
  <section class="card">
    <h2>
      Email to the supplier
      <span v-if="draft" class="badge amber">draft, review before sending</span>
    </h2>
    <p v-if="!draft" class="muted small">
      Drafts a short email explaining the blocking findings, using only the findings above. Nothing is sent.
    </p>
    <p v-if="error" class="bad">{{ error }}</p>
    <template v-if="draft">
      <p class="muted small">
        {{ draft.generatedBy === 'claude' ? 'Drafted by Claude' : 'Template (fixture mode, no API key)' }} ·
        {{ draft.language === 'es' ? 'Spanish' : 'English' }}
      </p>
      <textarea v-model="text" class="email" rows="14" />
    </template>
    <div class="row">
      <button type="button" :disabled="busy" @click="generate">
        {{ busy ? 'Drafting…' : draft ? 'Draft again' : 'Draft email to supplier' }}
      </button>
      <button v-if="draft" type="button" @click="copy">{{ copied ? 'Copied' : 'Copy' }}</button>
    </div>
  </section>
</template>
