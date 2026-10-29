<script setup lang="ts">
import type { Component } from 'vue'

defineProps<{ label: string; value: number | string; icon: Component; color: string; to?: string }>()
</script>

<template>
  <component
    :is="to ? 'RouterLink' : 'div'"
    :to="to"
    class="card flex items-center gap-4 p-5 hover:no-underline"
  >
    <span
      class="flex h-14 w-14 shrink-0 items-center justify-center text-white"
      :style="{ background: color }"
    >
      <component :is="icon" class="h-7 w-7" />
    </span>
    <span>
      <span class="block text-sm font-medium text-muted">{{ label }}</span>
      <span class="block text-3xl font-bold" data-testid="stat-value">{{ value }}</span>
    </span>
  </component>
</template>
