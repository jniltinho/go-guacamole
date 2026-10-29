<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const props = defineProps<{ total: number; limit: number }>()
const offset = defineModel<number>({ required: true })
const { t } = useI18n()
</script>

<template>
  <div
    v-if="props.total > props.limit"
    class="flex items-center justify-end gap-3 border-t border-line px-4 py-2 text-sm"
  >
    <span class="text-muted">{{
      t('common.pageOf', { from: offset + 1, to: Math.min(offset + limit, total), total })
    }}</span>
    <button class="btn btn-sm" :disabled="offset === 0" @click="offset = Math.max(0, offset - limit)">
      {{ t('common.previous') }}
    </button>
    <button class="btn btn-sm" :disabled="offset + limit >= total" @click="offset += limit">
      {{ t('common.next') }}
    </button>
  </div>
</template>
