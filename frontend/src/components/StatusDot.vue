<script setup lang="ts">
import { useI18n } from 'vue-i18n'

defineProps<{ status: string }>()
const { t } = useI18n()
const colors: Record<string, string> = { online: 'var(--success)', offline: 'var(--danger)' }
</script>

<template>
  <span class="inline-flex items-center gap-2 text-sm">
    <span
      class="inline-block h-2.5 w-2.5"
      :class="'status-' + status"
      :style="{ background: colors[status] ?? 'var(--muted)' }"
    />
    {{ t('status.' + status) }}
  </span>
</template>
