<script setup lang="ts">
import { FileQuestion } from '@lucide/vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<template>
  <div class="mx-auto mt-16 max-w-md text-center">
    <FileQuestion class="mx-auto mb-4 h-14 w-14 text-muted" />
    <h1 class="text-2xl font-bold">{{ t('pages.notFound') }}</h1>
    <p class="mt-2 text-muted">{{ t('pages.notFoundText') }}</p>
    <RouterLink to="/dashboard" class="btn btn-primary mt-6">{{ t('nav.dashboard') }}</RouterLink>
  </div>
</template>
