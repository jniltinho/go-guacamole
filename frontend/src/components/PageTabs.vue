<script setup lang="ts" generic="T extends string">
import { nextTick, type Component } from 'vue'

// Tab band of a page (AD Manager style, see .subtabs in main.css) and the panel below it, for pages laid out with
// .page-fill: the band sits on the first card and only the panel scrolls. ARIA tabs: arrow keys, Home and End.
export interface PageTab<K extends string> {
  id: K
  label: string
  icon?: Component
  dot?: boolean // a small marker, e.g. "enabled"
}

const props = defineProps<{ tabs: PageTab<T>[]; name: string; label?: string }>()
const model = defineModel<T>({ required: true })

const tabId = (id: string) => `${props.name}-tab-${id}`

function key(e: KeyboardEvent) {
  const ids = props.tabs.map((t) => t.id)
  const i = ids.indexOf(model.value)
  const next = { ArrowRight: i + 1, ArrowLeft: i - 1, Home: 0, End: ids.length - 1 }[e.key]
  if (next === undefined) return
  e.preventDefault()
  model.value = ids[(next + ids.length) % ids.length]!
  void nextTick(() => document.getElementById(tabId(model.value))?.focus())
}
</script>

<template>
  <!-- the band sits on the content: its bottom line overlaps the first card's top border -->
  <div class="subtabs relative z-10 -mb-[17px] shrink-0" role="tablist" :aria-label="label">
    <button
      v-for="t in tabs"
      :id="tabId(t.id)"
      :key="t.id"
      type="button"
      role="tab"
      class="subtab"
      :class="{ on: model === t.id }"
      :aria-selected="model === t.id"
      :aria-controls="name + '-panel'"
      :tabindex="model === t.id ? 0 : -1"
      :data-testid="name + '-' + t.id"
      @click="model = t.id"
      @keydown="key"
    >
      <component :is="t.icon" v-if="t.icon" class="h-4 w-4" :stroke-width="1.75" />
      {{ t.label }}
      <span v-if="t.dot" class="h-2 w-2 shrink-0 rounded-full bg-success" aria-hidden="true" />
    </button>
  </div>
  <div
    :id="name + '-panel'"
    class="flex min-h-0 flex-1 flex-col overflow-y-auto"
    role="tabpanel"
    :aria-labelledby="tabId(model)"
  >
    <slot />
  </div>
</template>
