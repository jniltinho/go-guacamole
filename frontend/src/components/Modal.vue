<script setup lang="ts">
import { Check, X } from '@lucide/vue'
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

// Form dialog in the panel style (as in AD Manager): a native <dialog> opened with showModal() on mount, so focus is
// trapped, the page is inert and Esc works; the title bar follows the theme. ✕, Esc and Cancel fade it out and emit
// "close"; submitting the form emits "submit". Render it with v-if: mounting opens it.
const props = withDefaults(
  defineProps<{
    title: string
    wide?: boolean
    busy?: boolean
    // ✕, Esc and Cancel do nothing while a write is in flight
    locked?: boolean
    // label of the submit button; null hides it (an information dialog with Close only)
    ok?: string | null
    danger?: boolean
    // disables the submit button (e.g. a typed confirmation that does not match yet)
    disabled?: boolean
    testid?: string
  }>(),
  { ok: '' },
)
const emit = defineEmits<{ close: []; submit: [] }>()
const { t } = useI18n()
const dlg = ref<HTMLDialogElement>()

onMounted(() => dlg.value?.showModal())

function close() {
  const el = dlg.value
  if (!el || props.locked || el.classList.contains('closing')) return
  if (matchMedia('(prefers-reduced-motion: reduce)').matches) return emit('close')
  el.classList.add('closing')
  setTimeout(() => emit('close'), 150)
}
defineExpose({ close })
</script>

<template>
  <dialog
    ref="dlg"
    class="modal card"
    :class="wide ? 'w-[760px]' : 'w-[460px]'"
    :aria-label="title"
    :data-testid="testid"
    @cancel.prevent="close"
  >
    <form class="flex max-h-[85vh] flex-col" @submit.prevent="emit('submit')">
      <div class="card-title flex shrink-0 items-center justify-between gap-3 px-4 py-2.5">
        <h2 class="section-title truncate">{{ title }}</h2>
        <button
          type="button"
          class="opacity-80 hover:opacity-100"
          :aria-label="t('common.close')"
          @click="close"
        >
          <X class="h-4 w-4" />
        </button>
      </div>
      <div class="min-h-0 flex-1 overflow-y-auto p-4"><slot /></div>
      <div class="flex shrink-0 flex-wrap items-center justify-end gap-2 border-t border-line px-4 py-3">
        <slot name="foot" />
        <button type="button" class="btn" :disabled="locked" data-testid="modal-cancel" @click="close">
          {{ ok === null ? t('common.close') : t('common.cancel') }}
        </button>
        <button
          v-if="ok !== null"
          type="submit"
          class="btn"
          :class="danger ? 'btn-danger' : 'btn-primary'"
          :disabled="busy || disabled"
          data-testid="modal-ok"
        >
          <Check class="h-4 w-4" /> {{ ok || t('common.save') }}
        </button>
      </div>
    </form>
  </dialog>
</template>
