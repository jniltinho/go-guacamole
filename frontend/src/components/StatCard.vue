<script setup lang="ts">
import type { Component } from 'vue'

defineProps<{ label: string; value: number | string; icon: Component; color: string; to?: string }>()
</script>

<template>
  <component
    :is="to ? 'RouterLink' : 'div'"
    :to="to"
    class="card flex items-center gap-3.5 px-4 py-3 hover:no-underline"
  >
    <span class="icon-tile h-10 w-10" :style="{ '--tile': color }">
      <component :is="icon" class="h-5 w-5" />
    </span>
    <span>
      <span class="block text-xs font-medium text-muted">{{ label }}</span>
      <span class="block text-2xl leading-tight font-bold" data-testid="stat-value">{{ value }}</span>
    </span>
  </component>
</template>
