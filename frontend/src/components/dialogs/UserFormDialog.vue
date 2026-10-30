<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { api } from '../../api/client'
import type { User } from '../../api/types'
import ErrorBox from '../ErrorBox.vue'
import Modal from '../Modal.vue'
import PasswordPair from '../PasswordPair.vue'
import TimezoneSelect from '../TimezoneSelect.vue'

// New user (Users → New user), with the Java client's fields: account, profile and restrictions. Create stays
// disabled until the basics are valid (the server applies the same rules). With `source` it clones that user (as
// in Guacamole): the form starts from its settings and the server copies its permissions and groups.
const props = defineProps<{ source?: User }>()
const emit = defineEmits<{ close: []; created: [user: User] }>()
const { t } = useI18n()
const s = props.source
const form = reactive({
  username: '',
  password: '',
  disabled: s?.disabled ?? false,
  fullName: '',
  email: '',
  organization: s?.organization ?? '',
  organizationalRole: s?.organizationalRole ?? '',
  mustChangePassword: true,
  validFrom: s?.validFrom ?? '',
  validUntil: s?.validUntil ?? '',
  accessWindowStart: s?.accessWindowStart?.slice(0, 5) ?? '',
  accessWindowEnd: s?.accessWindowEnd?.slice(0, 5) ?? '',
  timezone: s?.timezone ?? '',
})
const repeat = ref('')
const busy = ref(false)
const error = ref<unknown>(null)

const USERNAME = /^[A-Za-z0-9._@-]{1,128}$/
const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const passwordOk = computed(
  () => form.password.length >= 8 && new TextEncoder().encode(form.password).length <= 72,
)
// the first thing still missing or wrong (null when the form can be submitted)
const problem = computed(() => {
  if (!USERNAME.test(form.username) || !passwordOk.value || repeat.value !== form.password)
    return 'basicsHint'
  if (form.email && !EMAIL.test(form.email)) return 'emailInvalid'
  if (!form.accessWindowStart !== !form.accessWindowEnd) return 'windowBoth'
  if (form.validFrom && form.validUntil && form.validUntil < form.validFrom) return 'datesOrder'
  return null
})
const valid = computed(() => !problem.value)

async function submit() {
  if (!valid.value) return
  busy.value = true
  error.value = null
  try {
    emit('created', await api.post<User>('/users', { ...form, cloneFrom: s?.id }))
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
    :title="s ? t('users.cloneTitle', { name: s.username }) : t('users.new')"
    :ok="t('common.create')"
    :busy="busy"
    :locked="busy"
    :disabled="!valid"
    testid="user-form-dialog"
    @close="emit('close')"
    @submit="submit"
  >
    <p v-if="s" class="mb-4 border-l-2 border-accent pl-3 text-sm text-muted" data-testid="clone-hint">
      {{ t('users.cloneHint', { name: s.username }) }}
    </p>
    <h3 class="section-title mb-3">{{ t('users.account') }}</h3>
    <div class="grid gap-4 sm:grid-cols-2">
      <div>
        <label class="label" for="nu-username">{{ t('users.username') }} *</label>
        <input
          id="nu-username"
          v-model.trim="form.username"
          class="input"
          required
          autofocus
          autocomplete="off"
          maxlength="128"
          pattern="[A-Za-z0-9._@\-]+"
          data-testid="user-username"
        />
      </div>
      <label class="flex items-center gap-2 pt-5 text-sm">
        <input v-model="form.disabled" type="checkbox" /> {{ t('users.disabled') }}
      </label>
      <PasswordPair id="nu-password" v-model="form.password" v-model:repeat="repeat" required />
    </div>

    <h3 class="section-title mt-5 mb-3 border-t border-line pt-4">{{ t('users.profile') }}</h3>
    <div class="grid gap-4 sm:grid-cols-2">
      <div>
        <label class="label" for="nu-fullname">{{ t('users.fullName') }}</label>
        <input id="nu-fullname" v-model="form.fullName" class="input" />
      </div>
      <div>
        <label class="label" for="nu-email">{{ t('users.email') }}</label>
        <input id="nu-email" v-model.trim="form.email" class="input" type="email" />
      </div>
      <div>
        <label class="label" for="nu-org">{{ t('users.organization') }}</label>
        <input id="nu-org" v-model="form.organization" class="input" maxlength="256" />
      </div>
      <div>
        <label class="label" for="nu-role">{{ t('users.role') }}</label>
        <input id="nu-role" v-model="form.organizationalRole" class="input" maxlength="256" />
      </div>
    </div>

    <h3 class="section-title mt-5 mb-3 border-t border-line pt-4">{{ t('users.restrictions') }}</h3>
    <div class="grid gap-4 sm:grid-cols-3">
      <label class="flex items-center gap-2 text-sm sm:col-span-3">
        <input v-model="form.mustChangePassword" type="checkbox" /> {{ t('users.mustChange') }}
      </label>
      <div>
        <label class="label" for="nu-wstart">{{ t('users.windowStart') }}</label>
        <input id="nu-wstart" v-model="form.accessWindowStart" class="input" type="time" />
      </div>
      <div>
        <label class="label" for="nu-wend">{{ t('users.windowEnd') }}</label>
        <input id="nu-wend" v-model="form.accessWindowEnd" class="input" type="time" />
      </div>
      <div>
        <label class="label" for="nu-tz">{{ t('users.timezone') }}</label>
        <TimezoneSelect id="nu-tz" v-model="form.timezone" />
      </div>
      <div>
        <label class="label" for="nu-from">{{ t('users.validFrom') }}</label>
        <input id="nu-from" v-model="form.validFrom" class="input" type="date" />
      </div>
      <div>
        <label class="label" for="nu-until">{{ t('users.validUntil') }}</label>
        <input
          id="nu-until"
          v-model="form.validUntil"
          class="input"
          type="date"
          :min="form.validFrom || undefined"
        />
      </div>
    </div>
    <ErrorBox :error="error" class="mt-3" />
    <template #foot>
      <p v-if="problem" class="mr-auto text-xs text-muted" data-testid="user-form-hint">
        {{ t('users.' + problem) }}
      </p>
    </template>
  </Modal>
</template>
