<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { api } from '../api/client'
import ErrorBox from '../components/ErrorBox.vue'
import LocalePicker from '../components/LocalePicker.vue'
import PageHeader from '../components/PageHeader.vue'
import Panel from '../components/Panel.vue'
import ThemePicker from '../components/ThemePicker.vue'
import { useTheme } from '../composables/useTheme'
import { useAuth } from '../stores/auth'

const { t, locale } = useI18n()
const auth = useAuth()
const { themeId } = useTheme()
const current = ref('')
const next = ref('')
const confirm = ref('')
const error = ref<unknown>(null)
const notice = ref('')
const mismatch = ref(false)

async function change() {
  error.value = null
  notice.value = ''
  mismatch.value = next.value !== confirm.value
  if (mismatch.value) return
  try {
    await api.put('/me/password', { currentPassword: current.value, newPassword: next.value })
    current.value = next.value = confirm.value = ''
    notice.value = t('settings.passwordChanged')
  } catch (e) {
    error.value = e
  }
}
</script>

<template>
  <div class="mx-auto max-w-3xl space-y-5">
    <PageHeader :title="t('settings.title')" :subtitle="t('settings.subtitle')" />
    <Panel :title="t('settings.appearance')">
      <div class="grid gap-4 p-4 md:grid-cols-2">
        <div>
          <span class="label">{{ t('common.theme') }}</span>
          <div class="flex items-center gap-2">
            <ThemePicker />
            <span class="font-medium">{{ t('themes.' + themeId) }}</span>
          </div>
          <p class="mt-1 text-xs text-muted">{{ t('settings.themeHint') }}</p>
        </div>
        <div>
          <span class="label">{{ t('common.language') }}</span>
          <div class="flex items-center gap-2">
            <LocalePicker />
            <span class="font-medium">{{ t('locales.' + locale) }}</span>
          </div>
          <p class="mt-1 text-xs text-muted">{{ t('settings.languageHint') }}</p>
        </div>
      </div>
    </Panel>
    <Panel :title="t('settings.account')">
      <dl class="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 p-4 text-sm">
        <dt class="text-muted">{{ t('users.username') }}</dt>
        <dd>{{ auth.user?.username }}</dd>
        <dt class="text-muted">{{ t('users.groups') }}</dt>
        <dd>{{ auth.me?.groups.join(', ') || '—' }}</dd>
        <dt class="text-muted">{{ t('nav.permissions') }}</dt>
        <dd>{{ auth.me?.permissions.map((p) => t('permissions.perm.' + p)).join(', ') || '—' }}</dd>
      </dl>
    </Panel>
    <Panel :title="t('settings.password')">
      <form class="grid gap-4 p-4 md:grid-cols-3" @submit.prevent="change">
        <div>
          <label class="label">{{ t('settings.currentPassword') }}</label>
          <input v-model="current" class="input" type="password" autocomplete="current-password" required />
        </div>
        <div>
          <label class="label">{{ t('settings.newPassword') }}</label>
          <input
            v-model="next"
            class="input"
            type="password"
            autocomplete="new-password"
            minlength="8"
            required
          />
        </div>
        <div>
          <label class="label">{{ t('settings.confirmPassword') }}</label>
          <input v-model="confirm" class="input" type="password" autocomplete="new-password" required />
        </div>
        <div class="md:col-span-3">
          <p v-if="mismatch" class="mb-2 text-sm text-danger">{{ t('login.mismatch') }}</p>
          <ErrorBox :error="error" class="mb-2" />
          <p v-if="notice" class="mb-2 text-sm text-success">{{ notice }}</p>
          <button class="btn btn-primary" type="submit">{{ t('common.save') }}</button>
        </div>
      </form>
    </Panel>
  </div>
</template>
