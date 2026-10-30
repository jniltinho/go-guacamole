<script setup lang="ts">
import { inject } from 'vue'

// Card section. `fill` makes it take the remaining height of a flex/grid parent
// and scroll its content internally (the page itself never scrolls). Inside a tab (PageTabs) the title is a plain
// heading instead of the theme's title bar: the tab names the section and opens into the card.
defineProps<{ title?: string; fill?: boolean }>()
const inTab = inject('page-tabs', false)
</script>

<template>
  <section class="card flex min-w-0 flex-col" :class="{ 'min-h-0 flex-1': fill }">
    <!-- min-h-10: same title height with or without a button in the actions -->
    <header
      v-if="title || $slots.actions"
      class="flex min-h-10 shrink-0 items-center justify-between gap-2"
      :class="inTab ? 'border-b border-line px-4 pt-3 pb-2' : 'card-title px-3 py-1'"
    >
      <h2 class="section-title">{{ title }}</h2>
      <div class="flex items-center gap-2"><slot name="actions" /></div>
    </header>
    <slot name="toolbar" />
    <div class="min-w-0 flex-1 overflow-auto" :class="{ 'min-h-0': fill }"><slot /></div>
    <slot name="footer" />
  </section>
</template>
