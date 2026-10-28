<script setup lang="ts">
import { computed, ref } from 'vue'

const props = defineProps<{ title: string; confirmLabel: string; busy: boolean }>()
const emit = defineEmits<{ confirm: [reason: string]; cancel: [] }>()

const reason = ref('')
const valid = computed(() => reason.value.trim().length > 0)

function submit() {
  if (valid.value && !props.busy) emit('confirm', reason.value.trim())
}
</script>

<template>
  <div class="modal-backdrop" @click.self="emit('cancel')">
    <form class="modal" @submit.prevent="submit">
      <h3>{{ title }}</h3>
      <label for="reason">Reason (required, stored in the audit log)</label>
      <textarea id="reason" v-model="reason" rows="4" maxlength="500" autofocus />
      <div class="modal-actions">
        <button type="button" @click="emit('cancel')">Cancel</button>
        <button type="submit" class="primary" :disabled="!valid || busy">{{ busy ? 'Saving…' : confirmLabel }}</button>
      </div>
    </form>
  </div>
</template>
