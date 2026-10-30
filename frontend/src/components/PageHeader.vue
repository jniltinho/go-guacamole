<script setup lang="ts">
defineProps<{ title: string; subtitle?: string }>()
</script>

<template>
  <div class="flex shrink-0 flex-wrap items-end justify-between gap-3">
    <div>
      <h1 class="text-xl font-bold text-on-bg md:text-2xl">{{ title }}</h1>
      <p v-if="subtitle" class="mt-0.5 text-sm text-on-bg-muted">{{ subtitle }}</p>
    </div>
    <div class="flex flex-wrap items-center gap-2"><slot /></div>
  </div>
</template>
