<script setup lang="ts">
import { CheckCircle2, Info, X, XCircle } from '@lucide/vue'
import { useI18n } from 'vue-i18n'

import { useToast } from '../composables/useToast'

const { toasts, dismiss } = useToast()
const { t } = useI18n()
</script>

<template>
  <!-- polite region for results; errors are alerts -->
  <div
    class="pointer-events-none fixed right-4 bottom-4 z-[120] flex w-80 flex-col gap-2"
    aria-live="polite"
    role="status"
  >
    <TransitionGroup name="rise">
      <div
        v-for="m in toasts"
        :key="m.id"
        class="card pointer-events-auto flex items-start gap-2 px-3 py-2 text-sm shadow-elev-3"
        :role="m.kind === 'error' ? 'alert' : undefined"
        data-testid="toast"
      >
        <component
          :is="m.kind === 'error' ? XCircle : m.kind === 'info' ? Info : CheckCircle2"
          class="mt-0.5 h-4 w-4 shrink-0"
          :class="m.kind === 'error' ? 'text-danger' : m.kind === 'info' ? 'text-info' : 'text-success'"
        />
        <span class="flex-1">{{ m.text }}</span>
        <button class="text-muted hover:text-fg" :aria-label="t('common.close')" @click="dismiss(m.id)">
          <X class="h-4 w-4" />
        </button>
      </div>
    </TransitionGroup>
  </div>
</template>
