<script setup lang="ts">
import { CheckCircle2, FlaskConical, XCircle } from '@lucide/vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { api, ApiError } from '../api/client'
import ErrorBox from '../components/ErrorBox.vue'
import PageHeader from '../components/PageHeader.vue'
import Panel from '../components/Panel.vue'

// (no v-model on <component :is>: it binds modelValue, not a native input's value)
// Sign-in methods (Java client extensions: TOTP, LDAP, OpenID, SAML, header,
// encrypted JSON, Duo), stored in the database and editable here.
type Method = 'totp' | 'ldap' | 'oidc' | 'saml' | 'header' | 'json' | 'duo'
interface Field {
  key: string
  type: 'text' | 'password' | 'textarea' | 'bool'
  secret?: boolean
  placeholder?: string
}

const METHODS: Array<{ id: Method; fields: Field[]; test?: 'user' | 'plain' }> = [
  { id: 'totp', fields: [{ key: 'issuer', type: 'text', placeholder: 'GoGuacamole' }] },
  {
    id: 'ldap',
    test: 'user',
    fields: [
      { key: 'url', type: 'text', placeholder: 'ldaps://ldap.example.com:636' },
      { key: 'startTls', type: 'bool' },
      { key: 'insecureSkipVerify', type: 'bool' },
      { key: 'bindDn', type: 'text', placeholder: 'cn=guacamole,ou=services,dc=example,dc=com' },
      { key: 'bindPassword', type: 'password', secret: true },
      { key: 'userBaseDn', type: 'text', placeholder: 'ou=people,dc=example,dc=com' },
      { key: 'usernameAttribute', type: 'text', placeholder: 'uid / sAMAccountName' },
      { key: 'userFilter', type: 'text', placeholder: '(objectClass=person)' },
      { key: 'groupBaseDn', type: 'text', placeholder: 'ou=groups,dc=example,dc=com' },
      { key: 'groupFilter', type: 'text' },
      { key: 'memberAttribute', type: 'text', placeholder: 'member' },
      { key: 'groupNameAttribute', type: 'text', placeholder: 'cn' },
    ],
  },
  {
    id: 'oidc',
    test: 'plain',
    fields: [
      { key: 'label', type: 'text' },
      { key: 'issuer', type: 'text', placeholder: 'https://keycloak.example.com/realms/main' },
      { key: 'clientId', type: 'text' },
      { key: 'clientSecret', type: 'password', secret: true },
      { key: 'scopes', type: 'text', placeholder: 'openid profile email' },
      { key: 'usernameClaim', type: 'text', placeholder: 'preferred_username' },
      { key: 'groupsClaim', type: 'text', placeholder: 'groups' },
    ],
  },
  {
    id: 'saml',
    test: 'plain',
    fields: [
      { key: 'label', type: 'text' },
      { key: 'idpMetadataUrl', type: 'text', placeholder: 'https://idp.example.com/FederationMetadata.xml' },
      { key: 'entityId', type: 'text' },
      { key: 'usernameAttribute', type: 'text', placeholder: 'NameID' },
      { key: 'groupsAttribute', type: 'text' },
      { key: 'certificate', type: 'textarea', placeholder: '-----BEGIN CERTIFICATE-----' },
      { key: 'privateKey', type: 'textarea', secret: true, placeholder: '-----BEGIN PRIVATE KEY-----' },
    ],
  },
  {
    id: 'header',
    test: 'plain',
    fields: [
      { key: 'name', type: 'text', placeholder: 'REMOTE_USER' },
      { key: 'groupsHeader', type: 'text', placeholder: 'X-Groups' },
    ],
  },
  {
    id: 'json',
    test: 'user',
    fields: [{ key: 'secretKey', type: 'password', secret: true, placeholder: '32 hex' }],
  },
  {
    id: 'duo',
    test: 'plain',
    fields: [
      { key: 'apiHostname', type: 'text', placeholder: 'api-xxxxxxxx.duosecurity.com' },
      { key: 'clientId', type: 'text' },
      { key: 'clientSecret', type: 'password', secret: true },
    ],
  },
]

const { t } = useI18n()
type Section = Record<string, string | boolean>
const form = reactive<Record<string, Section> & { linkExternalUsers?: boolean }>({})
const secretsSet = ref<Record<string, boolean>>({})
const clear = ref(new Set<string>())
const info = ref({ publicUrl: '', trustedProxies: [] as string[], secureCookies: false, loadError: '' })
const loaded = ref(false)
const error = ref<unknown>(null)
const notice = ref('')
const busy = ref(false)
const testUser = reactive<Record<string, string>>({})
const results = reactive<Record<string, { ok: boolean; message: string } | undefined>>({})

const sec = (m: Method) => form[m] as Section
const urls = computed(() => {
  const b = info.value.publicUrl
  return {
    oidc: b + '/api/v1/auth/oidc/callback',
    saml: b + '/api/v1/auth/saml/metadata · ACS ' + b + '/api/v1/auth/saml/acs',
    duo: b + '/api/v1/auth/duo/callback',
    json: b + '/api/v1/auth/json?data=…',
    header: info.value.trustedProxies.join(', ') || '—',
  } as Record<string, string>
})

function apply(r: Record<string, unknown>) {
  const { secretsSet: s, publicUrl, trustedProxies, secureCookies, loadError, ...rest } = r
  Object.assign(form, rest)
  secretsSet.value = s as Record<string, boolean>
  info.value = {
    publicUrl: String(publicUrl),
    trustedProxies: (trustedProxies as string[]) ?? [],
    secureCookies: !!secureCookies,
    loadError: (loadError as string) ?? '',
  }
  clear.value = new Set()
}

onMounted(async () => {
  apply(await api.get('/settings/auth'))
  loaded.value = true
})

const body = () => ({ ...form, clear: [...clear.value] })

async function save() {
  busy.value = true
  error.value = null
  notice.value = ''
  try {
    apply(await api.put('/settings/auth', body()))
    notice.value = t('common.saved')
  } catch (e) {
    error.value = e
  } finally {
    busy.value = false
  }
}

async function test(m: Method) {
  results[m] = undefined
  try {
    results[m] = await api.post(`/settings/auth/test/${m}`, { ...body(), username: testUser[m] ?? '' })
  } catch (e) {
    results[m] = { ok: false, message: e instanceof ApiError ? e.message : String(e) }
  }
}

function toggleClear(name: string) {
  if (clear.value.has(name)) clear.value.delete(name)
  else clear.value.add(name)
}
</script>

<template>
  <div class="mx-auto max-w-4xl">
    <PageHeader class="mb-4" :title="t('authSettings.title')" :subtitle="t('authSettings.subtitle')" />
    <form v-if="loaded" class="space-y-5" data-testid="auth-settings" @submit.prevent="save">
      <p v-if="info.loadError" class="border-l-4 border-warning bg-warning/10 px-3 py-2 text-sm" role="alert">
        {{ t('authSettings.loadError') }}
      </p>
      <Panel :title="t('authSettings.general')">
        <div class="space-y-2 p-4 text-sm">
          <label class="flex items-center gap-2">
            <input v-model="form.linkExternalUsers" type="checkbox" />
            {{ t('authSettings.linkExternalUsers') }}
          </label>
          <p class="text-xs text-muted">{{ t('authSettings.linkHint') }}</p>
          <p class="text-xs text-muted">{{ t('authSettings.groupsHint') }}</p>
        </div>
      </Panel>

      <Panel v-for="m in METHODS" :key="m.id" :title="t('authSettings.' + m.id + '.title')">
        <template #actions>
          <label class="flex items-center gap-2 text-sm font-normal">
            <input v-model="sec(m.id).enabled" type="checkbox" :data-testid="'enable-' + m.id" />
            {{ t('authSettings.enabled') }}
          </label>
        </template>
        <div class="space-y-4 p-4">
          <p class="text-xs text-muted">{{ t('authSettings.' + m.id + '.hint') }}</p>
          <p v-if="urls[m.id]" class="font-mono text-xs break-all">
            {{ t('authSettings.' + m.id + '.url') }}: {{ urls[m.id] }}
          </p>
          <div class="grid gap-4 md:grid-cols-2">
            <div v-for="f in m.fields" :key="f.key" :class="{ 'md:col-span-2': f.type === 'textarea' }">
              <label v-if="f.type === 'bool'" class="flex items-center gap-2 pt-5 text-sm">
                <input v-model="sec(m.id)[f.key]" type="checkbox" />
                {{ t('authSettings.fields.' + f.key) }}
              </label>
              <template v-else>
                <label class="label" :for="m.id + '-' + f.key">{{ t('authSettings.fields.' + f.key) }}</label>
                <component
                  :is="f.type === 'textarea' ? 'textarea' : 'input'"
                  :id="m.id + '-' + f.key"
                  :value="sec(m.id)[f.key] ?? ''"
                  class="input"
                  :class="{ 'font-mono text-xs': f.type === 'textarea' }"
                  :type="f.type === 'password' ? 'password' : f.type === 'textarea' ? undefined : 'text'"
                  :rows="f.type === 'textarea' ? 5 : undefined"
                  autocomplete="off"
                  :placeholder="
                    f.secret && secretsSet[m.id + '.' + f.key] && !clear.has(m.id + '.' + f.key)
                      ? t('connections.form.secretSet')
                      : f.placeholder
                  "
                  @input="sec(m.id)[f.key] = ($event.target as HTMLInputElement).value"
                />
                <label
                  v-if="f.secret && secretsSet[m.id + '.' + f.key]"
                  class="mt-1 flex items-center gap-2 text-xs text-muted"
                >
                  <input
                    type="checkbox"
                    :checked="clear.has(m.id + '.' + f.key)"
                    @change="toggleClear(m.id + '.' + f.key)"
                  />
                  {{ t('connections.form.clearSecret') }}
                </label>
              </template>
            </div>
          </div>
          <div v-if="m.test" class="flex flex-wrap items-end gap-2 border-t border-line pt-3">
            <div v-if="m.test === 'user'" class="min-w-48">
              <label class="label">{{ t('authSettings.testUser') }}</label>
              <input v-model="testUser[m.id]" class="input" autocomplete="off" />
            </div>
            <button type="button" class="btn" :data-testid="'test-' + m.id" @click="test(m.id)">
              <FlaskConical class="h-4 w-4" /> {{ t('authSettings.test') }}
            </button>
            <p
              v-if="results[m.id]"
              class="flex min-w-0 flex-1 items-start gap-1.5 text-sm break-all"
              :class="results[m.id]!.ok ? 'text-success' : 'text-danger'"
              role="status"
            >
              <component :is="results[m.id]!.ok ? CheckCircle2 : XCircle" class="mt-0.5 h-4 w-4 shrink-0" />
              {{ results[m.id]!.message }}
            </p>
          </div>
        </div>
      </Panel>

      <ErrorBox :error="error" />
      <p v-if="error instanceof ApiError && error.field" class="text-sm text-danger">{{ error.message }}</p>
      <p v-if="notice" class="text-sm text-success" role="status">{{ notice }}</p>
      <div class="flex justify-end">
        <button class="btn btn-primary" type="submit" :disabled="busy" data-testid="save">
          {{ t('common.save') }}
        </button>
      </div>
    </form>
  </div>
</template>
