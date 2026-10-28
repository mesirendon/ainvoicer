<script setup lang="ts">
import { computed } from 'vue'
import type { Stage, StageId, Tone } from '../about/model'

const props = defineProps<{ stages: Stage[]; current: StageId; tone: Tone }>()

const index = computed(() => props.stages.findIndex((s) => s.id === props.current))
const position = computed(() => `${(index.value / (props.stages.length - 1)) * 100}%`)
</script>

<template>
  <div class="track" :style="{ '--pos': position }">
    <div class="rail">
      <div class="fill" :class="tone" />
      <div class="chip" :class="tone">
        <span>PDF</span>
      </div>
    </div>
    <ol class="nodes">
      <li
        v-for="(s, i) in stages"
        :key="s.id"
        :class="{ active: s.id === current, reached: i < index, [tone]: s.id === current }"
      >
        <span class="dot">{{ s.icon }}</span>
        <span class="label">{{ s.label }}</span>
      </li>
    </ol>
  </div>
</template>

<style scoped>
.track {
  position: relative;
  padding: 44px 28px 4px;
}

.rail {
  position: absolute;
  left: 46px;
  right: 46px;
  top: 60px;
  height: 4px;
  border-radius: 2px;
  background: var(--border);
}

.fill {
  height: 100%;
  width: var(--pos);
  border-radius: 2px;
  background: var(--accent);
  transition:
    width 0.9s ease,
    background 0.4s;
}

.chip {
  position: absolute;
  top: -48px;
  left: var(--pos);
  transform: translateX(-50%);
  transition: left 0.9s cubic-bezier(0.65, 0, 0.35, 1);
}

.chip span {
  display: inline-block;
  padding: 3px 8px;
  border-radius: 6px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: #fff;
  background: var(--accent);
  box-shadow: 0 4px 12px rgb(29 78 216 / 30%);
  animation: bob 1.6s ease-in-out infinite;
  transition: background 0.4s;
}

.fill.ok,
.chip.ok span {
  background: var(--green);
}

.fill.warn,
.chip.warn span {
  background: var(--amber);
}

.fill.block,
.chip.block span {
  background: var(--red);
}

.nodes {
  position: relative;
  display: flex;
  justify-content: space-between;
  list-style: none;
  margin: 0;
  padding: 0;
}

.nodes li {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  width: 36px;
}

.dot {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border-radius: 50%;
  border: 2px solid var(--border);
  background: var(--card);
  color: var(--muted);
  font-size: 15px;
  transition: all 0.4s;
}

.label {
  font-size: 11px;
  font-weight: 600;
  color: var(--muted);
  white-space: nowrap;
}

.reached .dot {
  border-color: var(--accent);
  color: var(--accent);
}

.active .dot {
  color: #fff;
  background: var(--accent);
  border-color: var(--accent);
  animation: pulse 1.6s ease-out infinite;
}

.active .label {
  color: var(--text);
}

.active.ok .dot {
  background: var(--green);
  border-color: var(--green);
}

.active.warn .dot {
  background: var(--amber);
  border-color: var(--amber);
}

.active.block .dot {
  background: var(--red);
  border-color: var(--red);
}

@keyframes pulse {
  0% {
    box-shadow: 0 0 0 0 rgb(29 78 216 / 35%);
  }
  100% {
    box-shadow: 0 0 0 14px rgb(29 78 216 / 0%);
  }
}

@keyframes bob {
  50% {
    transform: translateY(-3px);
  }
}

@media (prefers-reduced-motion: reduce) {
  .fill,
  .chip,
  .chip span,
  .dot {
    transition: none;
    animation: none !important;
  }
}
</style>
