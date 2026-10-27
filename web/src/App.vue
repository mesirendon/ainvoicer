<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
</script>

<template>
  <header class="topbar">
    <RouterLink to="/" class="brand">AP Agent</RouterLink>
  </header>
  <main class="container">
    <RouterView />
  </main>
</template>
