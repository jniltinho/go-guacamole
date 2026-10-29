<script setup lang="ts">
import { Monitor, MoreHorizontal, Play, RefreshCw, User, Users } from '@lucide/vue'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { api } from '../api/client'
import type { Dashboard } from '../api/types'
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
  <div class="mx-auto max-w-[2200px]">
    <div class="mb-5 flex flex-wrap items-end justify-between gap-3">
      <div>
        <h1 class="text-2xl font-bold md:text-3xl">{{ t('dashboard.title') }}</h1>
        <p class="mt-1 text-muted">{{ t('dashboard.subtitle') }}</p>
      </div>
      <div class="flex items-center gap-3 text-sm text-muted">
        <span>{{ d(now, 'long') }}</span>
        <button class="btn btn-sm" :aria-label="t('common.refresh')" @click="load">
          <RefreshCw class="h-4 w-4" />
        </button>
      </div>
    </div>
    <ErrorBox :error="error" class="mb-4" />
    <div v-if="data" class="space-y-5">
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
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

      <div class="grid grid-cols-1 gap-5 xl:grid-cols-3">
        <Panel :title="t('dashboard.available')" class="xl:col-span-2">
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
                <th>{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="c in data.connections" :key="c.id">
                <td>
                  <div class="flex items-center gap-3">
                    <ConnIcon :protocol="c.protocol" class="h-7 w-7 shrink-0 text-muted" />
                    <div class="min-w-0">
                      <div class="conn-name truncate font-semibold">{{ c.name }}</div>
                      <div class="host truncate text-xs text-muted">{{ c.hostname }}</div>
                    </div>
                  </div>
                </td>
                <td><ProtocolBadge :protocol="c.protocol" /></td>
                <td class="text-muted">{{ c.groupPath.join(' / ') || t('common.root') }}</td>
                <td><StatusDot :status="c.status" /></td>
                <td>
                  <div class="flex items-center gap-2">
                    <button class="btn btn-primary btn-sm" @click="connect(c)">
                      {{ t('connections.connect') }}
                    </button>
                    <RouterLink
                      v-if="c.canEdit"
                      :to="`/connections/${c.id}/edit`"
                      class="btn btn-sm"
                      :aria-label="t('common.edit')"
                    >
                      <MoreHorizontal class="h-4 w-4" />
                    </RouterLink>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
          <EmptyState v-else :text="t('dashboard.noConnections')" />
        </Panel>

        <Panel :title="t('dashboard.recentActivity')">
          <template #actions>
            <RouterLink v-if="auth.has('AUDIT')" to="/audit" class="text-sm font-medium text-accent">{{
              t('common.viewAll')
            }}</RouterLink>
          </template>
          <ul v-if="data.recentActivity.length" class="divide-y divide-line">
            <ActivityItem v-for="e in data.recentActivity" :key="e.id" :event="e" />
          </ul>
          <EmptyState v-else :text="t('dashboard.noActivity')" />
        </Panel>
      </div>

      <div class="grid grid-cols-1 gap-5 xl:grid-cols-2">
        <Panel :title="t('dashboard.activeSessions')">
          <template #actions>
            <RouterLink to="/sessions" class="text-sm font-medium text-accent">{{
              t('common.viewAll')
            }}</RouterLink>
          </template>
          <table v-if="data.activeSessions.length" class="table">
            <thead>
              <tr>
                <th>{{ t('dashboard.user') }}</th>
                <th>{{ t('dashboard.connection') }}</th>
                <th>{{ t('common.protocol') }}</th>
                <th>{{ t('dashboard.startTime') }}</th>
                <th>{{ t('dashboard.duration') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="s in data.activeSessions" :key="s.id">
                <td><span class="mr-2 inline-block h-2 w-2 bg-success" />{{ s.username }}</td>
                <td>{{ s.connectionName }}</td>
                <td><ProtocolBadge :protocol="s.protocol" /></td>
                <td>{{ d(s.startAt, 'time') }}</td>
                <td class="font-mono">{{ duration(now - s.startAt) }}</td>
              </tr>
            </tbody>
          </table>
          <EmptyState v-else :text="t('dashboard.noSessions')" />
        </Panel>

        <Panel v-if="data.permissionOverview" :title="t('dashboard.permissionOverview')">
          <template #actions>
            <RouterLink to="/permissions" class="text-sm font-medium text-accent">{{
              t('common.viewAll')
            }}</RouterLink>
          </template>
          <div class="flex border-b border-line px-4" role="tablist">
            <button
              v-for="k in ['user', 'group'] as const"
              :key="k"
              role="tab"
              :aria-selected="tab === k"
              class="-mb-px border-b-2 px-4 py-2 text-sm font-medium"
              :class="tab === k ? 'border-accent text-accent' : 'border-transparent text-muted'"
              @click="tab = k"
            >
              {{ t(k === 'user' ? 'dashboard.tabs.users' : 'dashboard.tabs.groups') }}
            </button>
          </div>
          <table class="table">
            <thead>
              <tr>
                <th>{{ t('common.name') }}</th>
                <th>{{ t('dashboard.type') }}</th>
                <th>{{ t('nav.permissions') }}</th>
                <th v-if="tab === 'user'">{{ t('dashboard.inheritedFrom') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="p in overview" :key="p.type + p.id">
                <td class="font-medium">{{ p.name }}</td>
                <td class="text-muted">{{ t(p.type === 'user' ? 'dashboard.user' : 'common.group') }}</td>
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
    </div>
  </div>
</template>
