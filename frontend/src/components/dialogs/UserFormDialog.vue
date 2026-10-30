<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { api } from '../../api/client'
import type { User } from '../../api/types'
import ErrorBox from '../ErrorBox.vue'
import Modal from '../Modal.vue'

// New user (Users → New user): a local account with its profile and restrictions; the user page edits them later
// and has the password reset and history.
const emit = defineEmits<{ close: []; created: [user: User] }>()
const { t } = useI18n()
const form = reactive({
  username: '',
  password: '',
  fullName: '',
  email: '',
  disabled: false,
  mustChangePassword: true,
  validFrom: '',
  validUntil: '',
  timezone: '',
  accessWindowStart: '',
  accessWindowEnd: '',
})
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
    wide
    :title="t('users.new')"
    :ok="t('common.create')"
    :busy="busy"
    :locked="busy"
    testid="user-form-dialog"
    @close="emit('close')"
    @submit="submit"
  >
    <h3 class="section-title mb-3">{{ t('users.profile') }}</h3>
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
      <label class="flex items-center gap-2 text-sm">
        <input v-model="form.disabled" type="checkbox" /> {{ t('users.disabled') }}
      </label>
      <label class="flex items-center gap-2 text-sm">
        <input v-model="form.mustChangePassword" type="checkbox" /> {{ t('users.mustChange') }}
      </label>
    </div>

    <h3 class="section-title mt-5 mb-3 border-t border-line pt-4">{{ t('users.restrictions') }}</h3>
    <div class="grid gap-4 sm:grid-cols-3">
      <div>
        <label class="label" for="nu-from">{{ t('users.validFrom') }}</label>
        <input id="nu-from" v-model="form.validFrom" class="input" type="date" />
      </div>
      <div>
        <label class="label" for="nu-until">{{ t('users.validUntil') }}</label>
        <input id="nu-until" v-model="form.validUntil" class="input" type="date" />
      </div>
      <div>
        <label class="label" for="nu-tz">{{ t('users.timezone') }}</label>
        <input id="nu-tz" v-model.trim="form.timezone" class="input" placeholder="America/Sao_Paulo" />
      </div>
      <div>
        <label class="label" for="nu-wstart">{{ t('users.windowStart') }}</label>
        <input id="nu-wstart" v-model="form.accessWindowStart" class="input" type="time" />
      </div>
      <div>
        <label class="label" for="nu-wend">{{ t('users.windowEnd') }}</label>
        <input id="nu-wend" v-model="form.accessWindowEnd" class="input" type="time" />
      </div>
    </div>
    <ErrorBox :error="error" class="mt-3" />
  </Modal>
</template>
