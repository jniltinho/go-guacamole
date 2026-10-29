<script setup lang="ts">
defineProps<{ text: string }>()
</script>

<template>
  <div class="px-4 py-10 text-center text-muted">{{ text }}</div>
</template>
