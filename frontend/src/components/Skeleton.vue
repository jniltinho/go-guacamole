<script setup lang="ts">
// Placeholder rows shaped like the list being loaded (shimmer follows the Effects preference).
withDefaults(defineProps<{ rows?: number }>(), { rows: 6 })
</script>

<template>
  <div class="space-y-2 p-3" aria-busy="true" role="status" data-testid="skeleton">
    <span class="sr-only">…</span>
    <div v-for="i in rows" :key="i" class="flex gap-3">
      <div class="skeleton h-4 w-1/4" />
      <div class="skeleton h-4 flex-1" />
      <div class="skeleton h-4 w-16" />
    </div>
  </div>
</template>
