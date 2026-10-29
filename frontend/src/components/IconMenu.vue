<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'

// Minimal icon button that opens a popover menu (theme / language pickers).
const props = withDefaults(defineProps<{ label: string; align?: 'left' | 'right'; testid?: string }>(), {
  align: 'right',
  testid: undefined,
})
const open = ref(false)
const root = ref<HTMLElement>()
const panel = ref<HTMLElement>()

function onDocClick(e: MouseEvent) {
  if (root.value && !root.value.contains(e.target as Node)) open.value = false
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    open.value = false
    root.value?.querySelector<HTMLButtonElement>('button')?.focus()
  }
}

watch(open, async (o) => {
  if (o) {
    document.addEventListener('mousedown', onDocClick)
    document.addEventListener('keydown', onKey)
    await nextTick()
    panel.value?.querySelector<HTMLElement>('[aria-checked="true"], [role="menuitemradio"]')?.focus()
  } else {
    document.removeEventListener('mousedown', onDocClick)
    document.removeEventListener('keydown', onKey)
  }
})
onBeforeUnmount(() => (open.value = false))

defineExpose({ close: () => (open.value = false) })
</script>

<template>
  <div ref="root" class="relative">
    <button
      type="button"
      class="icon-btn"
      :aria-label="props.label"
      :title="props.label"
      aria-haspopup="menu"
      :aria-expanded="open"
      :data-testid="props.testid"
      @click="open = !open"
    >
      <slot name="icon" />
    </button>
    <div
      v-if="open"
      ref="panel"
      class="card absolute z-50 mt-2 max-h-[70vh] w-60 overflow-y-auto py-1"
      :class="props.align === 'right' ? 'right-0' : 'left-0'"
      role="menu"
      :aria-label="props.label"
    >
      <div class="px-3 pt-1.5 pb-1 text-[0.7rem] font-semibold tracking-wider text-muted uppercase">
        {{ props.label }}
      </div>
      <slot :close="() => (open = false)" />
    </div>
  </div>
</template>

<style scoped>
.icon-btn {
  display: inline-flex;
  height: 1.875rem;
  width: 1.875rem;
  align-items: center;
  justify-content: center;
  color: var(--muted);
  border: 1px solid transparent;
  background: transparent;
  cursor: pointer;
  transition:
    color 0.12s,
    background-color 0.12s,
    border-color 0.12s;
}
.icon-btn:hover,
.icon-btn[aria-expanded='true'] {
  color: var(--fg);
  background: color-mix(in srgb, var(--fg) 8%, transparent);
}
</style>
