<script setup lang="ts">
import { Download, X } from '@lucide/vue'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import type { HistoryEntry } from '../api/types'

// Text transcript of an SSH session (what the user typed and saw on the server).
const props = defineProps<{ entry: HistoryEntry }>()
const emit = defineEmits<{ close: [] }>()
const { t, d } = useI18n()
const text = ref('')
const error = ref('')
const filter = ref('')

const lines = computed(() => {
  const all = text.value.split('\n')
  const f = filter.value.trim().toLowerCase()
  return f ? all.filter((l) => l.toLowerCase().includes(f)) : all
})

onMounted(async () => {
  const res = await fetch(`/api/v1/history/${props.entry.id}/typescript`, { credentials: 'same-origin' })
  if (res.ok) text.value = await res.text()
  else error.value = t('recordings.unavailable')
})
</script>

<template>
  <Teleport to="body">
    <div
      class="fixed inset-0 z-[100] flex items-center justify-center bg-black/70 p-4"
      role="dialog"
      aria-modal="true"
      @keydown.esc="emit('close')"
    >
      <div class="card flex h-[85vh] w-full max-w-5xl flex-col">
        <div class="card-title flex items-center justify-between gap-3 px-4 py-2.5">
          <div class="min-w-0">
            <h2 class="section-title truncate">
              {{ t('recordings.transcript') }} · {{ entry.connectionName }}
            </h2>
            <p class="text-xs text-muted">{{ entry.username }} · {{ d(entry.startAt, 'long') }}</p>
          </div>
          <div class="flex items-center gap-2">
            <a class="btn btn-sm" :href="`/api/v1/history/${entry.id}/typescript?format=raw`" download>
              <Download class="h-4 w-4" /> {{ t('recordings.download') }}
            </a>
            <button class="btn btn-sm" :aria-label="t('common.close')" @click="emit('close')">
              <X class="h-4 w-4" />
            </button>
          </div>
        </div>
        <div class="border-b border-line p-2">
          <input v-model="filter" type="search" class="input" :placeholder="t('recordings.filter')" />
        </div>
        <pre
          class="min-h-0 flex-1 overflow-auto bg-black p-4 font-mono text-[13px] leading-relaxed whitespace-pre-wrap text-neutral-100"
          data-testid="transcript"
          >{{ error || lines.join('\n') }}</pre>
        <p class="border-t border-line px-4 py-2 text-xs text-muted">{{ t('recordings.rawHint') }}</p>
      </div>
    </div>
  </Teleport>
</template>
