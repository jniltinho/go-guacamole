<script setup lang="ts">
import { Copy, Eye, EyeOff, Wand2 } from '@lucide/vue'
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { useToast } from '../composables/useToast'
import { generatePassword } from '../lib/password'

// Password + confirmation with show/hide, generate and copy (as in AD Manager). Generating fills both fields and
// shows the password so it can be copied and handed over.
const password = defineModel<string>({ required: true })
const repeat = defineModel<string>('repeat', { required: true })
const props = defineProps<{ id: string; label?: string; required?: boolean }>()
const { t } = useI18n()
const { show } = useToast()
const visible = ref(false)
const mismatch = computed(() => repeat.value !== '' && repeat.value !== password.value)

function generate() {
  password.value = repeat.value = generatePassword()
  visible.value = true
}
async function copy() {
  try {
    await navigator.clipboard.writeText(password.value)
    show(t('users.passwordCopied'))
  } catch {
    show(t('users.copyFailed'), 'error')
  }
}
const star = computed(() => (props.required ? ' *' : ''))
</script>

<template>
  <div>
    <label class="label" :for="id">{{ (label ?? t('users.password')) + star }}</label>
    <div class="flex gap-1">
      <input
        :id="id"
        v-model="password"
        class="input min-w-0 flex-1 font-mono"
        :type="visible ? 'text' : 'password'"
        autocomplete="new-password"
        :required="required"
        minlength="8"
        :aria-describedby="id + '-hint'"
        data-testid="user-password"
      />
      <button
        type="button"
        class="btn btn-sm shrink-0"
        :aria-pressed="visible"
        :aria-label="visible ? t('users.hidePassword') : t('users.showPassword')"
        :title="visible ? t('users.hidePassword') : t('users.showPassword')"
        @click="visible = !visible"
      >
        <component :is="visible ? EyeOff : Eye" class="h-4 w-4" />
      </button>
      <button
        type="button"
        class="btn btn-sm shrink-0"
        :aria-label="t('users.generatePassword')"
        :title="t('users.generatePassword')"
        data-testid="password-generate"
        @click="generate"
      >
        <Wand2 class="h-4 w-4" />
      </button>
      <button
        type="button"
        class="btn btn-sm shrink-0"
        :disabled="!password"
        :aria-label="t('users.copyPassword')"
        :title="t('users.copyPassword')"
        data-testid="password-copy"
        @click="copy"
      >
        <Copy class="h-4 w-4" />
      </button>
    </div>
    <p :id="id + '-hint'" class="mt-1 text-xs text-muted">{{ t('users.passwordHint') }}</p>
  </div>
  <div>
    <label class="label" :for="id + '-repeat'">{{ t('users.repeatPassword') + star }}</label>
    <input
      :id="id + '-repeat'"
      v-model="repeat"
      class="input font-mono"
      :type="visible ? 'text' : 'password'"
      autocomplete="new-password"
      :required="required"
      :aria-invalid="mismatch"
      data-testid="user-password-repeat"
    />
    <p v-if="mismatch" class="mt-1 text-xs text-danger" role="alert">{{ t('users.passwordMismatch') }}</p>
  </div>
</template>
