<script setup lang="ts">
// Card section. `fill` makes it take the remaining height of a flex/grid parent
// and scroll its content internally (the page itself never scrolls).
defineProps<{ title?: string; fill?: boolean }>()
</script>

<template>
  <section class="card flex min-w-0 flex-col" :class="{ 'min-h-0 flex-1': fill }">
    <header
      v-if="title || $slots.actions"
      class="card-title flex shrink-0 items-center justify-between gap-2 px-4 py-2.5"
    >
      <h2 class="section-title">{{ title }}</h2>
      <div class="flex items-center gap-2"><slot name="actions" /></div>
    </header>
    <slot name="toolbar" />
    <div class="min-w-0 flex-1 overflow-auto" :class="{ 'min-h-0': fill }"><slot /></div>
    <slot name="footer" />
  </section>
</template>
