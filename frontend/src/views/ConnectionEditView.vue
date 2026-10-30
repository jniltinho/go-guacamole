<script setup lang="ts">
import {
  CircleAlert,
  CircleCheck,
  ClipboardList,
  FolderSync,
  History,
  KeyRound,
  LoaderCircle,
  Monitor,
  Network,
  PlugZap,
  Server,
  Settings2,
  SlidersHorizontal,
  SquareTerminal,
  Trash2,
  TriangleAlert,
  Video,
} from '@lucide/vue'
import { computed, onMounted, reactive, ref, watch, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import { api, ApiError } from '../api/client'
import type { Connection, ConnectionGroup, ConnectionRecording, List, Protocol } from '../api/types'
import AccessLog from '../components/AccessLog.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import ErrorBox from '../components/ErrorBox.vue'
import PageHeader from '../components/PageHeader.vue'
import PageTabs from '../components/PageTabs.vue'
import Panel from '../components/Panel.vue'
import ProtocolBadge from '../components/ProtocolBadge.vue'
import RecordingPanel from '../components/RecordingPanel.vue'
import { useAuth } from '../stores/auth'
import { PROTOCOL_FIELDS, PROTOCOLS, SECTIONS, type Field } from '../lib/protocols'

type Section = (typeof SECTIONS)[number]
type Tab = 'connection' | 'display' | 'session' | 'sftp' | 'advanced' | 'recording' | 'history'

const route = useRoute()
const auth = useAuth()
const router = useRouter()
const { t, te } = useI18n()
const id = computed(() => route.params.id as string | undefined)

const form = reactive({
  name: '',
  protocol: 'ssh' as Protocol,
  parentId: '' as string,
  disabled: false,
  guacdHost: '',
  guacdPort: '' as string | number,
  guacdEncryption: '',
  maxConnections: '' as string | number,
  maxConnectionsPerUser: '' as string | number,
})
const params = reactive<Record<string, string>>({})
const storedSecrets = ref<Set<string>>(new Set())
const cleared = ref<Set<string>>(new Set())
const groups = ref<ConnectionGroup[]>([])
const error = ref<unknown>(null)
const errField = ref('')
const busy = ref(false)
const confirmDelete = ref(false)
const recording = ref<ConnectionRecording | null>(null)

const fields = computed(() => PROTOCOL_FIELDS[form.protocol])
const label = (f: Field) => (te('params.' + f.name) ? t('params.' + f.name) : f.name)

// each form section lives in one tab; a tab without fields for the protocol is hidden
const SECTION_TAB: Record<Section, Tab> = {
  network: 'connection',
  authentication: 'connection',
  display: 'display',
  terminal: 'display',
  clipboard: 'display',
  session: 'session',
  behavior: 'session',
  sftp: 'sftp',
  advanced: 'advanced',
}
const SECTION_ICON: Record<Section, Component> = {
  network: Network,
  authentication: KeyRound,
  display: Monitor,
  terminal: SquareTerminal,
  clipboard: ClipboardList,
  session: Settings2,
  behavior: SlidersHorizontal,
  sftp: FolderSync,
  advanced: SlidersHorizontal,
}
const sectionsOf = (tab: Tab) =>
  SECTIONS.filter((s) => SECTION_TAB[s] === tab && fields.value.some((f) => f.section === s))

const tab = ref<Tab>('connection')
const tabs = computed(() => [
  { id: 'connection' as Tab, label: t('connections.form.tabConnection'), icon: Server },
  ...(sectionsOf('display').length
    ? [{ id: 'display' as Tab, label: t('connections.form.tabDisplay'), icon: Monitor }]
    : []),
  ...(sectionsOf('session').length
    ? [{ id: 'session' as Tab, label: t('connections.form.tabSession'), icon: Settings2 }]
    : []),
  ...(sectionsOf('sftp').length
    ? [
        {
          id: 'sftp' as Tab,
          label: t('connections.form.sftp'),
          icon: FolderSync,
          dot: params['enable-sftp'] === 'true',
        },
      ]
    : []),
  { id: 'advanced' as Tab, label: t('connections.form.tabAdvanced'), icon: SlidersHorizontal },
  ...(id.value && recording.value
    ? [{ id: 'recording' as Tab, label: t('connections.form.tabRecording'), icon: Video }]
    : []),
  ...(id.value && auth.has('AUDIT')
    ? [{ id: 'history' as Tab, label: t('sessions.history'), icon: History }]
    : []),
])
const formTab = computed(() => tab.value !== 'recording' && tab.value !== 'history')
// switching the protocol of a new connection can remove the open tab
watch(tabs, (ts) => {
  if (!ts.some((x) => x.id === tab.value)) tab.value = 'connection'
})

// host fields take most of the row so the port sits beside them; numbers stay narrow
function span(f: Field) {
  if (f.type === 'textarea') return 'md:col-span-4'
  if (f.name === 'hostname' || f.name.endsWith('-hostname')) return 'md:col-span-3'
  if (f.type === 'number') return 'md:col-span-1'
  return 'md:col-span-2'
}
const inputs = (s: Section) => fields.value.filter((f) => f.section === s && f.type !== 'bool')
// the switch that turns a section on (Enable SFTP) comes first; the other options follow the inputs
const LEAD = 'enable-sftp'
const lead = (s: Section) => fields.value.filter((f) => f.section === s && f.name === LEAD)
const toggles = (s: Section) =>
  fields.value.filter((f) => f.section === s && f.type === 'bool' && f.name !== LEAD)

onMounted(async () => {
  groups.value = (await api.get<List<ConnectionGroup>>('/connection-groups')).items
  if (!id.value) {
    if (typeof route.query.parentId === 'string') form.parentId = route.query.parentId
    return
  }
  const c = await api.get<Connection>(`/connections/${id.value}`)
  recording.value = c.recording ?? null
  Object.assign(form, {
    name: c.name,
    protocol: c.protocol,
    parentId: c.parentId ?? '',
    disabled: c.disabled,
    guacdHost: c.guacdHost ?? '',
    guacdPort: c.guacdPort ?? '',
    guacdEncryption: c.guacdEncryption ?? '',
    maxConnections: c.maxConnections ?? '',
    maxConnectionsPerUser: c.maxConnectionsPerUser ?? '',
  })
  for (const [k, v] of Object.entries(c.parameters ?? {})) {
    if (typeof v === 'object') storedSecrets.value.add(k)
    else params[k] = v
  }
})

function num(v: string | number): number | null {
  return v === '' || v === null ? null : Number(v)
}

function body() {
  const out: Record<string, string | null> = {}
  for (const f of fields.value) {
    const v = params[f.name]
    if (f.secret) {
      if (v) out[f.name] = v
      else if (cleared.value.has(f.name)) out[f.name] = null
      // omitted secret → server keeps the stored value
    } else if (v !== undefined && v !== '') {
      out[f.name] = v
    }
  }
  return {
    name: form.name,
    protocol: form.protocol,
    parentId: form.parentId || null,
    disabled: form.disabled,
    guacdHost: form.guacdHost || null,
    guacdPort: num(form.guacdPort),
    guacdEncryption: form.guacdEncryption || null,
    maxConnections: num(form.maxConnections),
    maxConnectionsPerUser: num(form.maxConnectionsPerUser),
    parameters: out,
  }
}

// a rejected field opens the tab that holds it
function showError(e: unknown) {
  error.value = e
  if (!(e instanceof ApiError) || !e.field) return
  errField.value = e.field
  const f = fields.value.find((x) => 'parameters.' + x.name === e.field)
  tab.value = f
    ? SECTION_TAB[f.section]
    : e.field.startsWith('guacd') || e.field.startsWith('max')
      ? 'advanced'
      : 'connection'
}

async function save() {
  busy.value = true
  error.value = null
  errField.value = ''
  try {
    if (id.value) await api.put(`/connections/${id.value}`, body())
    else await api.post('/connections', body())
    router.push('/connections')
  } catch (e) {
    showError(e)
  } finally {
    busy.value = false
  }
}

interface TestResult {
  ok: boolean
  code?: string
  status?: number
  message?: string
  ms: number
}
const testing = ref(false)
const result = ref<TestResult | null>(null)
const resultText = computed(() => {
  const r = result.value
  if (!r) return ''
  if (r.ok)
    return t(r.code === 'UNVERIFIED' ? 'connections.test.unverified' : 'connections.test.ok', { ms: r.ms })
  if (r.code === 'CREDENTIALS_REQUIRED') {
    // guacd names the parameters it needs (username, domain, password): shown with the form's labels
    const names = (r.message ?? '').split(', ').map((n) => (te('params.' + n) ? t('params.' + n) : n))
    return t('connections.test.required', { fields: names.join(', ') })
  }
  if (r.code === 'KEY_PASSPHRASE') return t('connections.test.keyPassphrase')
  const key = 'connections.test.s' + r.status
  return te(key)
    ? t(key)
    : t('connections.test.other', { code: '0x' + (r.status ?? 0).toString(16).padStart(4, '0') })
})
const resultKind = computed(() =>
  !result.value ? '' : !result.value.ok ? 'fail' : result.value.code ? 'warn' : 'ok',
)

// a result describes the values it was run with: any edit (a secret marked for removal too) drops it, and an
// answer for values edited meanwhile is discarded
let revision = 0
watch(
  [params, form, cleared],
  () => {
    revision++
    result.value = null
  },
  { deep: true },
)

async function test() {
  testing.value = true
  result.value = null
  error.value = null
  errField.value = ''
  const sent = revision
  try {
    const r = await api.post<TestResult>('/connections/test', { ...body(), id: id.value })
    if (sent === revision) result.value = r
  } catch (e) {
    if (sent === revision) showError(e)
  } finally {
    testing.value = false
  }
}

async function remove() {
  confirmDelete.value = false
  try {
    await api.del(`/connections/${id.value}`)
    router.push('/connections')
  } catch (e) {
    error.value = e
  }
}

const invalid = (name: string) => errField.value === name || errField.value === 'parameters.' + name
</script>

<template>
  <div class="page-fill page">
    <PageHeader :title="id ? form.name || t('connections.edit') : t('connections.create')">
      <template #title-extra>
        <ProtocolBadge v-if="id" :protocol="form.protocol" />
        <span
          v-if="form.disabled"
          class="border border-current px-1.5 py-0.5 text-xs text-on-bg uppercase"
          data-testid="connection-disabled"
          >{{ t('connections.form.disabledBadge') }}</span
        >
      </template>
      <button v-if="id" class="btn btn-danger" @click="confirmDelete = true">
        <Trash2 class="h-4 w-4" /> {{ t('common.delete') }}
      </button>
    </PageHeader>
    <form
      class="flex min-h-0 flex-1 flex-col gap-4"
      data-testid="connection-form"
      @submit.prevent="formTab && save()"
    >
      <PageTabs
        v-model="tab"
        name="connection"
        :fill="tab === 'history'"
        :tabs="tabs"
        :label="t('connections.edit')"
      >
        <Panel v-if="formTab">
          <!-- general: name, protocol, folder and the on/off switch -->
          <section v-if="tab === 'connection'" class="conn-section">
            <h3 class="conn-heading"><Server class="h-4 w-4" /> {{ t('connections.form.general') }}</h3>
            <div class="grid gap-x-4 gap-y-3 md:grid-cols-4">
              <div class="md:col-span-2">
                <label class="label" for="f-name">{{ t('connections.form.name') }} *</label>
                <input
                  id="f-name"
                  v-model="form.name"
                  class="input"
                  :class="{ 'border-danger': invalid('name') }"
                  required
                  maxlength="128"
                />
              </div>
              <div>
                <label class="label" for="f-proto">{{ t('connections.form.protocol') }}</label>
                <select id="f-proto" v-model="form.protocol" class="input" :disabled="!!id">
                  <option v-for="p in PROTOCOLS" :key="p" :value="p">{{ p.toUpperCase() }}</option>
                </select>
              </div>
              <div>
                <label class="label" for="f-parent">{{ t('connections.form.parent') }}</label>
                <select id="f-parent" v-model="form.parentId" class="input">
                  <option value="">{{ t('common.root') }}</option>
                  <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option>
                </select>
              </div>
              <label class="conn-toggle md:col-span-4" :class="{ on: form.disabled }">
                <input v-model="form.disabled" type="checkbox" data-testid="connection-disable" />
                <span>
                  <span class="block font-medium">{{ t('connections.form.disabled') }}</span>
                  <span class="block text-xs text-muted">{{ t('connections.form.disabledHint') }}</span>
                </span>
              </label>
            </div>
          </section>

          <section v-for="s in sectionsOf(tab)" :key="s" class="conn-section">
            <h3 class="conn-heading">
              <component :is="SECTION_ICON[s]" class="h-4 w-4" /> {{ t('connections.form.' + s) }}
            </h3>
            <p v-if="s === 'sftp' && form.protocol === 'ssh'" class="mb-3 text-sm text-muted">
              {{ t('connections.form.sshSftpHint') }}
            </p>
            <div class="grid gap-x-4 gap-y-3 md:grid-cols-4">
              <label
                v-for="f in lead(s)"
                :key="f.name"
                class="conn-toggle md:col-span-4"
                :class="{ on: params[f.name] === 'true' }"
              >
                <input
                  type="checkbox"
                  :checked="params[f.name] === 'true'"
                  :data-testid="'param-' + f.name"
                  @change="params[f.name] = ($event.target as HTMLInputElement).checked ? 'true' : ''"
                />
                <span class="font-medium">{{ label(f) }}</span>
              </label>
              <div v-for="f in inputs(s)" :key="f.name" :class="span(f)">
                <label class="label" :for="'p-' + f.name">
                  {{ label(f) }}<span v-if="f.required" class="text-danger"> *</span>
                </label>
                <input
                  v-if="f.type === 'text' || f.type === 'number'"
                  :id="'p-' + f.name"
                  v-model="params[f.name]"
                  class="input"
                  :class="{ 'border-danger': invalid(f.name) }"
                  :type="f.type === 'number' ? 'number' : 'text'"
                  :placeholder="f.placeholder"
                  :required="f.required"
                  :data-testid="'param-' + f.name"
                />
                <template v-else-if="f.type === 'password' || f.type === 'textarea'">
                  <component
                    :is="f.type === 'textarea' ? 'textarea' : 'input'"
                    :id="'p-' + f.name"
                    :value="params[f.name] ?? ''"
                    class="input font-mono"
                    :class="{ 'border-danger': invalid(f.name) }"
                    :type="f.type === 'password' ? 'password' : undefined"
                    :rows="f.type === 'textarea' ? 4 : undefined"
                    autocomplete="new-password"
                    :placeholder="
                      storedSecrets.has(f.name) && !cleared.has(f.name) ? t('connections.form.secretSet') : ''
                    "
                    :data-testid="'param-' + f.name"
                    @input="params[f.name] = ($event.target as HTMLInputElement).value"
                  />
                  <label
                    v-if="storedSecrets.has(f.name)"
                    class="mt-1 flex items-center gap-2 text-xs text-muted"
                  >
                    <input
                      type="checkbox"
                      :checked="cleared.has(f.name)"
                      @change="cleared.has(f.name) ? cleared.delete(f.name) : cleared.add(f.name)"
                    />
                    {{
                      cleared.has(f.name)
                        ? t('connections.form.secretCleared')
                        : t('connections.form.clearSecret')
                    }}
                  </label>
                </template>
                <select v-else :id="'p-' + f.name" v-model="params[f.name]" class="input">
                  <option v-for="o in f.options" :key="o" :value="o">{{ o || '—' }}</option>
                </select>
              </div>
              <div v-if="toggles(s).length" class="grid gap-2 sm:grid-cols-2 md:col-span-4">
                <label
                  v-for="f in toggles(s)"
                  :key="f.name"
                  class="conn-toggle"
                  :class="{ on: params[f.name] === 'true' }"
                >
                  <input
                    type="checkbox"
                    :checked="params[f.name] === 'true'"
                    :data-testid="'param-' + f.name"
                    @change="params[f.name] = ($event.target as HTMLInputElement).checked ? 'true' : ''"
                  />
                  <span class="font-medium">{{ label(f) }}</span>
                </label>
              </div>
            </div>
          </section>

          <template v-if="tab === 'advanced'">
            <section class="conn-section">
              <h3 class="conn-heading"><Server class="h-4 w-4" /> {{ t('connections.form.guacd') }}</h3>
              <div class="grid gap-x-4 gap-y-3 md:grid-cols-4">
                <div class="md:col-span-2">
                  <label class="label" for="f-ghost">{{ t('connections.form.guacdHost') }}</label>
                  <input id="f-ghost" v-model="form.guacdHost" class="input" />
                </div>
                <div>
                  <label class="label" for="f-gport">{{ t('connections.form.guacdPort') }}</label>
                  <input
                    id="f-gport"
                    v-model="form.guacdPort"
                    class="input"
                    type="number"
                    min="1"
                    max="65535"
                  />
                </div>
                <div>
                  <label class="label" for="f-genc">{{ t('connections.form.guacdEncryption') }}</label>
                  <select id="f-genc" v-model="form.guacdEncryption" class="input">
                    <option value="">{{ t('connections.form.encryptionDefault') }}</option>
                    <option value="NONE">NONE</option>
                    <option value="SSL">SSL/TLS</option>
                  </select>
                </div>
              </div>
            </section>
            <section class="conn-section">
              <h3 class="conn-heading">
                <SlidersHorizontal class="h-4 w-4" /> {{ t('connections.form.limits') }}
              </h3>
              <div class="grid gap-x-4 gap-y-3 md:grid-cols-4">
                <div class="md:col-span-2">
                  <label class="label" for="f-max">{{ t('connections.form.maxConnections') }}</label>
                  <input
                    id="f-max"
                    v-model="form.maxConnections"
                    class="input"
                    type="number"
                    min="0"
                    :placeholder="t('connections.form.unlimited')"
                  />
                </div>
                <div class="md:col-span-2">
                  <label class="label" for="f-maxu">{{ t('connections.form.maxConnectionsPerUser') }}</label>
                  <input
                    id="f-maxu"
                    v-model="form.maxConnectionsPerUser"
                    class="input"
                    type="number"
                    min="0"
                    :placeholder="t('connections.form.unlimited')"
                  />
                </div>
              </div>
            </section>
          </template>
        </Panel>
        <Panel v-else-if="tab === 'history' && id" fill data-testid="history-panel">
          <AccessLog full :connection-id="id" />
        </Panel>
        <!-- stays mounted: it keeps the policy version it saved last, so a later save does not conflict -->
        <RecordingPanel
          v-if="id && recording"
          v-show="tab === 'recording'"
          :connection-id="id"
          :protocol="form.protocol"
          :recording="recording"
        />
      </PageTabs>

      <div
        v-if="result && formTab"
        class="conn-result shrink-0"
        :class="resultKind"
        role="status"
        data-testid="connection-test-result"
      >
        <component
          :is="resultKind === 'ok' ? CircleCheck : resultKind === 'warn' ? TriangleAlert : CircleAlert"
          class="mt-0.5 h-4 w-4 shrink-0"
        />
        <div>
          <p class="font-medium">{{ resultText }}</p>
          <p
            v-if="result.ok && form.protocol === 'rdp' && params.security !== 'nla'"
            class="text-xs opacity-80"
          >
            {{ t('connections.test.okRdp') }}
          </p>
          <p
            v-if="!result.ok && result.message && result.code !== 'CREDENTIALS_REQUIRED'"
            class="text-xs opacity-80"
          >
            {{ t('connections.test.detail', { message: result.message }) }}
          </p>
        </div>
      </div>
      <ErrorBox :error="error" class="shrink-0" />
      <div class="flex shrink-0 flex-wrap items-center gap-2">
        <button
          v-if="formTab"
          type="button"
          class="btn"
          :disabled="testing"
          data-testid="connection-test"
          @click="test"
        >
          <LoaderCircle v-if="testing" class="h-4 w-4 animate-spin" />
          <PlugZap v-else class="h-4 w-4" />
          {{ testing ? t('connections.test.running') : t('connections.test.button') }}
        </button>
        <div class="ml-auto flex gap-2">
          <RouterLink to="/connections" class="btn">{{ t('common.cancel') }}</RouterLink>
          <button v-if="formTab" class="btn btn-primary" type="submit" :disabled="busy" data-testid="save">
            {{ t('common.save') }}
          </button>
        </div>
      </div>
    </form>
    <ConfirmDialog
      :open="confirmDelete"
      danger
      :title="t('common.delete')"
      :message="t('connections.deleteConfirm', { name: form.name })"
      @confirm="remove"
      @cancel="confirmDelete = false"
    />
  </div>
</template>

<style scoped>
.conn-section {
  padding: 1rem 1.25rem 1.25rem;
}
.conn-section + .conn-section {
  border-top: 1px solid var(--border);
}
.conn-heading {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.875rem;
  font-size: 0.8125rem;
  font-weight: 600;
  letter-spacing: 0.02em;
  text-transform: uppercase;
  color: var(--muted);
}
/* checkbox options as bordered rows: the whole row is the target, the chosen ones are tinted */
.conn-toggle {
  display: flex;
  align-items: flex-start;
  gap: 0.625rem;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--border);
  font-size: 0.875rem;
  cursor: pointer;
  transition:
    background-color var(--dur-1) var(--ease-out),
    border-color var(--dur-1) var(--ease-out);
}
.conn-toggle input {
  margin-top: 0.2rem;
}
.conn-toggle:hover {
  background: color-mix(in srgb, var(--fg) 4%, transparent);
}
.conn-toggle.on {
  border-color: color-mix(in srgb, var(--accent) 55%, var(--border));
  background: color-mix(in srgb, var(--accent) 8%, transparent);
}
.conn-result {
  display: flex;
  gap: 0.625rem;
  padding: 0.625rem 0.875rem;
  border: 1px solid var(--border);
  border-left-width: 4px;
  font-size: 0.875rem;
}
.conn-result.ok {
  border-left-color: var(--success);
  background: color-mix(in srgb, var(--success) 8%, transparent);
}
.conn-result.warn {
  border-left-color: var(--warning);
  background: color-mix(in srgb, var(--warning) 8%, transparent);
}
.conn-result.fail {
  border-left-color: var(--danger);
  background: color-mix(in srgb, var(--danger) 8%, transparent);
}
</style>
