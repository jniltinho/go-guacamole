<script setup lang="ts">
defineProps<{ protocol: string }>()
</script>

<template>
  <span class="badge" :style="{ background: `var(--proto-${protocol}, var(--muted))` }">{{ protocol }}</span>
</template>
