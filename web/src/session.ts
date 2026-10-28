import { ref } from 'vue'
import { api } from './api'
import type { Clock, Meta } from './types'

export const meta = ref<Meta | null>(null)
export const clock = ref<Clock | null>(null)

export async function loadMeta(): Promise<void> {
  meta.value = await api.meta()
}

export async function refreshClock(): Promise<void> {
  try {
    clock.value = await api.clock()
  } catch {
    clock.value = null
  }
}
