<script setup lang="ts">
import { Check, Palette } from '@lucide/vue'
import { useI18n } from 'vue-i18n'

import { useTheme } from '../composables/useTheme'
import { useAuth } from '../stores/auth'
import IconMenu from './IconMenu.vue'

const { themes, themeId, setTheme } = useTheme()
const auth = useAuth()
const { t, te } = useI18n()

function choose(id: string, close: () => void) {
  setTheme(id)
  if (auth.me) auth.savePreferences({ theme: id }).catch(() => {})
  close()
}
</script>

<template>
  <IconMenu :label="t('common.theme')" testid="theme-picker">
    <template #icon><Palette class="h-4 w-4" :stroke-width="1.5" /></template>
    <template #default="{ close }">
      <button
        v-for="th in themes"
        :key="th.id"
        type="button"
        role="menuitemradio"
        :aria-checked="th.id === themeId"
        class="menu-item"
        :data-theme-option="th.id"
        @click="choose(th.id, close)"
      >
        <!-- swatch rendered with the theme's own tokens -->
        <span :data-theme="th.id" class="swatch theme-swatch">
          <span style="background: var(--sidebar)" />
          <span style="background: var(--bg)" />
          <span style="background: var(--accent)" />
        </span>
        <span class="flex-1 truncate">{{ te('themes.' + th.id) ? t('themes.' + th.id) : th.label }}</span>
        <Check v-if="th.id === themeId" class="h-4 w-4 text-accent" />
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
  padding: 0.4rem 0.75rem;
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
  outline: none;
}
.swatch {
  display: inline-flex;
  flex-shrink: 0;
  border: 1px solid var(--border);
}
.swatch > span {
  display: block;
  height: 1rem;
  width: 0.55rem;
}
</style>
