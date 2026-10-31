<script setup lang="ts">
import { Monitor, Pencil, Play, RefreshCw, User, Users } from '@lucide/vue'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { api } from '../api/client'
import type { Dashboard } from '../api/types'
import AccessLog from '../components/AccessLog.vue'
import ActivityItem from '../components/ActivityItem.vue'
import ConnIcon from '../components/ConnIcon.vue'
import EmptyState from '../components/EmptyState.vue'
import ErrorBox from '../components/ErrorBox.vue'
import Panel from '../components/Panel.vue'
import ProtocolBadge from '../components/ProtocolBadge.vue'
import StatCard from '../components/StatCard.vue'
import StatusDot from '../components/StatusDot.vue'
import { useConnect } from '../composables/useConnect'
import { duration } from '../lib/format'
import { useAuth } from '../stores/auth'

const { t, d } = useI18n()
const auth = useAuth()
const { connect } = useConnect()
const data = ref<Dashboard | null>(null)
const error = ref<unknown>(null)
const now = ref(Date.now())
const tab = ref<'user' | 'group'>('user')
let timer = 0

async function load() {
  try {
    data.value = await api.get<Dashboard>('/dashboard')
    error.value = null
  } catch (e) {
    error.value = e
  }
  now.value = Date.now()
}
onMounted(() => {
  load()
  timer = window.setInterval(() => {
    now.value = Date.now()
  }, 1000)
})
onBeforeUnmount(() => clearInterval(timer))

const overview = computed(() => (data.value?.permissionOverview ?? []).filter((p) => p.type === tab.value))
</script>

<template>
  <div class="page-fill page">
    <div class="flex shrink-0 flex-wrap items-end justify-between gap-3">
      <div>
        <h1 class="text-xl font-bold text-on-bg md:text-2xl">{{ t('dashboard.title') }}</h1>
        <p class="mt-0.5 text-sm text-on-bg-muted">{{ t('dashboard.subtitle') }}</p>
      </div>
      <div class="flex items-center gap-3 text-xs text-on-bg-muted">
        <span>{{ d(now, 'long') }}</span>
        <button class="btn btn-sm" :aria-label="t('common.refresh')" @click="load">
          <RefreshCw class="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
    <ErrorBox :error="error" />
    <template v-if="data">
      <div class="grid shrink-0 grid-cols-2 gap-3 xl:grid-cols-4">
        <StatCard
          :label="t('dashboard.cards.connections')"
          :value="data.counts.connections"
          :icon="Monitor"
          color="var(--accent)"
          to="/connections"
        />
        <StatCard
          :label="t('dashboard.cards.activeSessions')"
          :value="data.counts.activeSessions"
          :icon="Play"
          color="var(--success)"
          to="/sessions"
        />
        <StatCard
          v-if="data.counts.users !== undefined"
          :label="t('dashboard.cards.users')"
          :value="data.counts.users"
          :icon="User"
          color="var(--proto-rdp)"
          :to="auth.isAdmin ? '/users' : undefined"
        />
        <StatCard
          v-if="data.counts.groups !== undefined"
          :label="t('dashboard.cards.groups')"
          :value="data.counts.groups"
          :icon="Users"
          color="var(--proto-vnc)"
          :to="auth.isAdmin ? '/groups' : undefined"
        />
      </div>

      <div class="dash-grid min-h-0 flex-1">
        <Panel fill :title="t('dashboard.available')" class="area-conns">
          <template #actions>
            <RouterLink
              v-if="auth.has('CREATE_CONNECTION')"
              to="/connections/new"
              class="btn btn-primary btn-sm"
            >
              + {{ t('connections.new') }}
            </RouterLink>
          </template>
          <table v-if="data.connections.length" class="table">
            <thead>
              <tr>
                <th>{{ t('common.name') }}</th>
                <th>{{ t('common.protocol') }}</th>
                <th>{{ t('common.group') }}</th>
                <th>{{ t('common.status') }}</th>
                <th class="text-right">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="c in data.connections" :key="c.id">
                <td>
                  <div class="flex items-center gap-2.5">
                    <ConnIcon :protocol="c.protocol" class="h-4 w-4 shrink-0 text-muted" />
                    <!-- the host is a tooltip (and read by screen readers): one line per row -->
                    <span class="conn-name truncate font-semibold" :title="c.hostname">
                      {{ c.name }}<span class="host sr-only"> — {{ c.hostname }}</span>
                    </span>
                  </div>
                </td>
                <td><ProtocolBadge :protocol="c.protocol" /></td>
                <td class="text-muted">{{ c.groupPath.join(' / ') || t('common.root') }}</td>
                <td><StatusDot :status="c.status" /></td>
                <td>
                  <div class="flex justify-end gap-1.5">
                    <button class="btn btn-primary btn-sm" @click="connect(c)">
                      {{ t('connections.connect') }}
                    </button>
                    <RouterLink
                      v-if="c.canEdit"
                      :to="`/connections/${c.id}/edit`"
                      class="btn btn-sm"
                      :aria-label="t('common.edit')"
                      :title="t('common.edit')"
                    >
                      <Pencil class="h-4 w-4" />
                    </RouterLink>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
          <EmptyState v-else :text="t('dashboard.noConnections')" />
        </Panel>

        <Panel fill :title="t('dashboard.recentActivity')" class="area-activity">
          <template #actions>
            <RouterLink v-if="auth.has('AUDIT')" to="/audit" class="text-xs font-medium text-accent">{{
              t('common.viewAll')
            }}</RouterLink>
          </template>
          <ul v-if="data.recentActivity.length" class="divide-y divide-line">
            <ActivityItem v-for="e in data.recentActivity" :key="e.id" :event="e" />
          </ul>
          <EmptyState v-else :text="t('dashboard.noActivity')" />
        </Panel>

        <Panel fill :title="t('dashboard.activeSessions')" class="area-sessions">
          <template #actions>
            <RouterLink to="/sessions" class="text-xs font-medium text-accent">{{
              t('common.viewAll')
            }}</RouterLink>
          </template>
          <!-- same band height as the cards beside it: the three tables start on one line -->
          <template #toolbar>
            <div class="card-band text-xs text-muted" data-testid="sessions-now">
              <span class="flex items-center gap-2">
                <span v-if="data.activeSessions.length" class="live-pulse inline-block h-2 w-2 bg-success" />
                {{ t('dashboard.sessionsNow', data.activeSessions.length) }}
              </span>
            </div>
          </template>
          <table v-if="data.activeSessions.length" class="table table-sm">
            <thead>
              <tr>
                <th>{{ t('dashboard.user') }}</th>
                <th>{{ t('dashboard.connection') }}</th>
                <th>{{ t('dashboard.startTime') }}</th>
                <th>{{ t('dashboard.duration') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="s in data.activeSessions" :key="s.id">
                <td><span class="live-pulse mr-1.5 inline-block h-2 w-2 bg-success" />{{ s.username }}</td>
                <td>
                  <span class="flex items-center gap-2">
                    <ProtocolBadge :protocol="s.protocol" class="w-10 shrink-0 text-center" />
                    <span class="truncate">{{ s.connectionName }}</span>
                  </span>
                </td>
                <td>{{ d(s.startAt, 'time') }}</td>
                <td class="font-mono">{{ duration(now - s.startAt) }}</td>
              </tr>
            </tbody>
          </table>
          <EmptyState v-else :text="t('dashboard.noSessions')" />
        </Panel>

        <Panel
          v-if="auth.has('AUDIT')"
          fill
          :title="t('access.title')"
          class="area-access"
          data-testid="dashboard-access"
        >
          <template #actions>
            <RouterLink
              :to="{ path: '/audit', query: { tab: 'access' } }"
              class="text-xs font-medium text-accent"
              >{{ t('common.viewAll') }}</RouterLink
            >
          </template>
          <AccessLog />
        </Panel>

        <Panel
          v-if="data.permissionOverview"
          fill
          :title="t('dashboard.permissionOverview')"
          class="area-perms"
        >
          <template #actions>
            <RouterLink to="/permissions" class="text-xs font-medium text-accent">{{
              t('common.viewAll')
            }}</RouterLink>
          </template>
          <template #toolbar>
            <div class="card-band">
              <div role="tablist">
                <button
                  v-for="k in ['user', 'group'] as const"
                  :key="k"
                  role="tab"
                  :aria-selected="tab === k"
                  :class="
                    tab === k ? 'border-accent text-accent' : 'border-transparent text-muted hover:text-fg'
                  "
                  @click="tab = k"
                >
                  {{ t(k === 'user' ? 'dashboard.tabs.users' : 'dashboard.tabs.groups') }}
                </button>
              </div>
            </div>
          </template>
          <table class="table table-sm">
            <thead>
              <tr>
                <th>{{ t('common.name') }}</th>
                <th>{{ t('nav.permissions') }}</th>
                <th v-if="tab === 'user'">{{ t('dashboard.inheritedFrom') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="p in overview" :key="p.type + p.id">
                <td class="font-medium">{{ p.name }}</td>
                <td>
                  <RouterLink
                    :to="{ path: '/permissions', query: { subject: `${p.type}:${p.id}` } }"
                    class="text-accent"
                  >
                    {{ t('dashboard.permissionsCount', { n: p.permissions }) }}
                  </RouterLink>
                </td>
                <td v-if="tab === 'user'" class="text-muted">{{ p.inheritedFrom.join(', ') || '—' }}</td>
              </tr>
            </tbody>
          </table>
        </Panel>
      </div>
    </template>
  </div>
</template>

<style scoped>
/* 2 rows that share the remaining height; areas collapse when a panel is hidden */
.dash-grid {
  display: grid;
  gap: 0.5rem;
  grid-template-columns: 1fr;
}
@media (min-width: 1280px) {
  .dash-grid {
    grid-template-columns: repeat(12, minmax(0, 1fr));
    grid-template-rows: minmax(0, 1fr) minmax(0, 1fr);
  }
  .area-conns {
    grid-column: span 8;
  }
  .area-activity {
    grid-column: span 4;
  }
  .area-sessions {
    grid-column: span 4;
  }
  .area-access {
    grid-column: span 5;
  }
  .area-perms {
    grid-column: span 3;
  }
  .area-sessions:last-child {
    grid-column: span 12;
  }
  .area-access:last-child {
    grid-column: span 8;
  }
}
@media (max-width: 1279px) {
  .dash-grid > * {
    min-height: 18rem;
  }
}
</style>
