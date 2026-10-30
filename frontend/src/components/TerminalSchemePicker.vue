<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { api } from '../api/client'

// SSH terminal palette (like rustXterm): "follow" matches the UI theme; a
// connection that sets its own color scheme wins over this preference.
interface Scheme {
  id: string
  label: string
  dark: boolean
  foreground: string
  background: string
  colors: string[]
}
const props = defineProps<{ modelValue: string; uiTheme: string }>()
const emit = defineEmits<{ 'update:modelValue': [v: string] }>()
const { t } = useI18n()
const schemes = ref<Scheme[]>([])
const follow = ref<Record<string, string>>({})

onMounted(async () => {
  const r = await api.get<{ schemes: Scheme[]; follow: Record<string, string> }>('/terminal-schemes')
  schemes.value = r.schemes
  follow.value = r.follow
})

const followed = computed(() => schemes.value.find((s) => s.id === follow.value[props.uiTheme]))
const selected = computed(() => props.modelValue || 'follow')
const preview = computed(() =>
  selected.value === 'follow' ? followed.value : schemes.value.find((s) => s.id === selected.value),
)
</script>

<template>
  <div class="grid gap-3 md:grid-cols-[minmax(0,18rem)_1fr]">
    <div
      class="max-h-72 overflow-y-auto border border-line"
      role="radiogroup"
      :aria-label="t('settings.terminalScheme')"
    >
      <button
        type="button"
        role="radio"
        :aria-checked="selected === 'follow'"
        class="flex w-full items-center gap-2 px-2 py-1 text-left text-sm"
        :class="selected === 'follow' ? 'bg-accent text-accent-fg' : 'hover:bg-surface-raised'"
        data-testid="term-follow"
        @click="emit('update:modelValue', 'follow')"
      >
        {{ t('settings.terminalFollow') }}
        <span v-if="followed" class="text-xs">({{ followed.label }})</span>
      </button>
      <button
        v-for="s in schemes"
        :key="s.id"
        type="button"
        role="radio"
        :aria-checked="selected === s.id"
        class="flex w-full items-center gap-2 px-2 py-1 text-left text-sm"
        :class="selected === s.id ? 'bg-accent text-accent-fg' : 'hover:bg-surface-raised'"
        :data-testid="'term-' + s.id"
        @click="emit('update:modelValue', s.id)"
      >
        <span class="flex shrink-0 border border-line" aria-hidden="true">
          <span class="h-3 w-2" :style="{ background: s.background }" />
          <span v-for="c in s.colors.slice(0, 8)" :key="c" class="h-3 w-2" :style="{ background: c }" />
        </span>
        <span class="truncate">{{ s.label }}</span>
      </button>
    </div>
    <!-- live preview of the palette -->
    <pre
      v-if="preview"
      class="m-0 overflow-hidden border border-line p-3 font-mono text-xs leading-5"
      :style="{ background: preview.background, color: preview.foreground }"
      aria-hidden="true"
    ><span :style="{ color: preview.colors[10] }">ops@noc-01</span>:<span :style="{ color: preview.colors[12] }">~/srv</span>$ systemctl status nginx
<span :style="{ color: preview.colors[2] }">●</span> nginx.service - A high performance web server
   Active: <span :style="{ color: preview.colors[10] }">active (running)</span> since Mon 06:12
<span :style="{ color: preview.colors[3] }">warning</span>: disk /var at 81%   <span :style="{ color: preview.colors[1] }">error</span>: upstream timed out
$ ls -l
drwxr-xr-x <span :style="{ color: preview.colors[4] }">logs</span>  <span :style="{ color: preview.colors[6] }">backup.sh</span>  <span :style="{ color: preview.colors[5] }">nginx.conf</span></pre>
  </div>
</template>
