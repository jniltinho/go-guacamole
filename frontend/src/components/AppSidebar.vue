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
    :class="[collapsed ? 'w-12' : 'w-52', props.mobileOpen ? 'translate-x-0' : '-translate-x-full']"
    :aria-label="t('nav.menu')"
  >
    <div class="flex h-12 items-center gap-2 px-3">
      <AppLogo class="h-6 w-6 shrink-0" />
      <span v-if="!collapsed" class="truncate text-base font-bold tracking-tight">
        <span class="text-sidebar-active-fg">Go</span><span>Guacamole</span>
      </span>
    </div>
    <nav class="flex-1 overflow-y-auto px-1.5 py-2">
      <RouterLink
        v-for="i in items"
        :key="i.to"
        :to="i.to"
        class="nav-item mb-px flex items-center gap-2.5 px-2.5 py-1.5 text-[13px] font-medium"
        :title="collapsed ? t(i.label) : undefined"
        @click="emit('navigate')"
      >
        <component :is="i.icon" class="h-4 w-4 shrink-0" :stroke-width="1.75" />
        <span v-if="!collapsed" class="truncate">{{ t(i.label) }}</span>
      </RouterLink>
    </nav>
    <div
      v-if="!collapsed"
      class="flex items-center justify-between gap-2 border-t border-white/10 px-3 py-2 text-[11px] opacity-70"
    >
      <span class="flex items-center gap-1.5"><Activity class="h-3 w-3" /> GoGuacamole v{{ version }}</span>
      <span class="theme-clock font-mono">{{ d(now, 'time') }}</span>
    </div>
    <button
      class="hidden items-center justify-center gap-1.5 border-t border-white/10 py-1.5 text-[11px] opacity-70 hover:opacity-100 lg:flex"
      :title="collapsed ? t('nav.expand') : t('nav.collapse')"
      @click="collapsed = !collapsed"
    >
      <ChevronsRight v-if="collapsed" class="h-3.5 w-3.5" />
      <template v-else><ChevronsLeft class="h-3.5 w-3.5" /> {{ t('nav.collapse') }}</template>
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
