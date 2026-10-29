<script setup lang="ts">
import { computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'

import MatrixRain from './components/MatrixRain.vue'
import { useTheme } from './composables/useTheme'
import { setLocale } from './i18n'
import AppLayout from './layouts/AppLayout.vue'
import { useAuth } from './stores/auth'

const route = useRoute()
const auth = useAuth()
const { t } = useI18n()
const { theme, setTheme, inClient } = useTheme()

const bare = computed(() => !!route.meta.bare)

// server-side preferences win over the browser cache once signed in
watch(
  () => auth.me?.preferences,
  (p) => {
    if (p?.theme) setTheme(p.theme)
    if (p?.locale) setLocale(p.locale)
  },
  { immediate: true },
)

watch(
  () => [route.meta.title, t('app.name')] as const,
  ([title]) => {
    document.title = title ? `${t(title)} · ${t('app.name')}` : t('app.name')
  },
  { immediate: true },
)
</script>

<template>
  <MatrixRain v-if="theme.animated && !inClient" />
  <RouterView v-if="bare" />
  <AppLayout v-else>
    <RouterView />
  </AppLayout>
</template>
