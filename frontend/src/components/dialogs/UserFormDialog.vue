<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { api } from '../../api/client'
import type { User } from '../../api/types'
import ErrorBox from '../ErrorBox.vue'
import Modal from '../Modal.vue'

// New user (Users → New user): a local account; the user page has the restrictions, password reset and history.
const emit = defineEmits<{ close: []; created: [user: User] }>()
const { t } = useI18n()
const form = reactive({ username: '', password: '', fullName: '', email: '', mustChangePassword: true })
const busy = ref(false)
const error = ref<unknown>(null)

async function submit() {
  busy.value = true
  error.value = null
  try {
    emit('created', await api.post<User>('/users', form))
  } catch (e) {
    error.value = e
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Modal
    :title="t('users.new')"
    :ok="t('common.create')"
    :busy="busy"
    :locked="busy"
    testid="user-form-dialog"
    @close="emit('close')"
    @submit="submit"
  >
    <div class="grid gap-4 sm:grid-cols-2">
      <div>
        <label class="label" for="nu-username">{{ t('users.username') }}</label>
        <input
          id="nu-username"
          v-model.trim="form.username"
          class="input"
          required
          autofocus
          autocomplete="off"
          data-testid="user-username"
        />
      </div>
      <div>
        <label class="label" for="nu-password">{{ t('users.password') }}</label>
        <input
          id="nu-password"
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
        <label class="label" for="nu-fullname">{{ t('users.fullName') }}</label>
        <input id="nu-fullname" v-model="form.fullName" class="input" />
      </div>
      <div>
        <label class="label" for="nu-email">{{ t('users.email') }}</label>
        <input id="nu-email" v-model.trim="form.email" class="input" type="email" />
      </div>
      <label class="flex items-center gap-2 text-sm sm:col-span-2">
        <input v-model="form.mustChangePassword" type="checkbox" /> {{ t('users.mustChange') }}
      </label>
    </div>
    <ErrorBox :error="error" class="mt-3" />
  </Modal>
</template>
