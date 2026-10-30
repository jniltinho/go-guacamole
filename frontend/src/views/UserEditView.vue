<script setup lang="ts">
import { CalendarClock, History, KeyRound, ShieldCheck, Trash2, UserRound } from '@lucide/vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import { api } from '../api/client'
import type { User } from '../api/types'
import AccessLog from '../components/AccessLog.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import ErrorBox from '../components/ErrorBox.vue'
import PageHeader from '../components/PageHeader.vue'
import PageTabs from '../components/PageTabs.vue'
import Panel from '../components/Panel.vue'
import { useAuth } from '../stores/auth'
import { useToast } from '../composables/useToast'

const route = useRoute()
const auth = useAuth()
const router = useRouter()
const { t, d } = useI18n()
const { show } = useToast()
const form = reactive({
  username: '',
  fullName: '',
  email: '',
  disabled: false,
  mustChangePassword: true,
  validFrom: '',
  validUntil: '',
  accessWindowStart: '',
  accessWindowEnd: '',
  timezone: '',
})
const meta = ref<User | null>(null)
const groups = ref<string[]>([])
const error = ref<unknown>(null)
const newPassword = ref('')
const confirmDelete = ref(false)
const busy = ref(false)

type Tab = 'profile' | 'restrictions' | 'password' | 'history'
const tab = ref<Tab>('profile')
const tabs = computed(() => [
  { id: 'profile' as Tab, label: t('users.profile'), icon: UserRound },
  { id: 'restrictions' as Tab, label: t('users.restrictions'), icon: CalendarClock },
  ...(meta.value?.authSource === 'local'
    ? [{ id: 'password' as Tab, label: t('users.resetPassword'), icon: KeyRound }]
    : []),
  ...(auth.has('AUDIT') ? [{ id: 'history' as Tab, label: t('sessions.history'), icon: History }] : []),
])

onMounted(async () => {
  const r = await api.get<{ user: User; groups: string[] }>(`/users/${route.params.id}`)
  meta.value = r.user
  groups.value = r.groups
  Object.assign(form, {
    username: r.user.username,
    fullName: r.user.fullName ?? '',
    email: r.user.email ?? '',
    disabled: r.user.disabled,
    mustChangePassword: r.user.mustChangePassword,
    validFrom: r.user.validFrom ?? '',
    validUntil: r.user.validUntil ?? '',
    accessWindowStart: r.user.accessWindowStart?.slice(0, 5) ?? '',
    accessWindowEnd: r.user.accessWindowEnd?.slice(0, 5) ?? '',
    timezone: r.user.timezone ?? '',
  })
})

async function save() {
  busy.value = true
  error.value = null
  try {
    await api.put(`/users/${route.params.id}`, form)
    show(t('common.saved'))
  } catch (e) {
    error.value = e
  } finally {
    busy.value = false
  }
}

async function reset() {
  error.value = null
  try {
    await api.put(`/users/${route.params.id}/password`, {
      password: newPassword.value,
      mustChangePassword: true,
    })
    newPassword.value = ''
    show(t('users.resetDone'))
  } catch (e) {
    error.value = e
  }
}

async function resetTotp() {
  error.value = null
  try {
    await api.del(`/users/${route.params.id}/totp`)
    if (meta.value) meta.value.totpEnrolled = false
    show(t('users.totpResetDone'))
  } catch (e) {
    error.value = e
  }
}

async function remove() {
  confirmDelete.value = false
  try {
    await api.del(`/users/${route.params.id}`)
    router.push('/users')
  } catch (e) {
    error.value = e
  }
}
</script>

<template>
  <div class="page-fill page">
    <PageHeader :title="form.username">
      <template v-if="meta">
        <RouterLink :to="{ path: '/permissions', query: { subject: `user:${route.params.id}` } }" class="btn">
          <ShieldCheck class="h-4 w-4" /> {{ t('users.permissions') }}
        </RouterLink>
        <button class="btn btn-danger" @click="confirmDelete = true">
          <Trash2 class="h-4 w-4" /> {{ t('common.delete') }}
        </button>
      </template>
    </PageHeader>
    <form class="flex min-h-0 flex-1 flex-col gap-4" @submit.prevent="tab === 'password' ? reset() : save()">
      <PageTabs v-model="tab" name="user" :fill="tab === 'history'" :tabs="tabs" :label="t('users.profile')">
        <Panel v-if="tab === 'profile'" :title="t('users.profile')">
          <div class="grid gap-4 p-4 md:grid-cols-2">
            <div>
              <label class="label">{{ t('users.username') }}</label>
              <input v-model="form.username" class="input" required data-testid="user-username" />
            </div>
            <div>
              <label class="label">{{ t('users.fullName') }}</label>
              <input v-model="form.fullName" class="input" />
            </div>
            <div>
              <label class="label">{{ t('users.email') }}</label>
              <input v-model="form.email" class="input" type="email" />
            </div>
            <label class="flex items-center gap-2 text-sm"
              ><input v-model="form.disabled" type="checkbox" /> {{ t('users.disabled') }}</label
            >
            <label class="flex items-center gap-2 text-sm"
              ><input v-model="form.mustChangePassword" type="checkbox" /> {{ t('users.mustChange') }}</label
            >
            <p v-if="groups.length" class="text-sm text-muted md:col-span-2">
              {{ t('users.groups') }}: {{ groups.join(', ') }}
            </p>
            <p
              v-if="meta"
              class="flex flex-wrap items-center gap-2 text-sm md:col-span-2"
              data-testid="user-auth"
            >
              <span class="border border-line px-1.5 py-0.5 text-xs uppercase">{{
                t('users.source.' + meta.authSource)
              }}</span>
              <span class="text-muted"
                >TOTP: {{ meta.totpEnrolled ? t('users.totpOn') : t('users.totpOff') }}</span
              >
              <button v-if="meta.totpEnrolled" type="button" class="btn btn-sm" @click="resetTotp">
                {{ t('users.totpReset') }}
              </button>
            </p>
            <p v-if="meta" class="text-xs text-muted md:col-span-2">
              {{ t('users.created') }} {{ d(meta.createdAt, 'long') }} · {{ t('users.lastChange') }}
              {{ d(meta.passwordChangedAt, 'long') }}
            </p>
          </div>
        </Panel>
        <Panel v-else-if="tab === 'restrictions'" :title="t('users.restrictions')">
          <div class="grid gap-4 p-4 md:grid-cols-3">
            <div>
              <label class="label">{{ t('users.validFrom') }}</label>
              <input v-model="form.validFrom" class="input" type="date" />
            </div>
            <div>
              <label class="label">{{ t('users.validUntil') }}</label>
              <input v-model="form.validUntil" class="input" type="date" />
            </div>
            <div>
              <label class="label">{{ t('users.timezone') }}</label>
              <input v-model="form.timezone" class="input" placeholder="America/Sao_Paulo" />
            </div>
            <div>
              <label class="label">{{ t('users.windowStart') }}</label>
              <input v-model="form.accessWindowStart" class="input" type="time" />
            </div>
            <div>
              <label class="label">{{ t('users.windowEnd') }}</label>
              <input v-model="form.accessWindowEnd" class="input" type="time" />
            </div>
          </div>
        </Panel>
        <Panel v-else-if="tab === 'password'" :title="t('users.resetPassword')">
          <div class="flex flex-wrap items-end gap-3 p-4">
            <div class="min-w-60 flex-1">
              <label class="label">{{ t('settings.newPassword') }}</label>
              <input
                v-model="newPassword"
                class="input"
                type="password"
                autocomplete="new-password"
                minlength="8"
                required
              />
            </div>
            <button class="btn" type="button" :disabled="newPassword.length < 8" @click="reset">
              <KeyRound class="h-4 w-4" /> {{ t('users.resetPassword') }}
            </button>
          </div>
        </Panel>
        <Panel v-else fill :title="t('sessions.history')" data-testid="history-panel">
          <AccessLog full :user-id="route.params.id as string" />
        </Panel>
      </PageTabs>
      <ErrorBox :error="error" class="shrink-0" />
      <div class="flex shrink-0 justify-end gap-2">
        <RouterLink to="/users" class="btn">{{ t('common.back') }}</RouterLink>
        <button
          v-if="tab === 'profile' || tab === 'restrictions'"
          class="btn btn-primary"
          type="submit"
          :disabled="busy"
          data-testid="save"
        >
          {{ t('common.save') }}
        </button>
      </div>
    </form>
    <ConfirmDialog
      :open="confirmDelete"
      danger
      :title="t('common.delete')"
      :message="t('users.deleteConfirm', { name: form.username })"
      :type-to-confirm="form.username"
      @confirm="remove"
      @cancel="confirmDelete = false"
    />
  </div>
</template>
