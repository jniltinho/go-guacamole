<script setup lang="ts">
import {
  Activity,
  ChevronsLeft,
  ChevronsRight,
  FileText,
  FolderTree,
  LayoutDashboard,
  Monitor,
  PlayCircle,
  Settings,
  ShieldCheck,
  Users,
  UsersRound,
} from '@lucide/vue'
import { computed, onBeforeUnmount, onMounted, ref, type Component } from 'vue'
import { useI18n } from 'vue-i18n'

import { useAuth } from '../stores/auth'
import AppLogo from './AppLogo.vue'

const props = defineProps<{ mobileOpen: boolean }>()
const collapsed = defineModel<boolean>('collapsed', { default: false })
const emit = defineEmits<{ navigate: [] }>()
const auth = useAuth()
const { t, d } = useI18n()

interface Item {
  to: string
  label: string
  icon: Component
  perm?: string
}

const items = computed<Item[]>(() =>
  (
    [
      { to: '/dashboard', label: 'nav.dashboard', icon: LayoutDashboard },
      { to: '/connections', label: 'nav.connections', icon: Monitor },
      { to: '/sessions', label: 'nav.sessions', icon: PlayCircle },
      { to: '/users', label: 'nav.users', icon: Users, perm: 'ADMINISTER' },
      { to: '/groups', label: 'nav.groups', icon: UsersRound, perm: 'ADMINISTER' },
      {
        to: '/connection-groups',
        label: 'nav.connectionGroups',
        icon: FolderTree,
        perm: 'CREATE_CONNECTION',
      },
      { to: '/permissions', label: 'nav.permissions', icon: ShieldCheck, perm: 'ADMINISTER' },
      { to: '/audit', label: 'nav.audit', icon: FileText, perm: 'AUDIT' },
      { to: '/settings', label: 'nav.settings', icon: Settings },
    ] as Item[]
  ).filter((i) => !i.perm || auth.has(i.perm)),
)

const version = computed(() => (auth.me?.build.version ?? 'dev').replace(/^v/, ''))
const now = ref(new Date())
let timer = 0
onMounted(() => (timer = window.setInterval(() => (now.value = new Date()), 15000)))
onBeforeUnmount(() => clearInterval(timer))
</script>

<template>
  <aside
    class="fixed inset-y-0 left-0 z-40 flex flex-col border-r border-line bg-sidebar text-sidebar-fg transition-transform lg:static lg:translate-x-0"
    :class="[collapsed ? 'w-16' : 'w-64', props.mobileOpen ? 'translate-x-0' : '-translate-x-full']"
    :aria-label="t('nav.menu')"
  >
    <div class="flex h-16 items-center gap-3 px-4">
      <AppLogo class="h-8 w-8 shrink-0" />
      <span v-if="!collapsed" class="truncate text-xl font-bold tracking-tight">
        <span class="text-sidebar-active-fg">Go</span><span>Guacamole</span>
      </span>
    </div>
    <nav class="flex-1 overflow-y-auto px-2 py-3">
      <RouterLink
        v-for="i in items"
        :key="i.to"
        :to="i.to"
        class="nav-item mb-1 flex items-center gap-3 px-3 py-2.5 text-[0.95rem] font-medium"
        :title="collapsed ? t(i.label) : undefined"
        @click="emit('navigate')"
      >
        <component :is="i.icon" class="h-5 w-5 shrink-0" />
        <span v-if="!collapsed" class="truncate">{{ t(i.label) }}</span>
      </RouterLink>
    </nav>
    <div v-if="!collapsed" class="m-3 border border-white/10 bg-black/15 p-3 text-xs">
      <div class="mb-1 flex items-center gap-2 text-sm font-semibold">
        <Activity class="h-4 w-4" /> {{ t('app.poweredBy') }}
      </div>
      <div class="opacity-80">GoGuacamole v{{ version }}</div>
      <div class="theme-clock mt-2 font-mono">{{ d(now, 'time') }}</div>
    </div>
    <button
      class="hidden items-center justify-center gap-2 border-t border-white/10 py-2 text-xs opacity-80 hover:opacity-100 lg:flex"
      :title="collapsed ? t('nav.expand') : t('nav.collapse')"
      @click="collapsed = !collapsed"
    >
      <ChevronsRight v-if="collapsed" class="h-4 w-4" />
      <template v-else><ChevronsLeft class="h-4 w-4" /> {{ t('nav.collapse') }}</template>
    </button>
  </aside>
</template>

<style scoped>
.nav-item {
  color: var(--sidebar-fg);
}
.nav-item:hover {
  background: color-mix(in srgb, var(--sidebar-active) 25%, transparent);
}
.nav-item.router-link-active {
  background: var(--sidebar-active);
  color: var(--sidebar-active-fg);
}
</style>
