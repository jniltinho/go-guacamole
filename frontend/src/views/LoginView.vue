<script setup lang="ts">
import { KeyRound, LogIn } from '@lucide/vue'
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import { api, ApiError } from '../api/client'
import AppLogo from '../components/AppLogo.vue'
import ErrorBox from '../components/ErrorBox.vue'
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
// second factor: TOTP code (enroll = first time, with the secret to scan)
const totp = ref<null | 'code' | 'enroll'>(null)
const totpCode = ref('')
const enroll = ref<{ secret: string; uri: string; qr?: string } | null>(null)
const sso = ref<Array<{ label: string; url: string }>>([])
const version = ref('')

onMounted(async () => {
  if (typeof route.query.error === 'string')
    error.value = new ApiError(401, route.query.error, route.query.error)
  try {
    const m = await api.get<{
      header: boolean
      sso: Array<{ label: string; url: string }>
      version?: string
    }>('/auth/methods')
    sso.value = m.sso
    // release only: "0.2.0-4-gabc123-dirty" → "0.2.0"
    version.value = (m.version ?? '')
      .replace(/^v/, '')
      .replace(/-\d+-g[0-9a-f]+/, '')
      .replace(/-dirty$/, '')
    // proxy (header) sign-in: try once; a failure comes back with ?error=
    if (m.header && !route.query.error) location.href = '/api/v1/auth/header'
  } catch {
    /* methods are optional */
  }
})

async function submit() {
  error.value = null
  mismatch.value = false
  if (mustChange.value && newPassword.value !== confirm.value) {
    mismatch.value = true
    return
  }
  busy.value = true
  try {
    await auth.login(
      username.value,
      password.value,
      mustChange.value ? newPassword.value : undefined,
      totp.value ? totpCode.value : undefined,
    )
    const next =
      typeof route.query.next === 'string' && route.query.next.startsWith('/')
        ? route.query.next
        : '/dashboard'
    router.replace(next)
  } catch (e) {
    const code = e instanceof ApiError ? e.code : ''
    if (code === 'PASSWORD_CHANGE_REQUIRED') mustChange.value = true
    else if (code === 'TOTP_REQUIRED') totp.value = 'code'
    else if (code === 'TOTP_ENROLL') {
      totp.value = 'enroll'
      enroll.value = (e as ApiError).data as typeof enroll.value
    } else if (code === 'DUO_REQUIRED') {
      location.href = String((e as ApiError).data?.redirect)
      return
    } else {
      if (code === 'TOTP_INVALID') {
        totpCode.value = ''
        if ((e as ApiError).data) enroll.value = (e as ApiError).data as typeof enroll.value
      }
      error.value = e
    }
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="login-page app-min-h relative px-4">
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
                :disabled="mustChange || !!totp"
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
                :disabled="mustChange || !!totp"
              />
            </div>

            <template v-if="totp">
              <div v-if="totp === 'enroll' && enroll" class="space-y-2 text-center" data-testid="totp-enroll">
                <p class="text-sm">{{ t('login.totpEnroll') }}</p>
                <img v-if="enroll.qr" :src="enroll.qr" alt="QR code" class="mx-auto h-44 w-44 bg-white p-1" />
                <p class="font-mono text-xs break-all text-muted">{{ enroll.secret }}</p>
              </div>
              <div>
                <label class="label" for="totp">{{ t('login.totpCode') }}</label>
                <input
                  id="totp"
                  v-model="totpCode"
                  class="input text-center font-mono tracking-[0.4em]"
                  inputmode="numeric"
                  autocomplete="one-time-code"
                  maxlength="6"
                  pattern="[0-9]{6}"
                  autofocus
                  required
                  data-testid="totp-code"
                />
              </div>
            </template>

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
            <template v-if="sso.length && !mustChange && !totp">
              <div class="flex items-center gap-2 text-xs text-muted">
                <span class="h-px flex-1 bg-line" />{{ t('login.or') }}<span class="h-px flex-1 bg-line" />
              </div>
              <a
                v-for="s in sso"
                :key="s.url"
                :href="s.url"
                class="btn w-full justify-center py-2.5"
                data-testid="sso-button"
              >
                <KeyRound class="h-4 w-4" /> {{ s.label }}
              </a>
            </template>
          </div>
        </div>
      </form>
    </main>
    <!-- discreet: small, but in the page's own AAA text color (no opacity) -->
    <p
      v-if="version"
      class="absolute right-3 bottom-2 font-mono text-[11px] tracking-wide text-on-bg-muted"
      data-testid="login-version"
    >
      v{{ version }}
    </p>
  </div>
</template>
