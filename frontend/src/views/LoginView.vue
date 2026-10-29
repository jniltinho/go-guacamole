<script setup lang="ts">
import { LogIn } from '@lucide/vue'
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import { ApiError } from '../api/client'
import AppLogo from '../components/AppLogo.vue'
import ErrorBox from '../components/ErrorBox.vue'
import LocalePicker from '../components/LocalePicker.vue'
import ThemePicker from '../components/ThemePicker.vue'
import { useAuth } from '../stores/auth'

const auth = useAuth()
const router = useRouter()
const route = useRoute()
const { t } = useI18n()

const username = ref('')
const password = ref('')
const newPassword = ref('')
const confirm = ref('')
const mustChange = ref(false)
const error = ref<unknown>(null)
const busy = ref(false)
const mismatch = ref(false)

async function submit() {
  error.value = null
  mismatch.value = false
  if (mustChange.value && newPassword.value !== confirm.value) {
    mismatch.value = true
    return
  }
  busy.value = true
  try {
    await auth.login(username.value, password.value, mustChange.value ? newPassword.value : undefined)
    const next =
      typeof route.query.next === 'string' && route.query.next.startsWith('/')
        ? route.query.next
        : '/dashboard'
    router.replace(next)
  } catch (e) {
    if (e instanceof ApiError && e.code === 'PASSWORD_CHANGE_REQUIRED') mustChange.value = true
    else error.value = e
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="login-page app-min-h relative px-4">
    <div class="login-tools absolute top-3 right-3 flex items-center gap-0.5 p-0.5">
      <ThemePicker />
      <LocalePicker />
    </div>

    <main class="mx-auto w-full max-w-[380px] pt-[14vh] pb-10">
      <form class="card" data-testid="login-form" novalidate @submit.prevent="submit">
        <!-- single child: themes with decorated title bars (xterm, Mac OS, XP, 98) stay aligned -->
        <div class="card-title px-5 py-2.5">
          <h1 class="text-sm font-semibold tracking-wide">{{ t('app.name') }}</h1>
        </div>

        <div class="px-7 pt-6 pb-7">
          <div class="mb-6 flex flex-col items-center text-center">
            <AppLogo class="mb-3 h-12 w-12 text-fg" />
            <h2 class="text-xl font-bold">{{ mustChange ? t('login.changeSubmit') : t('login.title') }}</h2>
            <p class="mt-1 text-sm text-muted">{{ t('login.subtitle') }}</p>
          </div>

          <div class="space-y-4">
            <div>
              <label class="label" for="u">{{ t('login.username') }}</label>
              <input
                id="u"
                v-model="username"
                class="input"
                autocomplete="username"
                autofocus
                required
                :disabled="mustChange"
              />
            </div>
            <div>
              <label class="label" for="p">{{ t('login.password') }}</label>
              <input
                id="p"
                v-model="password"
                class="input"
                type="password"
                autocomplete="current-password"
                required
                :disabled="mustChange"
              />
            </div>

            <template v-if="mustChange">
              <p class="border-l-4 border-warning bg-warning/10 px-3 py-2 text-sm">
                {{ t('login.mustChange') }}
              </p>
              <div>
                <label class="label" for="np">{{ t('login.newPassword') }}</label>
                <input
                  id="np"
                  v-model="newPassword"
                  class="input"
                  type="password"
                  autocomplete="new-password"
                  required
                />
              </div>
              <div>
                <label class="label" for="cp">{{ t('login.confirmPassword') }}</label>
                <input
                  id="cp"
                  v-model="confirm"
                  class="input"
                  type="password"
                  autocomplete="new-password"
                  required
                />
              </div>
              <p v-if="mismatch" class="text-sm text-danger" role="alert">{{ t('login.mismatch') }}</p>
            </template>

            <ErrorBox :error="error" />

            <button class="btn btn-primary w-full justify-center py-2.5" :disabled="busy" type="submit">
              <LogIn class="h-4 w-4" />
              {{ mustChange ? t('login.changeSubmit') : t('login.submit') }}
            </button>
          </div>
        </div>
      </form>
    </main>
  </div>
</template>

<style scoped>
.login-tools {
  background: color-mix(in srgb, var(--surface) 72%, transparent);
  backdrop-filter: blur(4px);
}
</style>
