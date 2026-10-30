<script setup lang="ts">
import { ShieldX } from '@lucide/vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<template>
  <div class="mx-auto mt-16 max-w-md text-center">
    <ShieldX class="mx-auto mb-4 h-14 w-14 text-danger" />
    <h1 class="text-2xl font-bold text-on-bg">{{ t('pages.forbidden') }}</h1>
    <p class="mt-2 text-muted">{{ t('pages.forbiddenText') }}</p>
    <RouterLink to="/dashboard" class="btn btn-primary mt-6">{{ t('nav.dashboard') }}</RouterLink>
  </div>
</template>
