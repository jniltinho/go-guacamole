<script setup lang="ts">
import { KeyRound, ShieldCheck, Trash2 } from '@lucide/vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import { api } from '../api/client'
import type { User } from '../api/types'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import ErrorBox from '../components/ErrorBox.vue'
import PageHeader from '../components/PageHeader.vue'
import Panel from '../components/Panel.vue'

const route = useRoute()
const router = useRouter()
const { t, d } = useI18n()
const isNew = computed(() => route.params.id === 'new')
const form = reactive({
  username: '',
  password: '',
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
const notice = ref('')
const newPassword = ref('')
const confirmDelete = ref(false)
const busy = ref(false)

onMounted(async () => {
  if (isNew.value) return
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
  notice.value = ''
  try {
    if (isNew.value) {
      const u = await api.post<User>('/users', form)
      router.replace(`/users/${u.id}`)
    } else {
      const { password: _pw, ...body } = form
      void _pw
      await api.put(`/users/${route.params.id}`, body)
      notice.value = t('common.saved')
    }
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
    notice.value = t('users.resetDone')
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
  <div class="mx-auto max-w-4xl">
    <PageHeader :title="isNew ? t('users.new') : form.username">
      <template v-if="!isNew">
        <RouterLink :to="{ path: '/permissions', query: { subject: `user:${route.params.id}` } }" class="btn">
          <ShieldCheck class="h-4 w-4" /> {{ t('users.permissions') }}
        </RouterLink>
        <button class="btn btn-danger" @click="confirmDelete = true">
          <Trash2 class="h-4 w-4" /> {{ t('common.delete') }}
        </button>
      </template>
    </PageHeader>
    <form class="space-y-5" @submit.prevent="save">
      <Panel :title="t('users.profile')">
        <div class="grid gap-4 p-4 md:grid-cols-2">
          <div>
            <label class="label">{{ t('users.username') }}</label>
            <input v-model="form.username" class="input" required data-testid="user-username" />
          </div>
          <div v-if="isNew">
            <label class="label">{{ t('users.password') }}</label>
            <input
              v-model="form.password"
              class="input"
              type="password"
              autocomplete="new-password"
              required
              minlength="8"
              data-testid="user-password"
            />
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
          <p v-if="meta" class="text-xs text-muted md:col-span-2">
            {{ t('users.created') }} {{ d(meta.createdAt, 'long') }} · {{ t('users.lastChange') }}
            {{ d(meta.passwordChangedAt, 'long') }}
          </p>
        </div>
      </Panel>
      <Panel :title="t('users.restrictions')">
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
      <ErrorBox :error="error" />
      <p v-if="notice" class="text-sm text-success" role="status">{{ notice }}</p>
      <div class="flex justify-end gap-2">
        <RouterLink to="/users" class="btn">{{ t('common.back') }}</RouterLink>
        <button class="btn btn-primary" type="submit" :disabled="busy" data-testid="save">
          {{ t('common.save') }}
        </button>
      </div>
    </form>
    <Panel v-if="!isNew" :title="t('users.resetPassword')" class="mt-5">
      <form class="flex flex-wrap items-end gap-3 p-4" @submit.prevent="reset">
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
        <button class="btn" type="submit"><KeyRound class="h-4 w-4" /> {{ t('users.resetPassword') }}</button>
      </form>
    </Panel>
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
