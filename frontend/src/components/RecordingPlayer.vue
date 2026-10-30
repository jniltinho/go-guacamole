<script setup lang="ts">
import { Pause, Play, X } from '@lucide/vue'
import Guacamole from 'guacamole-common-js'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import type { HistoryEntry } from '../api/types'
import { duration as fmt } from '../lib/format'

// Plays a guacd screen recording (Guacamole protocol dump) in the browser.
const props = defineProps<{ entry: HistoryEntry }>()
const emit = defineEmits<{ close: [] }>()
const { t, d } = useI18n()

const host = ref<HTMLDivElement>()
const box = ref<HTMLDivElement>()
const loading = ref(true)
const error = ref('')
const playing = ref(false)
const position = ref(0)
const total = ref(0)
let rec: Guacamole.SessionRecording | null = null
let raf = 0
let observer: ResizeObserver | null = null

const progress = computed(() => (total.value ? (position.value / total.value) * 100 : 0))

function fit() {
  if (!rec || !box.value) return
  const display = rec.getDisplay()
  const w = display.getWidth()
  const h = display.getHeight()
  if (!w || !h) return
  const b = box.value.getBoundingClientRect()
  display.scale(Math.min(b.width / w, b.height / h))
}

function tick() {
  if (rec) position.value = rec.getPosition()
  if (playing.value) raf = requestAnimationFrame(tick)
}

function toggle() {
  if (!rec) return
  if (rec.isPlaying()) rec.pause()
  else {
    if (position.value >= total.value) rec.seek(0)
    rec.play()
  }
}

function seek(e: Event) {
  if (!rec) return
  const v = Number((e.target as HTMLInputElement).value)
  rec.seek((v / 100) * total.value)
}

onMounted(async () => {
  try {
    // A tunnel source (streamed over HTTP) is used instead of a Blob: guacamole-common-js
    // 1.5.0 fails to initialise Blob sources (its internal recordingBlob stays undefined).
    const tunnel = new Guacamole.StaticHTTPTunnel(`/api/v1/history/${props.entry.id}/recording`, false)
    tunnel.onerror = () => {
      error.value = t('recordings.unavailable')
      loading.value = false
    }
    rec = new Guacamole.SessionRecording(tunnel)
    const display = rec.getDisplay()
    host.value!.replaceChildren(display.getElement())
    display.onresize = fit
    rec.onprogress = (dur: number) => {
      total.value = dur
    }
    rec.onload = () => {
      loading.value = false
      total.value = rec!.getDuration()
      rec!.play()
    }
    rec.onerror = (m: string) => {
      error.value = m
      loading.value = false
    }
    rec.onplay = () => {
      playing.value = true
      tick()
    }
    rec.onpause = () => {
      playing.value = false
      cancelAnimationFrame(raf)
      position.value = rec!.getPosition()
    }
    rec.onseek = (p: number) => {
      position.value = p
    }
    rec.connect()
    observer = new ResizeObserver(fit)
    observer.observe(box.value!)
  } catch {
    error.value = t('recordings.unavailable')
    loading.value = false
  }
})

onBeforeUnmount(() => {
  cancelAnimationFrame(raf)
  observer?.disconnect()
  rec?.pause()
  rec?.disconnect()
  rec = null
})
</script>

<template>
  <Teleport to="body">
    <div
      class="fixed inset-0 z-[100] flex items-start justify-center bg-black/70 px-4 pt-[15vh] pb-4"
      role="dialog"
      aria-modal="true"
      @keydown.esc="emit('close')"
    >
      <div class="card flex h-[80vh] w-full max-w-6xl flex-col">
        <div class="card-title flex items-center justify-between gap-3 px-4 py-2.5">
          <div class="min-w-0">
            <h2 class="section-title truncate">{{ t('recordings.screen') }} · {{ entry.connectionName }}</h2>
            <p class="text-xs opacity-85">{{ entry.username }} · {{ d(entry.startAt, 'long') }}</p>
          </div>
          <button class="btn btn-sm" :aria-label="t('common.close')" @click="emit('close')">
            <X class="h-4 w-4" />
          </button>
        </div>
        <div ref="box" class="relative min-h-0 flex-1 overflow-hidden bg-black">
          <div
            ref="host"
            class="absolute inset-0 flex items-center justify-center"
            data-testid="recording-display"
          />
          <div
            v-if="loading || error"
            class="absolute inset-0 flex items-center justify-center text-sm text-white/80"
          >
            {{ error || t('common.loading') }}
          </div>
        </div>
        <div class="flex items-center gap-3 border-t border-line px-4 py-2.5">
          <button
            class="btn btn-sm btn-primary"
            :disabled="loading || !!error"
            data-testid="recording-toggle"
            @click="toggle"
          >
            <component :is="playing ? Pause : Play" class="h-4 w-4" />
            {{ playing ? t('recordings.pause') : t('recordings.play') }}
          </button>
          <input
            type="range"
            min="0"
            max="100"
            step="0.1"
            class="flex-1 accent-[var(--accent)]"
            :value="progress"
            :disabled="loading || !!error"
            :aria-label="t('recordings.position')"
            @input="seek"
          />
          <span class="w-36 text-right font-mono text-xs text-muted"
            >{{ fmt(position) }} / {{ fmt(total) }}</span
          >
        </div>
      </div>
    </div>
  </Teleport>
</template>
