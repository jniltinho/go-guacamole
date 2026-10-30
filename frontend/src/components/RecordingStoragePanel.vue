<script setup lang="ts">
import { CheckCircle2, CloudUpload, PlugZap, XCircle } from '@lucide/vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { api } from '../api/client'
import { useToast } from '../composables/useToast'
import ErrorBox from './ErrorBox.vue'
import Panel from './Panel.vue'

// Administration → Recording → object storage (openspec add-async-jobs-object-storage): secrets are write-only.
interface Target {
  provider: string
  bucket: string
  prefix: string
  region: string
  endpoint: string
  pathStyle: boolean
  namespace: string
  ociAuth: string
  compartment: string
  sse: string
  kmsKeyId: string
  accessKey: string
  secretKey: string
  serviceAccount: string
}
interface View {
  enabled: boolean
  keepLocal: boolean
  targetId: string
  target: Partial<Target> | null
  secretsSet: Record<string, boolean>
  available: boolean
  unavailableReason?: string
}

const { t } = useI18n()
const { show } = useToast()
const v = ref<View | null>(null)
const error = ref<unknown>(null)
const busy = ref(false)
const result = ref<{ ok: boolean; message?: string; versioned?: boolean; deleteRefused?: boolean } | null>(
  null,
)
const blank = (): Target => ({
  provider: '', bucket: '', prefix: '', region: '', endpoint: '', pathStyle: false, namespace: '',
  ociAuth: 'instance_principal_auth', compartment: '', sse: '', kmsKeyId: '', accessKey: '', secretKey: '', serviceAccount: '',
}) // prettier-ignore
const form = reactive({ enabled: false, keepLocal: true, target: blank() })
const providers = ['', 'aws', 's3', 'oci', 'oci-native', 'gcs'] as const
const p = computed(() => form.target.provider)
const has = (...ps: string[]) => ps.includes(p.value)

async function load() {
  v.value = await api.get<View>('/settings/recording/storage')
  Object.assign(form, { enabled: v.value.enabled, keepLocal: v.value.keepLocal })
  form.target = { ...blank(), ...v.value.target, secretKey: '', serviceAccount: '' }
}

const body = () => ({
  enabled: form.enabled,
  keepLocal: form.keepLocal,
  target: form.target.provider ? form.target : null,
})

async function save() {
  busy.value = true
  error.value = null
  try {
    v.value = await api.put<View>('/settings/recording/storage', body())
    form.target.secretKey = form.target.serviceAccount = ''
    show(t('common.saved'))
  } catch (e) {
    error.value = e
  } finally {
    busy.value = false
  }
}

async function test() {
  busy.value = true
  result.value = null
  error.value = null
  try {
    result.value = await api.post<NonNullable<typeof result.value>>(
      '/settings/recording/storage/test',
      body(),
    )
  } catch (e) {
    error.value = e
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <Panel :title="t('recordingStorage.title')">
    <form v-if="v" class="space-y-4 p-4" data-testid="storage-form" @submit.prevent="save">
      <p class="text-xs text-muted">{{ t('recordingStorage.hint') }}</p>
      <p
        v-if="!v.available"
        class="flex items-center gap-2 text-sm text-warning"
        data-testid="storage-unavailable"
      >
        <XCircle class="h-4 w-4 shrink-0" /> {{ v.unavailableReason }}
      </p>

      <div class="grid gap-4 sm:grid-cols-2">
        <div>
          <label class="label" for="st-provider">{{ t('recordingStorage.provider') }}</label>
          <select
            id="st-provider"
            v-model="form.target.provider"
            class="input"
            data-testid="storage-provider"
          >
            <option v-for="o in providers" :key="o" :value="o">
              {{ t('recordingStorage.providers.' + (o || 'local')) }}
            </option>
          </select>
        </div>
        <template v-if="p">
          <div v-if="has('s3')">
            <label class="label" for="st-endpoint">{{ t('recordingStorage.endpoint') }}</label>
            <input
              id="st-endpoint"
              v-model.trim="form.target.endpoint"
              class="input"
              placeholder="https://s3.example.com"
            />
          </div>
          <div v-if="has('oci', 'oci-native')">
            <label class="label" for="st-ns">{{ t('recordingStorage.namespace') }}</label>
            <input id="st-ns" v-model.trim="form.target.namespace" class="input" />
          </div>
          <div v-if="has('aws', 's3', 'oci', 'oci-native')">
            <label class="label" for="st-region">{{ t('recordingStorage.region') }}</label>
            <input id="st-region" v-model.trim="form.target.region" class="input" placeholder="us-east-1" />
          </div>
          <div>
            <label class="label" for="st-bucket">{{ t('recordingStorage.bucket') }}</label>
            <input
              id="st-bucket"
              v-model.trim="form.target.bucket"
              class="input"
              data-testid="storage-bucket"
            />
          </div>
          <div>
            <label class="label" for="st-prefix">{{ t('recordingStorage.prefix') }}</label>
            <input
              id="st-prefix"
              v-model.trim="form.target.prefix"
              class="input"
              placeholder="goguacamole/recordings"
            />
          </div>
          <div v-if="has('oci-native')">
            <label class="label" for="st-comp">{{ t('recordingStorage.compartment') }}</label>
            <input
              id="st-comp"
              v-model.trim="form.target.compartment"
              class="input"
              placeholder="ocid1.compartment…"
            />
          </div>
          <div v-if="has('oci-native')">
            <label class="label" for="st-ociauth">{{ t('recordingStorage.ociAuth') }}</label>
            <select id="st-ociauth" v-model="form.target.ociAuth" class="input">
              <option value="instance_principal_auth">{{ t('recordingStorage.ociAuths.instance') }}</option>
              <option value="workload_identity_auth">{{ t('recordingStorage.ociAuths.workload') }}</option>
            </select>
          </div>
          <template v-if="has('aws', 's3', 'oci')">
            <div>
              <label class="label" for="st-ak">{{ t('recordingStorage.accessKey') }}</label>
              <input id="st-ak" v-model.trim="form.target.accessKey" class="input" autocomplete="off" />
            </div>
            <div>
              <label class="label" for="st-sk">{{ t('recordingStorage.secretKey') }}</label>
              <input
                id="st-sk"
                v-model="form.target.secretKey"
                class="input"
                type="password"
                autocomplete="new-password"
                :placeholder="v.secretsSet.secretKey ? t('recordingStorage.secretSet') : ''"
              />
            </div>
          </template>
          <div v-if="has('gcs')" class="sm:col-span-2">
            <label class="label" for="st-sa">{{ t('recordingStorage.serviceAccount') }}</label>
            <textarea
              id="st-sa"
              v-model="form.target.serviceAccount"
              class="input font-mono text-xs"
              rows="4"
              :placeholder="
                v.secretsSet.serviceAccount
                  ? t('recordingStorage.secretSet')
                  : '{ &quot;type&quot;: &quot;service_account&quot;, … }'
              "
            />
          </div>
          <label v-if="has('s3')" class="flex items-center gap-2 text-sm sm:col-span-2">
            <input v-model="form.target.pathStyle" type="checkbox" /> {{ t('recordingStorage.pathStyle') }}
          </label>
          <div v-if="has('aws')">
            <label class="label" for="st-sse">{{ t('recordingStorage.sse') }}</label>
            <select id="st-sse" v-model="form.target.sse" class="input">
              <option value="">{{ t('recordingStorage.sseNone') }}</option>
              <option value="AES256">AES256</option>
              <option value="aws:kms">aws:kms</option>
            </select>
          </div>
          <div v-if="has('aws') && form.target.sse === 'aws:kms'">
            <label class="label" for="st-kms">{{ t('recordingStorage.kmsKeyId') }}</label>
            <input id="st-kms" v-model.trim="form.target.kmsKeyId" class="input" />
          </div>
        </template>
      </div>

      <template v-if="p">
        <label class="flex items-start gap-2 text-sm">
          <input
            v-model="form.enabled"
            type="checkbox"
            class="mt-0.5"
            :disabled="!v.available"
            data-testid="storage-enabled"
          />
          <span>
            {{ t('recordingStorage.enabled') }}
            <span class="block text-xs text-muted">{{ t('recordingStorage.enabledHint') }}</span>
          </span>
        </label>
        <label class="flex items-start gap-2 text-sm">
          <input v-model="form.keepLocal" type="checkbox" class="mt-0.5" />
          <span>
            {{ t('recordingStorage.keepLocal') }}
            <span class="block text-xs text-muted">{{ t('recordingStorage.keepLocalHint') }}</span>
          </span>
        </label>
      </template>

      <div
        v-if="result"
        class="flex items-start gap-2 text-sm"
        :class="result.ok ? 'text-success' : 'text-danger'"
        role="status"
        data-testid="storage-test-result"
      >
        <component :is="result.ok ? CheckCircle2 : XCircle" class="mt-0.5 h-4 w-4 shrink-0" />
        <span>
          {{ result.ok ? t('recordingStorage.testOk') : result.message }}
          <span v-if="result.ok" class="block text-xs text-muted">
            {{ result.versioned ? t('recordingStorage.versioned') : t('recordingStorage.notVersioned') }}
          </span>
          <span v-if="result.deleteRefused" class="block text-xs">{{
            t('recordingStorage.deleteRefused')
          }}</span>
        </span>
      </div>
      <ErrorBox :error="error" />

      <div class="flex flex-wrap justify-end gap-2">
        <button v-if="p" class="btn" type="button" :disabled="busy" data-testid="storage-test" @click="test">
          <PlugZap class="h-4 w-4" /> {{ t('recordingStorage.test') }}
        </button>
        <button class="btn btn-primary" type="submit" :disabled="busy" data-testid="storage-save">
          <CloudUpload class="h-4 w-4" /> {{ t('common.save') }}
        </button>
      </div>
    </form>
  </Panel>
</template>
