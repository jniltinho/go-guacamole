<script setup lang="ts">
import { ChevronDown, LogOut, Menu, Search, Settings } from '@lucide/vue'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
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
// "/" focuses the search from anywhere (console habit), unless the user is typing
const searchInput = ref<HTMLInputElement>()
function onSlash(e: KeyboardEvent) {
  const el = e.target as HTMLElement | null
  if (e.key !== '/' || e.ctrlKey || e.metaKey || e.altKey) return
  if (el && (el.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName))) return
  if (document.documentElement.classList.contains('in-client')) return
  if (document.querySelector('[aria-modal="true"]')) return // never steal focus from a dialog
  if (localStorage.getItem('gg-slash-search') === 'off') return // WCAG 2.1.4: can be turned off
  e.preventDefault()
  searchInput.value?.focus()
}
onMounted(() => window.addEventListener('keydown', onSlash))
onBeforeUnmount(() => window.removeEventListener('keydown', onSlash))
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
  <header
    class="topbar flex h-10 shrink-0 items-center gap-3 border-b border-line bg-surface px-3 shadow-elev-1"
  >
    <button class="btn btn-sm lg:hidden" :aria-label="t('nav.menu')" @click="emit('toggle-menu')">
      <Menu class="h-4 w-4" />
    </button>
    <form class="relative w-full max-w-xs" role="search" @submit.prevent="search">
      <Search class="pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-muted" />
      <input
        ref="searchInput"
        v-model="q"
        class="input h-7 py-0.5 pl-8 text-[13px]"
        aria-keyshortcuts="/"
        type="search"
        :placeholder="t(auth.isAdmin ? 'common.searchPlaceholder' : 'common.searchConnections')"
        :aria-label="t('common.search')"
      />
    </form>
    <div class="ml-auto flex items-center gap-2">
      <ThemePicker />
      <LocalePicker />
      <span class="mx-1 hidden h-4 w-px bg-line sm:block" aria-hidden="true" />
      <div class="relative">
        <button
          class="flex items-center gap-2 px-1.5 py-0.5 hover:bg-current/10 aria-expanded:bg-current/10"
          :aria-expanded="open"
          aria-haspopup="menu"
          @click="open = !open"
        >
          <span
            class="flex h-7 w-7 items-center justify-center bg-accent text-[15px] font-bold text-accent-fg"
          >
            {{ initials }}
          </span>
          <span class="hidden text-left leading-tight sm:block">
            <span class="block text-[13px] leading-4 font-semibold">{{ name }}</span>
            <span class="block text-[11px] leading-4 text-muted">{{
              auth.isAdmin ? t('common.role.admin') : t('common.role.user')
            }}</span>
          </span>
          <ChevronDown class="h-3.5 w-3.5 text-muted" />
        </button>
        <div
          v-if="open"
          class="card absolute right-0 z-50 mt-1 w-52 py-1 shadow-elev-3"
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
