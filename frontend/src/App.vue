<script setup lang="ts">
import { computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'

import MatrixRain from './components/MatrixRain.vue'
import ToastHost from './components/ToastHost.vue'
import { useEffects } from './composables/useEffects'
import { useTheme } from './composables/useTheme'
import { setLocale } from './i18n'
import AppLayout from './layouts/AppLayout.vue'
import { useAuth } from './stores/auth'

const route = useRoute()
const auth = useAuth()
const { t } = useI18n()
const { theme, setTheme, inClient } = useTheme()
const { setEffects } = useEffects()

const bare = computed(() => !!route.meta.bare)
// the new page starts at the top of the scrolling area
const toTop = () => document.querySelector('main')?.scrollTo(0, 0)

// server-side preferences win over the browser cache once signed in
watch(
  () => auth.me?.preferences,
  (p) => {
    if (p?.theme) setTheme(p.theme)
    if (p) setEffects(p.effects ?? '')
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
    <!-- page change as in AD Manager: fade out, then fade in rising (motion tokens follow the Effects setting) -->
    <RouterView v-slot="{ Component, route: r }">
      <Transition name="page" mode="out-in" @before-enter="toTop">
        <!-- keyed wrapper: views with more than one root element animate too -->
        <div :key="r.path" class="h-full">
          <component :is="Component" />
        </div>
      </Transition>
    </RouterView>
  </AppLayout>
  <ToastHost v-if="!inClient" />
</template>
