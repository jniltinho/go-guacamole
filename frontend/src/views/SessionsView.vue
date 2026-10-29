<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { api } from '../api/client'
import type { HistoryEntry, List, TunnelInfo } from '../api/types'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import EmptyState from '../components/EmptyState.vue'
import ErrorBox from '../components/ErrorBox.vue'
import PageHeader from '../components/PageHeader.vue'
import Pager from '../components/Pager.vue'
import Panel from '../components/Panel.vue'
import ProtocolBadge from '../components/ProtocolBadge.vue'
import { duration } from '../lib/format'
import { useAuth } from '../stores/auth'

const { t, d } = useI18n()
const auth = useAuth()
const tab = ref<'active' | 'history'>('active')
const active = ref<TunnelInfo[]>([])
const history = ref<HistoryEntry[]>([])
const total = ref(0)
const offset = ref(0)
const q = ref('')
const limit = 25
const now = ref(Date.now())
const error = ref<unknown>(null)
const kill = ref<TunnelInfo | null>(null)
let timer = 0

async function load() {
  try {
    if (tab.value === 'active') active.value = (await api.get<List<TunnelInfo>>('/sessions')).items
    else {
      const r = await api.get<List<HistoryEntry>>('/history', { q: q.value, limit, offset: offset.value })
      history.value = r.items
      total.value = r.total
    }
    error.value = null
  } catch (e) {
    error.value = e
  }
}

async function doKill() {
  const s = kill.value
  kill.value = null
  if (!s) return
  try {
    await api.del(`/sessions/${s.id}`)
    load()
  } catch (e) {
    error.value = e
  }
}

watch([tab, offset], load)
watch(q, () => {
  offset.value = 0
  load()
})
onMounted(() => {
  load()
  timer = window.setInterval(() => {
    now.value = Date.now()
    if (tab.value === 'active' && now.value % 10000 < 1000) load()
  }, 1000)
})
onBeforeUnmount(() => clearInterval(timer))
</script>

<template>
  <div class="mx-auto max-w-[2200px]">
    <PageHeader :title="t('sessions.title')" :subtitle="t('sessions.subtitle')" />
    <ErrorBox :error="error" class="mb-4" />
    <Panel>
      <div class="flex border-b border-line px-4" role="tablist">
        <button
          v-for="k in ['active', 'history'] as const"
          :key="k"
          role="tab"
          :aria-selected="tab === k"
          class="-mb-px border-b-2 px-4 py-2.5 text-sm font-medium"
          :class="tab === k ? 'border-accent text-accent' : 'border-transparent text-muted'"
          @click="tab = k"
        >
          {{ t('sessions.' + k) }}
        </button>
        <input
          v-if="tab === 'history'"
          v-model.lazy="q"
          type="search"
          class="input my-1.5 ml-auto max-w-xs"
          :placeholder="t('common.search')"
        />
      </div>
      <template v-if="tab === 'active'">
        <table v-if="active.length" class="table">
          <thead>
            <tr>
              <th>{{ t('sessions.user') }}</th>
              <th>{{ t('sessions.connection') }}</th>
              <th>{{ t('common.protocol') }}</th>
              <th>{{ t('sessions.clientIp') }}</th>
              <th>{{ t('sessions.start') }}</th>
              <th>{{ t('sessions.duration') }}</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="s in active" :key="s.id">
              <td><span class="mr-2 inline-block h-2 w-2 bg-success" />{{ s.username }}</td>
              <td>{{ s.connectionName }}</td>
              <td><ProtocolBadge :protocol="s.protocol" /></td>
              <td class="font-mono text-xs">{{ s.clientIp }}</td>
              <td>{{ d(s.startAt, 'long') }}</td>
              <td class="font-mono">{{ duration(now - s.startAt) }}</td>
              <td class="text-right">
                <button
                  v-if="auth.isAdmin || s.userId === auth.user?.id"
                  class="btn btn-sm btn-danger"
                  @click="kill = s"
                >
                  {{ t('sessions.kill') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
        <EmptyState v-else :text="t('sessions.none')" />
      </template>
      <template v-else>
        <table v-if="history.length" class="table">
          <thead>
            <tr>
              <th>{{ t('sessions.user') }}</th>
              <th>{{ t('sessions.connection') }}</th>
              <th>{{ t('common.protocol') }}</th>
              <th>{{ t('sessions.clientIp') }}</th>
              <th>{{ t('sessions.start') }}</th>
              <th>{{ t('sessions.duration') }}</th>
              <th>{{ t('sessions.reason') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="h in history" :key="h.id">
              <td>{{ h.username }}</td>
              <td>{{ h.connectionName }}</td>
              <td><ProtocolBadge :protocol="h.protocol" /></td>
              <td class="font-mono text-xs">{{ h.clientIp }}</td>
              <td>{{ d(h.startAt, 'long') }}</td>
              <td class="font-mono">{{ h.endAt ? duration(h.endAt - h.startAt) : t('sessions.running') }}</td>
              <td class="text-muted">{{ h.closeReason ?? '—' }}</td>
            </tr>
          </tbody>
        </table>
        <EmptyState v-else :text="t('common.noResults')" />
        <Pager v-model="offset" :total="total" :limit="limit" />
      </template>
    </Panel>
    <ConfirmDialog
      :open="!!kill"
      danger
      :title="t('sessions.kill')"
      :message="
        kill ? t('sessions.killConfirm', { user: kill.username, connection: kill.connectionName }) : ''
      "
      @confirm="doKill"
      @cancel="kill = null"
    />
  </div>
</template>
