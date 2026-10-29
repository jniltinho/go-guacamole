<script setup lang="ts">
import { Clock, FileText, PlayCircle, Server, UserRound } from '@lucide/vue'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { api } from '../api/client'
import type { HistoryEntry, List } from '../api/types'
import { human } from '../lib/format'
import EmptyState from './EmptyState.vue'
import Pager from './Pager.vue'
import ProtocolBadge from './ProtocolBadge.vue'
import RecordingPlayer from './RecordingPlayer.vue'
import TranscriptModal from './TranscriptModal.vue'

// Server access log (who accessed which server, when, for how long) and time
// spent per server / per user — the Java client's "History", for auditors.
// With userId / connectionId it becomes the edit screens' "History" tab (recent access only).
const props = defineProps<{ full?: boolean; userId?: string; connectionId?: string }>()
const scoped = computed(() => !!(props.userId || props.connectionId))
const { t, d } = useI18n()

interface Stat {
  key: string
  sessions: number
  totalMs: number
  distinct: number
  lastAccess: number
  active: number
}

type Tab = 'recent' | 'servers' | 'users'
const tab = ref<Tab>('recent')
const days = ref(30)
const q = ref('')
const offset = ref(0)
const limit = computed(() => (props.full ? 50 : 30))
const rows = ref<HistoryEntry[]>([])
const total = ref(0)
const stats = ref<{ servers: Stat[]; users: Stat[] }>({ servers: [], users: [] })
const now = ref(Date.now())
const player = ref<HistoryEntry | null>(null)
const transcript = ref<HistoryEntry | null>(null)
let timer = 0

const list = computed(() => (tab.value === 'servers' ? stats.value.servers : stats.value.users))
const maxTotal = computed(() => Math.max(1, ...list.value.map((s) => s.totalMs)))

async function load() {
  const from = Date.now() - days.value * 86400_000
  if (tab.value === 'recent') {
    const r = await api.get<List<HistoryEntry>>('/history', {
      q: q.value,
      from,
      userId: props.userId,
      connectionId: props.connectionId,
      limit: limit.value,
      offset: offset.value,
    })
    rows.value = r.items
    total.value = r.total
  } else {
    stats.value = await api.get('/history/stats', { days: days.value })
  }
}

watch([tab, days, offset], load)
watch(q, () => {
  offset.value = 0
  load()
})
onMounted(() => {
  load()
  timer = window.setInterval(() => (now.value = Date.now()), 1000)
})
onBeforeUnmount(() => clearInterval(timer))

const spent = (h: HistoryEntry) => (h.endAt ?? now.value) - h.startAt
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <div class="flex shrink-0 flex-wrap items-center gap-2 border-b border-line px-3 py-1.5">
      <div v-if="!scoped" class="flex" role="tablist">
        <button
          v-for="k in ['recent', 'servers', 'users'] as const"
          :key="k"
          role="tab"
          :aria-selected="tab === k"
          class="-mb-[7px] flex items-center gap-1.5 border-b-2 px-2.5 py-1.5 text-xs font-medium"
          :class="tab === k ? 'border-accent text-accent' : 'border-transparent text-muted hover:text-fg'"
          :data-testid="'access-tab-' + k"
          @click="tab = k"
        >
          <component
            :is="k === 'recent' ? Clock : k === 'servers' ? Server : UserRound"
            class="h-3.5 w-3.5"
          />
          {{ t('access.tabs.' + k) }}
        </button>
      </div>
      <input
        v-if="full && tab === 'recent'"
        v-model.lazy="q"
        type="search"
        class="input ml-auto max-w-64 py-1 text-xs"
        :placeholder="t('access.search')"
      />
      <select
        v-model.number="days"
        class="input w-auto py-1 text-xs"
        :class="{ 'ml-auto': !(full && tab === 'recent') }"
      >
        <option v-for="n in [1, 7, 30, 90, 365]" :key="n" :value="n">{{ t('access.days', { n }, n) }}</option>
      </select>
    </div>

    <div class="min-h-0 flex-1 overflow-auto">
      <template v-if="tab === 'recent'">
        <table v-if="rows.length" class="table" data-testid="access-log">
          <thead>
            <tr>
              <th>{{ t('sessions.user') }}</th>
              <th>{{ t('access.server') }}</th>
              <th v-if="full">{{ t('common.protocol') }}</th>
              <th>{{ t('sessions.start') }}</th>
              <th v-if="full">{{ t('sessions.end') }}</th>
              <th>{{ t('access.timeSpent') }}</th>
              <th v-if="full">{{ t('sessions.clientIp') }}</th>
              <th v-if="full">{{ t('recordings.column') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="h in rows" :key="h.id">
              <td class="font-medium">{{ h.username }}</td>
              <td>
                <span class="conn-name">{{ h.connectionName }}</span>
                <ProtocolBadge v-if="!full" :protocol="h.protocol" class="ml-1.5 scale-90" />
              </td>
              <td v-if="full"><ProtocolBadge :protocol="h.protocol" /></td>
              <td class="whitespace-nowrap text-muted">{{ d(h.startAt, 'long') }}</td>
              <td v-if="full" class="whitespace-nowrap text-muted">
                {{ h.endAt ? d(h.endAt, 'long') : '—' }}
              </td>
              <td class="whitespace-nowrap font-mono">
                {{ human(spent(h)) }}
                <span
                  v-if="!h.endAt"
                  class="ml-1 inline-block h-2 w-2 bg-success"
                  :title="t('sessions.running')"
                />
              </td>
              <td v-if="full" class="font-mono text-xs">{{ h.clientIp }}</td>
              <td v-if="full">
                <div class="flex gap-1">
                  <button
                    v-if="h.hasRecording"
                    class="btn btn-sm"
                    :title="t('recordings.screen')"
                    @click="player = h"
                  >
                    <PlayCircle class="h-3.5 w-3.5" />
                  </button>
                  <button
                    v-if="h.hasTypescript"
                    class="btn btn-sm"
                    :title="t('recordings.transcript')"
                    @click="transcript = h"
                  >
                    <FileText class="h-3.5 w-3.5" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
        <EmptyState v-else :text="t('access.none')" />
      </template>

      <ul v-else-if="list.length" class="divide-y divide-line" :data-testid="'access-' + tab">
        <li v-for="s in list" :key="s.key" class="px-3 py-2">
          <div class="flex items-baseline justify-between gap-3 text-sm">
            <span class="conn-name truncate font-medium">{{ s.key }}</span>
            <span class="shrink-0 font-mono font-semibold">{{ human(s.totalMs) }}</span>
          </div>
          <div class="my-1 h-1.5 bg-surface-2">
            <div class="h-full bg-accent" :style="{ width: `${(s.totalMs / maxTotal) * 100}%` }" />
          </div>
          <div class="flex flex-wrap gap-x-3 text-xs text-muted">
            <span>{{ t('access.sessions', { n: s.sessions }, s.sessions) }}</span>
            <span>{{
              t(tab === 'servers' ? 'access.users' : 'access.servers', { n: s.distinct }, s.distinct)
            }}</span>
            <span>{{ t('access.last') }} {{ d(s.lastAccess, 'long') }}</span>
            <span v-if="s.active" class="text-success">● {{ t('access.activeNow', { n: s.active }) }}</span>
          </div>
        </li>
      </ul>
      <EmptyState v-else :text="t('access.none')" />
    </div>
    <Pager v-if="full && tab === 'recent'" v-model="offset" :total="total" :limit="limit" />
    <RecordingPlayer v-if="player" :entry="player" @close="player = null" />
    <TranscriptModal v-if="transcript" :entry="transcript" @close="transcript = null" />
  </div>
</template>
