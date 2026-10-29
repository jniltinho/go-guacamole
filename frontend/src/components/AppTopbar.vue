<script setup lang="ts">
import { ChevronDown, LogOut, Menu, Search, Settings } from '@lucide/vue'
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import { useAuth } from '../stores/auth'
import LocalePicker from './LocalePicker.vue'
import ThemePicker from './ThemePicker.vue'

const emit = defineEmits<{ 'toggle-menu': [] }>()
const auth = useAuth()
const router = useRouter()
const { t } = useI18n()
const q = ref('')
const open = ref(false)

const name = computed(() => auth.user?.fullName || auth.user?.username || '')
const initials = computed(() =>
  name.value
    .split(/[\s._-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0]!.toUpperCase())
    .join(''),
)

function search() {
  router.push({ path: '/connections', query: q.value ? { q: q.value } : {} })
}

async function logout() {
  await auth.logout()
  router.push('/login')
}
</script>

<template>
  <header class="topbar flex h-16 shrink-0 items-center gap-3 border-b border-line bg-surface px-3 md:px-6">
    <button class="btn btn-sm lg:hidden" :aria-label="t('nav.menu')" @click="emit('toggle-menu')">
      <Menu class="h-4 w-4" />
    </button>
    <form class="relative max-w-xl flex-1" role="search" @submit.prevent="search">
      <Search class="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-muted" />
      <input
        v-model="q"
        class="input pl-9"
        type="search"
        :placeholder="t('common.searchPlaceholder')"
        :aria-label="t('common.search')"
      />
    </form>
    <div class="ml-auto flex items-center gap-2">
      <ThemePicker />
      <LocalePicker />
      <span class="mx-1.5 hidden h-5 w-px bg-line sm:block" aria-hidden="true" />
      <div class="relative">
        <button
          class="flex items-center gap-2 px-2 py-1 hover:bg-surface-2"
          :aria-expanded="open"
          aria-haspopup="menu"
          @click="open = !open"
        >
          <span class="flex h-9 w-9 items-center justify-center bg-accent text-sm font-bold text-accent-fg">
            {{ initials }}
          </span>
          <span class="hidden text-left leading-tight sm:block">
            <span class="block text-sm font-semibold">{{ name }}</span>
            <span class="block text-xs text-muted">{{
              auth.isAdmin ? t('common.role.admin') : t('common.role.user')
            }}</span>
          </span>
          <ChevronDown class="h-4 w-4 text-muted" />
        </button>
        <div
          v-if="open"
          class="card absolute right-0 z-50 mt-1 w-52 py-1"
          role="menu"
          @click="open = false"
          @keydown.esc="open = false"
        >
          <RouterLink
            to="/settings"
            class="flex items-center gap-2 px-3 py-2 text-sm hover:bg-surface-2"
            role="menuitem"
          >
            <Settings class="h-4 w-4" /> {{ t('nav.settings') }}
          </RouterLink>
          <button
            class="flex w-full items-center gap-2 px-3 py-2 text-left text-sm text-danger hover:bg-surface-2"
            role="menuitem"
            @click="logout"
          >
            <LogOut class="h-4 w-4" /> {{ t('nav.logout') }}
          </button>
        </div>
      </div>
    </div>
  </header>
</template>
