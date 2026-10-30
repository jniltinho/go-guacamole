<script setup lang="ts">
import { CheckCircle2, Info, X, XCircle } from '@lucide/vue'
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { useToast } from '../composables/useToast'

// Toast stack in the theme (as in AD Manager): a manual popover, so it lives in the top layer and shows above an
// open modal dialog; it is raised again on every new toast to stay the topmost layer.
const { toasts, dismiss } = useToast()
const { t } = useI18n()
const stack = ref<HTMLElement>()
const ICONS = { success: CheckCircle2, error: XCircle, info: Info }

function raise() {
  const el = stack.value
  if (!el?.showPopover) return
  if (el.matches(':popover-open')) el.hidePopover()
  el.showPopover()
}
onMounted(raise)
watch(
  () => toasts.value.length,
  (n, old) => n > old && raise(),
)
</script>

<template>
  <div ref="stack" class="toasts" popover="manual" aria-live="polite">
    <TransitionGroup name="rise">
      <div
        v-for="m in toasts"
        :key="m.id"
        class="toast"
        :class="m.kind"
        :role="m.kind === 'error' ? 'alert' : 'status'"
        data-testid="toast"
      >
        <div class="toast-head">
          <component :is="ICONS[m.kind]" class="toast-icon h-4 w-4 shrink-0" />
          <span class="flex-1">{{ t('common.toast.' + m.kind) }}</span>
          <button class="opacity-70 hover:opacity-100" :aria-label="t('common.close')" @click="dismiss(m.id)">
            <X class="h-4 w-4" />
          </button>
        </div>
        <div class="toast-body">{{ m.text }}</div>
      </div>
    </TransitionGroup>
  </div>
</template>
