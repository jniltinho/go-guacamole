<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { ApiError } from '../api/client'

const props = defineProps<{ error: unknown }>()
const { t, te } = useI18n()
const text = computed(() => {
  const e = props.error
  if (!e) return ''
  if (e instanceof ApiError) return te('errors.' + e.code) ? t('errors.' + e.code) : e.message
  return t('common.error')
})
</script>

<template>
  <div v-if="error" class="border border-danger bg-danger/10 px-3 py-2 text-sm text-danger" role="alert">
    {{ text }}
  </div>
</template>
