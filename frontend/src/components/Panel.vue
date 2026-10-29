<script setup lang="ts">
defineProps<{ title?: string }>()
</script>

<template>
  <section class="card flex min-w-0 flex-col">
    <header
      v-if="title || $slots.actions"
      class="card-title flex items-center justify-between gap-2 px-4 py-3"
    >
      <h2 class="section-title">{{ title }}</h2>
      <div class="flex items-center gap-2"><slot name="actions" /></div>
    </header>
    <div class="min-w-0 flex-1 overflow-x-auto"><slot /></div>
  </section>
</template>
