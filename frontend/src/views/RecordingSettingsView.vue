<script setup lang="ts">
import {
  CheckCircle2,
  Cloud,
  HardDrive,
  ListChecks,
  ShieldCheck,
  Trash2,
  TriangleAlert,
  XCircle,
} from '@lucide/vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { api, ApiError } from '../api/client'
import type { RecordingDefaults } from '../api/types'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import ErrorBox from '../components/ErrorBox.vue'
import PageHeader from '../components/PageHeader.vue'
import PageTabs from '../components/PageTabs.vue'
import Panel from '../components/Panel.vue'
import RecordingPolicyForm from '../components/RecordingPolicyForm.vue'
import RecordingStoragePanel from '../components/RecordingStoragePanel.vue'
import { useToast } from '../composables/useToast'

// Administration → Recording: global defaults, fail-closed exceptions,
// retention purge and storage usage (see recording-policy spec).
interface Totals {
  files: number
  bytes: number
}
interface Exception {
  until: number
  reason: string
  by: string
}
interface Settings {
  defaults: RecordingDefaults
  exceptions: { global?: Exception; connections: Record<string, Exception> }
  state: {
    lastPurge?: { at: number; video: Totals; text: Totals; orphans: Totals; missing: number; errors: number }
    usage?: {
      at: number
      video: Totals
      text: Totals
      top: Array<{ connectionId: string; connectionName: string; bytes: number }>
    }
    inventory?: Array<{
      connectionName: string
      video: boolean
      text: boolean
      keys: boolean
      removed: string[]
    }>
  }
  available: boolean
  unavailableReason?: string
  directory: string
  fromSettings: boolean
}

const { t, d } = useI18n()
const { show } = useToast()
const s = ref<Settings | null>(null)
const form = reactive({
  video: false,
  text: false,
  keys: false,
  videoDays: 90 as number | null,
  textDays: 90 as number | null,
  notice: true,
})
const exc = reactive({ minutes: 60, reason: '' })
const error = ref<unknown>(null)
const busy = ref(false)
const preview = ref<{ recordings: number; bytes: number; token: string } | null>(null)

const mb = (b: number) => (b / 1048576).toFixed(1) + ' MB'
const now = Date.now()
const activeConnExceptions = computed(() =>
  Object.entries(s.value?.exceptions.connections ?? {}).filter(([, x]) => x.until > now),
)

async function load() {
  s.value = await api.get<Settings>('/settings/recording')
  const df = s.value.defaults
  Object.assign(form, {
    video: df.video,
    text: df.text,
    keys: df.keys,
    videoDays: df.retentionVideoDays,
    textDays: df.retentionTextDays,
    notice: !df.hideNotice,
  })
}

async function run(fn: () => Promise<unknown>, ok = t('common.saved')) {
  busy.value = true
  error.value = null
  try {
    await fn()
    show(ok)
    await load()
  } catch (e) {
    if (e instanceof ApiError && e.code === 'RETENTION_PREVIEW' && e.data)
      preview.value = e.data as typeof preview.value
    else error.value = e
  } finally {
    busy.value = false
  }
}

const save = (previewToken?: string) =>
  run(() =>
    api.put('/settings/recording', {
      video: form.video,
      text: form.text,
      keys: form.video && form.keys,
      retentionVideoDays: form.videoDays,
      retentionTextDays: form.textDays,
      hideNotice: !form.notice,
      previewToken,
    }),
  )

function confirmPreview() {
  const tok = preview.value?.token
  preview.value = null
  save(tok)
}

const grant = () =>
  run(() => api.put('/settings/recording/exception', { minutes: exc.minutes, reason: exc.reason }))
const clear = (connectionId?: string) =>
  run(() => api.put('/settings/recording/exception', { connectionId: connectionId ?? '', minutes: 0 }))
const purge = () => run(() => api.post('/recordings/purge'), t('recordingPolicy.purgeDone'))
const usage = () => run(() => api.post('/recordings/usage'), t('recordingPolicy.usageDone'))

onMounted(load)

type Tab = 'policy' | 'storage' | 'exceptions' | 'retention' | 'inventory'
const TAB_ICONS = {
  policy: ShieldCheck,
  storage: Cloud,
  exceptions: TriangleAlert,
  retention: HardDrive,
  inventory: ListChecks,
}
const tab = ref<Tab>('policy')
const tabs = computed(() =>
  (['policy', 'storage', 'exceptions', 'retention', 'inventory'] as const)
    .filter((id) => id !== 'inventory' || !!s.value?.state.inventory?.length)
    .map((id) => ({ id: id as Tab, label: t('recordingPolicy.tabs.' + id), icon: TAB_ICONS[id] })),
)
</script>

<template>
  <div class="page-fill page">
    <PageHeader :title="t('recordingPolicy.adminTitle')" :subtitle="t('recordingPolicy.adminSubtitle')" />
    <div v-if="s" class="flex min-h-0 flex-1 flex-col gap-4" data-testid="recording-settings">
      <PageTabs v-model="tab" name="recording" :tabs="tabs" :label="t('recordingPolicy.adminTitle')">
        <div v-if="tab === 'policy'" class="space-y-5">
          <Panel :title="t('recordingPolicy.status')">
            <div class="space-y-1 p-4 text-sm">
              <p class="flex items-center gap-2" :class="s.available ? 'text-success' : 'text-danger'">
                <component :is="s.available ? CheckCircle2 : XCircle" class="h-4 w-4" />
                {{ s.available ? t('recordingPolicy.available') : t('recordingPolicy.unavailable') }}
                <span v-if="s.unavailableReason" class="text-muted">— {{ s.unavailableReason }}</span>
              </p>
              <p class="text-xs text-muted">
                {{ t('recordingPolicy.directory') }}: <code>{{ s.directory }}</code>
              </p>
              <p v-if="!s.fromSettings" class="text-xs text-muted">{{ t('recordingPolicy.fromEnv') }}</p>
            </div>
          </Panel>

          <Panel :title="t('recordingPolicy.defaults')">
            <form class="space-y-4 p-4" @submit.prevent="save()">
              <RecordingPolicyForm
                v-model:video="form.video"
                v-model:text="form.text"
                v-model:keys="form.keys"
                v-model:video-days="form.videoDays"
                v-model:text-days="form.textDays"
              />
              <!-- transparency: on by default; turning it off is audited like every change here -->
              <label class="flex items-start gap-2 text-sm">
                <input v-model="form.notice" type="checkbox" class="mt-0.5" data-testid="rec-notice-toggle" />
                <span>
                  {{ t('recordingPolicy.showNotice') }}
                  <span class="block text-xs text-muted">{{ t('recordingPolicy.showNoticeHint') }}</span>
                </span>
              </label>
              <div class="flex justify-end">
                <button class="btn btn-primary" type="submit" :disabled="busy" data-testid="save">
                  {{ t('common.save') }}
                </button>
              </div>
            </form>
          </Panel>
        </div>

        <RecordingStoragePanel v-if="tab === 'storage'" />

        <Panel v-if="tab === 'exceptions'" :title="t('recordingPolicy.exceptions')">
          <div class="space-y-3 p-4">
            <p class="text-xs text-muted">{{ t('recordingPolicy.exceptionsHint') }}</p>
            <p
              v-if="s.exceptions.global && s.exceptions.global.until > now"
              class="flex items-center gap-2 text-sm text-warning"
            >
              {{ t('recordingPolicy.globalUntil', { at: d(s.exceptions.global.until, 'long') }) }} —
              {{ s.exceptions.global.reason }} ({{ s.exceptions.global.by }})
              <button class="btn btn-sm" @click="clear()">{{ t('recordingPolicy.clear') }}</button>
            </p>
            <ul v-if="activeConnExceptions.length" class="text-sm">
              <li v-for="[cid, x] in activeConnExceptions" :key="cid" class="flex items-center gap-2">
                #{{ cid }}: {{ x.reason }} · {{ d(x.until, 'long') }}
                <button class="btn btn-sm" @click="clear(cid)">{{ t('recordingPolicy.clear') }}</button>
              </li>
            </ul>
            <form class="flex flex-wrap items-end gap-2" @submit.prevent="grant">
              <div>
                <label class="label">{{ t('recordingPolicy.minutes') }}</label>
                <input
                  v-model.number="exc.minutes"
                  class="input w-28"
                  type="number"
                  min="1"
                  max="1440"
                  required
                />
              </div>
              <div class="min-w-60 flex-1">
                <label class="label">{{ t('recordingPolicy.reason') }}</label>
                <input v-model="exc.reason" class="input" maxlength="256" required />
              </div>
              <button class="btn" type="submit" :disabled="busy">{{ t('recordingPolicy.grant') }}</button>
            </form>
          </div>
        </Panel>

        <Panel v-if="tab === 'retention'" :title="t('recordingPolicy.storage')">
          <template #actions>
            <button class="btn btn-sm" :disabled="busy" @click="usage">
              <HardDrive class="h-3.5 w-3.5" /> {{ t('recordingPolicy.recalculate') }}
            </button>
            <button class="btn btn-sm" :disabled="busy" data-testid="purge-now" @click="purge">
              <Trash2 class="h-3.5 w-3.5" /> {{ t('recordingPolicy.purgeNow') }}
            </button>
          </template>
          <div class="grid gap-4 p-4 text-sm md:grid-cols-2">
            <div v-if="s.state.usage">
              <p class="font-semibold">
                {{ t('recordingPolicy.usage') }} · {{ d(s.state.usage.at, 'long') }}
              </p>
              <p>
                {{ t('recordingPolicy.kinds.video') }}: {{ mb(s.state.usage.video.bytes) }} ({{
                  s.state.usage.video.files
                }})
              </p>
              <p>
                {{ t('recordingPolicy.kinds.text') }}: {{ mb(s.state.usage.text.bytes) }} ({{
                  s.state.usage.text.files
                }})
              </p>
              <ul class="mt-2 text-xs text-muted">
                <li v-for="c in s.state.usage.top" :key="c.connectionId">
                  {{ c.connectionName }} — {{ mb(c.bytes) }}
                </li>
              </ul>
            </div>
            <p v-else class="text-muted">{{ t('recordingPolicy.noUsage') }}</p>
            <div v-if="s.state.lastPurge">
              <p class="font-semibold">
                {{ t('recordingPolicy.lastPurge') }} · {{ d(s.state.lastPurge.at, 'long') }}
              </p>
              <p>
                {{ t('recordingPolicy.kinds.video') }}: {{ s.state.lastPurge.video.files }} ·
                {{ t('recordingPolicy.kinds.text') }}: {{ s.state.lastPurge.text.files }} ·
                {{ t('recordingPolicy.orphans') }}: {{ s.state.lastPurge.orphans.files }}
              </p>
              <p v-if="s.state.lastPurge.errors" class="text-danger">
                {{ t('recordingPolicy.purgeErrors', { n: s.state.lastPurge.errors }) }}
              </p>
            </div>
          </div>
        </Panel>

        <Panel v-if="tab === 'inventory'" :title="t('recordingPolicy.inventory')">
          <ul class="space-y-1 p-4 text-sm">
            <li v-for="(it, i) in s.state.inventory" :key="i">
              <strong>{{ it.connectionName }}</strong
              >:
              {{
                (['video', 'text', 'keys'] as const)
                  .filter((k) => it[k])
                  .map((k) => t('recordingPolicy.kinds.' + k))
                  .join(' · ') || t('recordingPolicy.nothing')
              }}
              <span class="text-xs text-muted">({{ it.removed.join(', ') }})</span>
            </li>
          </ul>
        </Panel>
      </PageTabs>

      <ErrorBox :error="error" />
      <p v-if="error instanceof ApiError && error.field" class="text-sm text-danger">{{ error.message }}</p>
    </div>
    <ConfirmDialog
      :open="!!preview"
      danger
      :title="t('recordingPolicy.previewTitle')"
      :message="
        t('recordingPolicy.previewMessage', {
          n: preview?.recordings ?? 0,
          bytes: ((preview?.bytes ?? 0) / 1048576).toFixed(1),
        })
      "
      @confirm="confirmPreview"
      @cancel="preview = null"
    />
  </div>
</template>
