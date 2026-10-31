<script setup lang="ts">
import { Clock, FileText, Lock, PlayCircle, RotateCw, Server, UserRound } from '@lucide/vue'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { api } from '../api/client'
import type { HistoryEntry, List } from '../api/types'
import { askConfirm, askText } from '../lib/dialogs'
import { human } from '../lib/format'
import { useAuth } from '../stores/auth'
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

// recording states shown next to the playback buttons (kind:state)
const auth = useAuth()
const canHold = computed(() => auth.has('AUDIT'))
function recStates(h: HistoryEntry): string[] {
  const out: string[] = []
  if (h.recException) out.push('video:exception')
  for (const [k, cap, files] of [
    ['video', h.videoCapture, h.videoFiles],
    ['text', h.textCapture, h.textFiles],
  ] as const) {
    if (cap === 'failed') out.push(k + ':failed')
    if (files === 'purged') out.push(k + ':expired')
    if (files === 'missing') out.push(k + ':missing')
  }
  // object storage (openspec add-async-jobs-object-storage)
  for (const [k, st] of [
    ['video', h.videoStorage],
    ['text', h.textStorage],
  ] as const) {
    if (st === 'remote') out.push(k + ':stored')
    if (st === 'owed' || st === 'uploading') out.push(k + ':uploading')
    if (st === 'failed') out.push(k + ':uploadFailed')
    if (st === 'delete_pending') out.push(k + ':deletePending')
  }
  return out
}

const storageError = (h: HistoryEntry) =>
  [h.videoStorageError, h.textStorageError].filter(Boolean).join(' · ')
const canRetry = (h: HistoryEntry) =>
  auth.isAdmin && (h.videoStorage === 'failed' || h.textStorage === 'failed')

async function retry(h: HistoryEntry) {
  await api.post(`/history/${h.id}/storage/retry`, {})
  load()
}

async function toggleHold(h: HistoryEntry) {
  if (h.holdAt) {
    if (!(await askConfirm(t('recordingPolicy.releaseConfirm'), { title: t('recordingPolicy.hold') }))) return
    await api.del(`/history/${h.id}/hold`)
  } else {
    const reason = await askText(t('recordingPolicy.holdReason'), {
      title: t('recordingPolicy.hold'),
      maxlength: 256,
    })
    if (!reason) return
    await api.put(`/history/${h.id}/hold`, { reason })
  }
  load()
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <!-- a narrow card shows the tabs as icons (names as tooltips): the band keeps one line and one height -->
    <div class="card-band @container">
      <div v-if="!scoped" class="flex" role="tablist">
        <button
          v-for="k in ['recent', 'servers', 'users'] as const"
          :key="k"
          role="tab"
          :aria-selected="tab === k"
          :class="tab === k ? 'border-accent text-accent' : 'border-transparent text-muted hover:text-fg'"
          :data-testid="'access-tab-' + k"
          :title="t('access.tabs.' + k)"
          @click="tab = k"
        >
          <component
            :is="k === 'recent' ? Clock : k === 'servers' ? Server : UserRound"
            class="h-3.5 w-3.5"
          />
          <span class="@max-[34rem]:sr-only">{{ t('access.tabs.' + k) }}</span>
        </button>
      </div>
      <input
        v-if="full && tab === 'recent'"
        v-model.lazy="q"
        type="search"
        class="input ml-auto max-w-64 py-1 text-xs"
        :placeholder="t('access.search')"
      />
      <!-- compact tabs and select: they share one line in the dashboard card, even with monospace themes -->
      <select
        v-model.number="days"
        class="input w-auto px-2 py-1 text-xs"
        :class="{ 'ml-auto': !(full && tab === 'recent') }"
      >
        <option v-for="n in [1, 7, 30, 90, 365]" :key="n" :value="n">{{ t('access.days', { n }, n) }}</option>
      </select>
    </div>

    <div class="min-h-0 flex-1 overflow-auto">
      <template v-if="tab === 'recent'">
        <table v-if="rows.length" class="table" :class="{ 'table-sm': !full }" data-testid="access-log">
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
                <!-- the badge first, at one width: badges and names line up on every row -->
                <span class="flex items-center gap-2">
                  <ProtocolBadge v-if="!full" :protocol="h.protocol" class="w-10 shrink-0 text-center" />
                  <span class="conn-name truncate">{{ h.connectionName }}</span>
                </span>
              </td>
              <td v-if="full"><ProtocolBadge :protocol="h.protocol" /></td>
              <td class="whitespace-nowrap text-muted">{{ d(h.startAt, 'compact') }}</td>
              <td v-if="full" class="whitespace-nowrap text-muted">
                {{ h.endAt ? d(h.endAt, 'compact') : '—' }}
              </td>
              <td class="whitespace-nowrap font-mono">
                {{ human(spent(h)) }}
                <span
                  v-if="!h.endAt"
                  class="live-pulse ml-1 inline-block h-2 w-2 bg-success"
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
                  <span
                    v-for="st in recStates(h)"
                    :key="st"
                    class="border border-line px-1 text-[10px] uppercase"
                    :class="/failed$/i.test(st) ? 'text-danger' : 'text-muted'"
                    :title="st.endsWith('uploadFailed') ? storageError(h) : undefined"
                    >{{
                      t('recordingPolicy.state.' + st.split(':')[1], {
                        kind: t('recordingPolicy.kinds.' + st.split(':')[0]),
                      })
                    }}</span
                  >
                  <button
                    v-if="canRetry(h)"
                    class="btn btn-sm"
                    :title="t('recordingStorage.retry')"
                    :data-testid="'storage-retry-' + h.id"
                    @click="retry(h)"
                  >
                    <RotateCw class="h-3.5 w-3.5" />
                  </button>
                  <button
                    v-if="canHold && (h.hasRecording || h.hasTypescript || h.holdAt)"
                    class="btn btn-sm"
                    :class="{ 'text-warning': h.holdAt }"
                    :title="
                      h.holdAt
                        ? t('recordingPolicy.heldBy', { by: h.holdBy, reason: h.holdReason })
                        : t('recordingPolicy.hold')
                    "
                    :data-testid="'hold-' + h.id"
                    @click="toggleHold(h)"
                  >
                    <Lock class="h-3.5 w-3.5" />
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
