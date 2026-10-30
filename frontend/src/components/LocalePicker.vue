<script setup lang="ts">
import { Check, Languages } from '@lucide/vue'
import { useI18n } from 'vue-i18n'

import { LOCALES, setLocale } from '../i18n'
import { useAuth } from '../stores/auth'
import IconMenu from './IconMenu.vue'

const { t, locale } = useI18n()
const auth = useAuth()
const short: Record<string, string> = { en: 'EN', 'pt-BR': 'PT' }

function choose(l: string, close: () => void) {
  setLocale(l)
  if (auth.me) auth.savePreferences({ locale: l }).catch(() => {})
  close()
}
</script>

<template>
  <IconMenu :label="t('common.language')" testid="locale-picker">
    <template #icon><Languages class="h-4 w-4" :stroke-width="1.5" /></template>
    <template #default="{ close }">
      <button
        v-for="l in LOCALES"
        :key="l"
        type="button"
        role="menuitemradio"
        :aria-checked="l === locale"
        class="menu-item"
        :data-locale-option="l"
        @click="choose(l, close)"
      >
        <span class="w-7 font-mono text-xs text-muted">{{ short[l] }}</span>
        <span class="flex-1">{{ t('locales.' + l) }}</span>
        <Check v-if="l === locale" class="h-4 w-4 text-accent" />
      </button>
    </template>
  </IconMenu>
</template>

<style scoped>
.menu-item {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 0.6rem;
  padding: 0.45rem 0.75rem;
  font-size: 0.85rem;
  text-align: left;
  color: var(--fg);
  background: transparent;
  border: 0;
  cursor: pointer;
}
.menu-item:hover,
.menu-item:focus-visible {
  background: var(--surface-2);
}
.menu-item:focus-visible {
  outline: 2px solid var(--focus);
  outline-offset: -2px;
}
</style>
