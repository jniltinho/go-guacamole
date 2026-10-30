<script setup lang="ts">
import { Trash2 } from '@lucide/vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import { api, ApiError } from '../api/client'
import type { Connection, ConnectionGroup, ConnectionRecording, List, Protocol } from '../api/types'
import AccessLog from '../components/AccessLog.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import ErrorBox from '../components/ErrorBox.vue'
import PageHeader from '../components/PageHeader.vue'
import RecordingPanel from '../components/RecordingPanel.vue'
import Panel from '../components/Panel.vue'
import { useAuth } from '../stores/auth'
import { PROTOCOL_FIELDS, PROTOCOLS, SECTIONS, type Field } from '../lib/protocols'

const route = useRoute()
const auth = useAuth()
const router = useRouter()
const { t, te } = useI18n()
const id = computed(() => route.params.id as string | undefined)

const form = reactive({
  name: '',
  protocol: 'ssh' as Protocol,
  parentId: '' as string,
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
const sections = computed(() => SECTIONS.filter((s) => fields.value.some((f) => f.section === s)))
const label = (f: Field) => (te('params.' + f.name) ? t('params.' + f.name) : f.name)

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

async function save() {
  busy.value = true
  error.value = null
  errField.value = ''
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
  const body = {
    name: form.name,
    protocol: form.protocol,
    parentId: form.parentId || null,
    guacdHost: form.guacdHost || null,
    guacdPort: num(form.guacdPort),
    guacdEncryption: form.guacdEncryption || null,
    maxConnections: num(form.maxConnections),
    maxConnectionsPerUser: num(form.maxConnectionsPerUser),
    parameters: out,
  }
  try {
    if (id.value) await api.put(`/connections/${id.value}`, body)
    else await api.post('/connections', body)
    router.push('/connections')
  } catch (e) {
    error.value = e
    if (e instanceof ApiError && e.field) errField.value = e.field
  } finally {
    busy.value = false
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
  <div class="page">
    <PageHeader class="mb-4" :title="id ? t('connections.edit') : t('connections.create')">
      <button v-if="id" class="btn btn-danger" @click="confirmDelete = true">
        <Trash2 class="h-4 w-4" /> {{ t('common.delete') }}
      </button>
    </PageHeader>
    <form class="space-y-5" data-testid="connection-form" @submit.prevent="save">
      <Panel :title="t('connections.form.general')">
        <div class="grid gap-4 p-4 md:grid-cols-3">
          <div>
            <label class="label" for="f-name">{{ t('connections.form.name') }}</label>
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
        </div>
      </Panel>

      <Panel v-for="s in sections" :key="s" :title="t('connections.form.' + s)">
        <div class="grid gap-4 p-4 md:grid-cols-2">
          <p v-if="s === 'sftp' && form.protocol === 'ssh'" class="text-sm text-muted md:col-span-2">
            {{ t('connections.form.sshSftpHint') }}
          </p>
          <div
            v-for="f in fields.filter((x) => x.section === s)"
            :key="f.name"
            :class="{ 'md:col-span-2': f.type === 'textarea' }"
          >
            <label v-if="f.type !== 'bool'" class="label" :for="'p-' + f.name">
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
                :type="f.type === 'password' ? 'password' : undefined"
                :rows="f.type === 'textarea' ? 4 : undefined"
                autocomplete="new-password"
                :placeholder="
                  storedSecrets.has(f.name) && !cleared.has(f.name) ? t('connections.form.secretSet') : ''
                "
                :data-testid="'param-' + f.name"
                @input="params[f.name] = ($event.target as HTMLInputElement).value"
              />
              <label v-if="storedSecrets.has(f.name)" class="mt-1 flex items-center gap-2 text-xs text-muted">
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
            <select
              v-else-if="f.type === 'select'"
              :id="'p-' + f.name"
              v-model="params[f.name]"
              class="input"
            >
              <option v-for="o in f.options" :key="o" :value="o">{{ o || '—' }}</option>
            </select>
            <label v-else class="flex items-center gap-2 pt-5 text-sm">
              <input
                type="checkbox"
                :checked="params[f.name] === 'true'"
                @change="params[f.name] = ($event.target as HTMLInputElement).checked ? 'true' : ''"
              />
              {{ label(f) }}
            </label>
          </div>
        </div>
      </Panel>

      <Panel :title="t('connections.form.advanced') + ' · guacd'">
        <div class="grid gap-4 p-4 md:grid-cols-3">
          <div>
            <label class="label">{{ t('connections.form.guacdHost') }}</label>
            <input v-model="form.guacdHost" class="input" />
          </div>
          <div>
            <label class="label">{{ t('connections.form.guacdPort') }}</label>
            <input v-model="form.guacdPort" class="input" type="number" min="1" max="65535" />
          </div>
          <div>
            <label class="label">{{ t('connections.form.guacdEncryption') }}</label>
            <select v-model="form.guacdEncryption" class="input">
              <option value="">{{ t('connections.form.encryptionDefault') }}</option>
              <option value="NONE">NONE</option>
              <option value="SSL">SSL/TLS</option>
            </select>
          </div>
          <div>
            <label class="label">{{ t('connections.form.maxConnections') }}</label>
            <input
              v-model="form.maxConnections"
              class="input"
              type="number"
              min="0"
              :placeholder="t('connections.form.unlimited')"
            />
          </div>
          <div>
            <label class="label">{{ t('connections.form.maxConnectionsPerUser') }}</label>
            <input
              v-model="form.maxConnectionsPerUser"
              class="input"
              type="number"
              min="0"
              :placeholder="t('connections.form.unlimited')"
            />
          </div>
        </div>
      </Panel>

      <ErrorBox :error="error" />
      <div class="flex justify-end gap-2">
        <RouterLink to="/connections" class="btn">{{ t('common.cancel') }}</RouterLink>
        <button class="btn btn-primary" type="submit" :disabled="busy" data-testid="save">
          {{ t('common.save') }}
        </button>
      </div>
    </form>
    <RecordingPanel
      v-if="id && recording"
      class="mt-5"
      :connection-id="id"
      :protocol="form.protocol"
      :recording="recording"
    />
    <Panel
      v-if="id && auth.has('AUDIT')"
      :title="t('sessions.history')"
      class="mt-5"
      data-testid="history-panel"
    >
      <div class="h-96"><AccessLog full :connection-id="id" /></div>
    </Panel>
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
