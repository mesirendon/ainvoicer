<script setup lang="ts">
import { computed } from 'vue'
import { boxHeight, boxWidth, components, edges, type ComponentId } from '../about/model'

const props = defineProps<{ active: ComponentId[]; selected: ComponentId | null }>()
const emit = defineEmits<{ select: [id: ComponentId] }>()

const byId = Object.fromEntries(components.map((c) => [c.id, c]))

function center(id: ComponentId) {
  const c = byId[id]!
  return { x: c.x + boxWidth / 2, y: c.y + boxHeight / 2 }
}

const lines = computed(() =>
  edges.map(([from, to]) => {
    const a = center(from)
    const b = center(to)
    return { key: `${from}-${to}`, a, b, live: props.active.includes(from) && props.active.includes(to) }
  }),
)
</script>

<template>
  <svg class="map" viewBox="0 0 1010 580" role="img" aria-label="Architecture map">
    <g class="zones">
      <rect x="345" y="20" width="510" height="530" rx="14" />
      <text x="360" y="542">api container (Go)</text>
    </g>
    <line
      v-for="l in lines"
      :key="l.key"
      :x1="l.a.x"
      :y1="l.a.y"
      :x2="l.b.x"
      :y2="l.b.y"
      class="edge"
      :class="{ live: l.live }"
    />
    <g
      v-for="c in components"
      :key="c.id"
      class="node"
      :class="[c.kind, { active: active.includes(c.id), selected: selected === c.id }]"
      tabindex="0"
      role="button"
      :aria-label="c.label"
      @click="emit('select', c.id)"
      @keydown.enter="emit('select', c.id)"
      @keydown.space.prevent="emit('select', c.id)"
    >
      <rect :x="c.x" :y="c.y" :width="boxWidth" :height="boxHeight" rx="10" />
      <text :x="c.x + boxWidth / 2" :y="c.y + 25" class="title">{{ c.label }}</text>
      <text :x="c.x + boxWidth / 2" :y="c.y + 43" class="sub">{{ c.sub }}</text>
    </g>
  </svg>
</template>

<style scoped>
.map {
  width: 100%;
  height: auto;
  display: block;
}

.zones rect {
  fill: var(--zone-bg);
  stroke: var(--border);
  stroke-dasharray: 4 4;
}

.zones text {
  font-size: 12px;
  fill: var(--muted);
}

.edge {
  stroke: var(--edge);
  stroke-width: 2;
  transition: stroke 0.4s;
}

.edge.live {
  stroke: var(--accent);
  stroke-width: 3;
  stroke-dasharray: 8 6;
  animation: flow 0.7s linear infinite;
}

.node {
  cursor: pointer;
  outline: none;
}

.node rect {
  fill: var(--card);
  stroke: var(--edge);
  stroke-width: 1.5;
  transition:
    fill 0.4s,
    stroke 0.4s,
    transform 0.4s;
}

.node.rules rect {
  fill: var(--node-rules);
}

.node.data rect {
  fill: var(--node-data);
}

.node.external rect {
  fill: var(--node-external);
}

.node.job rect {
  fill: var(--node-job);
}

.node text {
  text-anchor: middle;
  pointer-events: none;
}

.title {
  font-size: 13px;
  font-weight: 700;
  fill: var(--text);
}

.sub {
  font-size: 11px;
  fill: var(--muted);
}

.node.active rect {
  stroke: var(--accent);
  stroke-width: 2.5;
  filter: drop-shadow(0 4px 10px rgb(29 78 216 / 30%));
}

.node.selected rect,
.node:focus-visible rect {
  stroke: var(--text);
  stroke-width: 2.5;
}

.node:hover rect {
  stroke: var(--accent);
}

@keyframes flow {
  to {
    stroke-dashoffset: -28;
  }
}

@media (prefers-reduced-motion: reduce) {
  .edge.live {
    animation: none;
  }
}
</style>
