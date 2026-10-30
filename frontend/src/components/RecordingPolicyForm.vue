<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import type { Tri } from '../api/types'

// Shared fields of the recording policy: video / text / keys and the two
// retentions. `tri` selects inherit/on/off (per connection); otherwise on/off
// checkboxes (global defaults). Retention: null = inherit, -1 = keep forever.
const props = withDefaults(
  defineProps<{
    tri?: boolean
    disabled?: boolean
    textAllowed?: boolean
    inherited?: { video: boolean; text: boolean; keys: boolean; videoDays: number; textDays: number }
  }>(),
  { textAllowed: true },
)
const video = defineModel<Tri | boolean>('video', { required: true })
const text = defineModel<Tri | boolean>('text', { required: true })
const keys = defineModel<Tri | boolean>('keys', { required: true })
const videoDays = defineModel<number | null>('videoDays', { required: true })
const textDays = defineModel<number | null>('textDays', { required: true })
const { t } = useI18n()

const days = (n: number) => (n === -1 ? t('recordingPolicy.forever') : t('recordingPolicy.days', { n }, n))
const inheritLabel = (on: boolean | undefined) =>
  t('recordingPolicy.inherit') +
  (on === undefined ? '' : ` (${on ? t('recordingPolicy.on') : t('recordingPolicy.off')})`)

function mode(v: number | null): 'inherit' | 'days' | 'forever' {
  return v === null ? 'inherit' : v === -1 ? 'forever' : 'days'
}
function setMode(kind: 'video' | 'text', m: string, fallback: number) {
  const v = m === 'inherit' ? null : m === 'forever' ? -1 : fallback > 0 ? fallback : 90
  if (kind === 'video') videoDays.value = v
  else textDays.value = v
}
</script>

<template>
  <div class="grid gap-4 md:grid-cols-3">
    <template v-for="k in ['video', 'text', 'keys'] as const" :key="k">
      <div v-if="k !== 'text' || textAllowed !== false">
        <label class="label" :for="'rec-' + k">{{ t('recordingPolicy.kinds.' + k) }}</label>
        <select
          v-if="tri"
          :id="'rec-' + k"
          class="input"
          :disabled="disabled || (k === 'keys' && video === 'off')"
          :value="k === 'video' ? video : k === 'text' ? text : keys"
          :data-testid="'rec-' + k"
          @change="
            (e) => {
              const v = (e.target as HTMLSelectElement).value as Tri
              if (k === 'video') video = v
              else if (k === 'text') text = v
              else keys = v
            }
          "
        >
          <option value="inherit">{{ inheritLabel(props.inherited?.[k]) }}</option>
          <option value="on">{{ t('recordingPolicy.on') }}</option>
          <option value="off">{{ t('recordingPolicy.off') }}</option>
        </select>
        <label v-else class="flex items-center gap-2 pt-1 text-sm">
          <input
            type="checkbox"
            :disabled="disabled || (k === 'keys' && !video)"
            :checked="!!(k === 'video' ? video : k === 'text' ? text : keys)"
            :data-testid="'rec-' + k"
            @change="
              (e) => {
                const v = (e.target as HTMLInputElement).checked
                if (k === 'video') video = v
                else if (k === 'text') text = v
                else keys = v
              }
            "
          />
          {{ t('recordingPolicy.on') }}
        </label>
        <p class="mt-1 text-xs text-muted">{{ t('recordingPolicy.hint.' + k) }}</p>
      </div>
    </template>
    <div
      v-for="r in ['video', 'text'] as const"
      :key="r"
      :class="{ hidden: r === 'text' && textAllowed === false }"
    >
      <label class="label">{{ t('recordingPolicy.retention.' + r) }}</label>
      <div class="flex gap-2">
        <select
          class="input"
          :disabled="disabled"
          :value="mode(r === 'video' ? videoDays : textDays)"
          @change="
            setMode(
              r,
              ($event.target as HTMLSelectElement).value,
              (r === 'video' ? props.inherited?.videoDays : props.inherited?.textDays) ?? 90,
            )
          "
        >
          <option v-if="tri" value="inherit">
            {{ t('recordingPolicy.inherit') }}
            <template v-if="props.inherited">
              ({{ days(r === 'video' ? props.inherited.videoDays : props.inherited.textDays) }})</template
            >
          </option>
          <option value="days">{{ t('recordingPolicy.inDays') }}</option>
          <option value="forever">{{ t('recordingPolicy.forever') }}</option>
        </select>
        <input
          v-if="mode(r === 'video' ? videoDays : textDays) === 'days'"
          class="input w-28"
          type="number"
          min="1"
          max="3650"
          :disabled="disabled"
          :value="r === 'video' ? videoDays : textDays"
          :data-testid="'rec-days-' + r"
          @input="
            (e) => {
              const n = Number((e.target as HTMLInputElement).value)
              if (r === 'video') videoDays = n
              else textDays = n
            }
          "
        />
      </div>
    </div>
  </div>
</template>
