<script setup lang="ts">
import {
  Disc,
  KeyRound,
  ListChecks,
  ChevronsLeft,
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
import { computed, type Component } from 'vue'
import { useI18n } from 'vue-i18n'

import { useAuth } from '../stores/auth'
import AppLogo from './AppLogo.vue'

const props = defineProps<{ mobileOpen: boolean }>()
const collapsed = defineModel<boolean>('collapsed', { default: false })
const emit = defineEmits<{ navigate: [] }>()
const auth = useAuth()
const { t } = useI18n()

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
      { to: '/recording', label: 'nav.recording', icon: Disc, perm: 'ADMINISTER' },
      { to: '/authentication', label: 'nav.authentication', icon: KeyRound, perm: 'ADMINISTER' },
      { to: '/settings', label: 'nav.settings', icon: Settings },
    ] as Item[]
  ).filter((i) => !i.perm || auth.has(i.perm)),
)

const version = computed(() => (auth.me?.build.version ?? 'dev').replace(/^v/, ''))
</script>

<template>
  <aside
    class="sidebar fixed inset-y-0 left-0 z-40 flex flex-col overflow-hidden border-r border-line bg-sidebar text-sidebar-fg lg:static lg:translate-x-0"
    :class="[
      collapsed ? 'is-collapsed w-12' : 'w-52',
      props.mobileOpen ? 'translate-x-0' : '-translate-x-full',
    ]"
    :aria-label="t('nav.menu')"
  >
    <div class="flex h-10 items-center gap-2 px-3">
      <AppLogo class="h-6 w-6 shrink-0" />
      <span class="side-label truncate text-base font-bold tracking-tight">
        <span class="font-black">Go</span><span>Guacamole</span>
      </span>
    </div>
    <nav class="flex-1 overflow-y-auto py-2">
      <RouterLink
        v-for="i in items"
        :key="i.to"
        :to="i.to"
        class="nav-item mb-px flex items-center gap-2 px-3 py-1.5 text-[13px] font-medium"
        :title="collapsed ? t(i.label) : undefined"
        @click="emit('navigate')"
      >
        <!-- mx-1: the icon is centered in the collapsed bar and the label lines up with the logo text -->
        <component :is="i.icon" class="mx-1 h-4 w-4 shrink-0" :stroke-width="1.75" />
        <span class="side-label truncate">{{ t(i.label) }}</span>
      </RouterLink>
      <!-- job dashboard: a server page (asynqmon), opened in its own tab -->
      <a
        v-if="auth.me?.jobsDashboard"
        href="/asynqmon"
        target="_blank"
        rel="noopener"
        class="nav-item mb-px flex items-center gap-2 px-3 py-1.5 text-[13px] font-medium"
        :title="collapsed ? t('nav.jobs') : undefined"
        data-testid="nav-jobs"
      >
        <ListChecks class="mx-1 h-4 w-4 shrink-0" :stroke-width="1.75" />
        <span class="side-label truncate">{{ t('nav.jobs') }}</span>
      </a>
    </nav>
    <!-- the release only (the app name is in the header; full version in the tooltip) -->
    <p
      class="side-label truncate border-t border-white/10 px-3 py-1.5 text-right font-mono text-[11px] tracking-wide"
      :title="`v${version}`"
    >
      v{{ version.replace(/-\d+-g[0-9a-f]+/, '') }}
    </p>
    <button
      class="hidden items-center gap-2 border-t border-white/10 px-4 py-1.5 text-[11px] lg:flex"
      :title="collapsed ? t('nav.expand') : t('nav.collapse')"
      @click="collapsed = !collapsed"
    >
      <ChevronsLeft class="chevron h-3.5 w-3.5 shrink-0" />
      <!-- hidden while collapsed, but still the button's accessible name -->
      <span class="side-label whitespace-nowrap">{{ collapsed ? t('nav.expand') : t('nav.collapse') }}</span>
    </button>
  </aside>
</template>

<style scoped>
/* collapsing is a drawer: the width eases, labels stay in place, clipped and fading */
.sidebar {
  transition:
    width var(--dur-3) var(--ease-in-out),
    transform var(--dur-3) var(--ease-in-out);
}
.side-label {
  transition: opacity var(--dur-2) var(--ease-out);
}
.is-collapsed .side-label {
  opacity: 0;
}
.chevron {
  transition: transform var(--dur-3) var(--ease-in-out);
}
.is-collapsed .chevron {
  transform: rotate(180deg);
}
.nav-item {
  color: var(--sidebar-fg);
}
.nav-item:hover {
  background: var(--sidebar-hover);
  color: var(--sidebar-hover-fg);
  font-weight: 700;
  box-shadow: inset 3px 0 0 var(--sidebar-hover-fg);
}
.nav-item.router-link-active {
  background: var(--sidebar-active);
  color: var(--sidebar-active-fg);
}
</style>
