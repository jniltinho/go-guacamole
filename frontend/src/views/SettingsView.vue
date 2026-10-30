<script setup lang="ts">
import {
  AppWindow,
  Check,
  Info,
  KeyRound,
  Palette,
  Save,
  ShieldCheck,
  SlidersHorizontal,
  UserRound,
} from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { api } from '../api/client'
import ErrorBox from '../components/ErrorBox.vue'
import PageHeader from '../components/PageHeader.vue'
import PageTabs from '../components/PageTabs.vue'
import TerminalSchemePicker from '../components/TerminalSchemePicker.vue'
import { setAppTitle, useAppTitle } from '../composables/useAppTitle'
import { useEffects } from '../composables/useEffects'
import { useTheme } from '../composables/useTheme'
import { LOCALES, setLocale } from '../i18n'
import { useAuth } from '../stores/auth'
import { useToast } from '../composables/useToast'

const { t, te, d, locale } = useI18n()
const { show } = useToast()
const auth = useAuth()
const { themes, themeId, setTheme } = useTheme()

type Section = 'appearance' | 'preferences' | 'account' | 'security' | 'application' | 'about'
const section = ref<Section>('appearance')
const sections = computed(() =>
  (
    [
      { id: 'appearance', icon: Palette },
      { id: 'preferences', icon: SlidersHorizontal },
      { id: 'account', icon: UserRound },
      { id: 'security', icon: KeyRound },
      { id: 'application', icon: AppWindow, admin: true },
      { id: 'about', icon: Info },
    ] as { id: Section; icon: typeof Palette; admin?: boolean }[]
  ).filter((s) => !s.admin || auth.isAdmin),
)

const tabs = computed(() => sections.value.map((s) => ({ ...s, label: t('settings.sections.' + s.id) })))

// ---- application name (administrators; every user sees it) ----
const { title: appTitle, custom: appTitleCustom } = useAppTitle()
const brandTitle = ref(appTitleCustom.value)
const brandError = ref<unknown>(null)
const brandBusy = ref(false)
async function saveBranding() {
  brandError.value = null
  brandBusy.value = true
  try {
    const r = await api.put<{ title: string }>('/settings/branding', { title: brandTitle.value })
    brandTitle.value = r.title
    setAppTitle(r.title)
    show(t('common.saved'))
  } catch (e) {
    brandError.value = e
  } finally {
    brandBusy.value = false
  }
}

function chooseTheme(id: string) {
  setTheme(id)
  auth.savePreferences({ theme: id }).catch(() => {})
}
function chooseLocale(l: string) {
  setLocale(l)
  auth.savePreferences({ locale: l }).catch(() => {})
}
const { preference: effectsPref, setEffects } = useEffects()
function pickEffects(v: '' | 'full' | 'reduced' | 'off') {
  setEffects(v)
  auth.savePreferences({ effects: v }).catch(() => {})
}
// per-browser switch for the "/" search shortcut (single-key shortcuts must be avoidable)
const slashSearch = ref(safeGet('gg-slash-search') !== 'off')
watch(slashSearch, (v) => {
  try {
    localStorage.setItem('gg-slash-search', v ? 'on' : 'off')
  } catch {
    /* storage unavailable */
  }
})
function safeGet(k: string) {
  try {
    return localStorage.getItem(k)
  } catch {
    return null
  }
}
const collapsed = computed({
  get: () => auth.me?.preferences.sidebarCollapsed ?? false,
  set: (v: boolean) => auth.savePreferences({ sidebarCollapsed: v }).catch(() => {}),
})

const initials = computed(() =>
  (auth.user?.fullName || auth.user?.username || '')
    .split(/[\s._-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0]!.toUpperCase())
    .join(''),
)

// ---- password ----
const current = ref('')
const next = ref('')
const confirm = ref('')
const error = ref<unknown>(null)
const busy = ref(false)
const strength = computed(() => {
  const p = next.value
  let s = 0
  if (p.length >= 8) s++
  if (p.length >= 12) s++
  if (/[a-z]/.test(p) && /[A-Z]/.test(p)) s++
  if (/\d/.test(p)) s++
  if (/[^A-Za-z0-9]/.test(p)) s++
  return Math.min(4, s)
})
const mismatch = computed(() => confirm.value !== '' && confirm.value !== next.value)

async function changePassword() {
  error.value = null
  if (next.value !== confirm.value) return
  busy.value = true
  try {
    await api.put('/me/password', { currentPassword: current.value, newPassword: next.value })
    current.value = next.value = confirm.value = ''
    show(t('settings.passwordChanged'))
  } catch (e) {
    error.value = e
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <!-- fits the viewport on desktop: only the tab's content scrolls, never the page -->
  <div class="page-fill page">
    <PageHeader :title="t('settings.title')" :subtitle="t('settings.subtitle')" />
    <PageTabs v-model="section" name="settings" :tabs="tabs" :label="t('settings.title')">
      <!-- Appearance: theme -->
      <section v-if="section === 'appearance'" class="space-y-5">
        <div class="card p-5">
          <h2 class="section-title">{{ t('common.theme') }}</h2>
          <p class="mb-4 text-sm text-muted">{{ t('settings.themeHint') }}</p>
          <div class="grid grid-cols-2 gap-3 sm:grid-cols-4 2xl:grid-cols-8">
            <button
              v-for="th in themes"
              :key="th.id"
              class="theme-card lift text-left"
              :class="{ selected: th.id === themeId }"
              :aria-pressed="th.id === themeId"
              :data-theme-card="th.id"
              @click="chooseTheme(th.id)"
            >
              <!-- miniature of the app shell rendered with the theme's own tokens -->
              <span class="theme-swatch preview" :data-theme="th.id">
                <span class="pv-side" />
                <span class="pv-main">
                  <span class="pv-bar" />
                  <span class="pv-cards"><span /><span /><span /></span>
                  <span class="pv-btn" />
                </span>
              </span>
              <span class="flex items-center justify-between gap-2 px-2.5 py-2 text-sm">
                <span class="truncate">{{ te('themes.' + th.id) ? t('themes.' + th.id) : th.label }}</span>
                <Check v-if="th.id === themeId" class="h-4 w-4 shrink-0 text-accent" />
              </span>
            </button>
          </div>
        </div>
      </section>

      <!-- Preferences: language, effects, terminal, layout -->
      <section v-else-if="section === 'preferences'">
        <div class="card grid gap-5 p-5 md:grid-cols-2">
          <div>
            <h2 class="section-title">{{ t('common.language') }}</h2>
            <p class="mb-3 text-sm text-muted">{{ t('settings.languageHint') }}</p>
            <div class="inline-flex border border-line" role="radiogroup">
              <button
                v-for="l in LOCALES"
                :key="l"
                role="radio"
                :aria-checked="l === locale"
                class="px-4 py-2 text-sm"
                :class="l === locale ? 'bg-accent text-accent-fg' : 'hover:bg-surface-2'"
                @click="chooseLocale(l)"
              >
                {{ t('locales.' + l) }}
              </button>
            </div>
          </div>
          <div>
            <h2 class="section-title">{{ t('settings.effects') }}</h2>
            <p class="mb-3 text-sm text-muted">{{ t('settings.effectsHint') }}</p>
            <div class="flex flex-wrap gap-2" role="radiogroup" :aria-label="t('settings.effects')">
              <button
                v-for="e in ['', 'full', 'reduced', 'off'] as const"
                :key="e"
                type="button"
                role="radio"
                :aria-checked="effectsPref === e"
                class="btn btn-sm"
                :class="{ 'btn-primary': effectsPref === e }"
                :data-testid="'effects-' + (e || 'system')"
                @click="pickEffects(e)"
              >
                {{ t('settings.effectsOptions.' + (e || 'system')) }}
              </button>
            </div>
          </div>
          <div>
            <h2 class="section-title">{{ t('settings.terminalScheme') }}</h2>
            <p class="mb-3 text-sm text-muted">{{ t('settings.terminalSchemeHint') }}</p>
            <TerminalSchemePicker
              :model-value="auth.me?.preferences.terminalScheme ?? ''"
              :ui-theme="themeId"
              @update:model-value="(v) => auth.savePreferences({ terminalScheme: v }).catch(() => {})"
            />
          </div>
          <div>
            <h2 class="section-title">{{ t('settings.layout') }}</h2>
            <p class="mb-3 text-sm text-muted">{{ t('settings.layoutHint') }}</p>
            <label class="flex items-center gap-3 text-sm">
              <input v-model="collapsed" type="checkbox" class="h-4 w-4 accent-[var(--accent)]" />
              {{ t('settings.compactSidebar') }}
            </label>
            <label class="mt-2 flex items-center gap-3 text-sm">
              <input v-model="slashSearch" type="checkbox" class="h-4 w-4 accent-[var(--accent)]" />
              {{ t('settings.slashSearch') }}
            </label>
          </div>
        </div>
      </section>

      <!-- Account -->
      <section v-else-if="section === 'account'" class="space-y-5">
        <div class="card flex flex-wrap items-center gap-5 p-5">
          <span
            class="flex h-16 w-16 items-center justify-center bg-accent text-2xl font-bold text-accent-fg"
            >{{ initials }}</span
          >
          <div class="min-w-0 flex-1">
            <div class="text-xl font-bold">{{ auth.user?.fullName || auth.user?.username }}</div>
            <div class="text-sm text-muted">
              @{{ auth.user?.username }}<template v-if="auth.user?.email"> · {{ auth.user.email }}</template>
            </div>
          </div>
          <span
            class="badge"
            :class="
              auth.isAdmin ? 'bg-accent text-accent-fg' : 'border border-line bg-surface-raised text-fg'
            "
          >
            {{ auth.isAdmin ? t('common.role.admin') : t('common.role.user') }}
          </span>
        </div>
        <div class="card p-5">
          <dl class="grid gap-x-8 gap-y-5 text-sm sm:grid-cols-2">
            <div>
              <dt class="label">{{ t('users.groups') }}</dt>
              <dd class="flex flex-wrap gap-1.5">
                <span v-for="g in auth.me?.groups" :key="g" class="chip">{{ g }}</span>
                <span v-if="!auth.me?.groups.length" class="text-muted">—</span>
              </dd>
            </div>
            <div>
              <dt class="label">{{ t('nav.permissions') }}</dt>
              <dd class="flex flex-wrap gap-1.5">
                <span v-for="p in auth.me?.permissions" :key="p" class="chip"
                  ><ShieldCheck class="h-3.5 w-3.5" />{{ t('permissions.perm.' + p) }}</span
                >
                <span v-if="!auth.me?.permissions.length" class="text-muted">—</span>
              </dd>
            </div>
            <div>
              <dt class="label">{{ t('users.created') }}</dt>
              <dd>{{ auth.user ? d(auth.user.createdAt, 'long') : '—' }}</dd>
            </div>
            <div>
              <dt class="label">{{ t('settings.sessionExpires') }}</dt>
              <dd>{{ auth.me ? d(auth.me.expiresAt, 'long') : '—' }}</dd>
            </div>
          </dl>
        </div>
      </section>

      <!-- Security -->
      <section v-else-if="section === 'security'" class="card p-5">
        <h2 class="section-title">{{ t('settings.password') }}</h2>
        <p class="mb-4 text-sm text-muted">{{ t('settings.passwordHint') }}</p>
        <form class="max-w-md space-y-4" @submit.prevent="changePassword">
          <div>
            <label class="label" for="cur">{{ t('settings.currentPassword') }}</label>
            <input
              id="cur"
              v-model="current"
              class="input"
              type="password"
              autocomplete="current-password"
              required
            />
          </div>
          <div>
            <label class="label" for="new">{{ t('settings.newPassword') }}</label>
            <input
              id="new"
              v-model="next"
              class="input"
              type="password"
              autocomplete="new-password"
              minlength="8"
              required
            />
            <div class="mt-2 flex gap-1" aria-hidden="true">
              <span
                v-for="i in 4"
                :key="i"
                class="h-1 flex-1"
                :style="{
                  background:
                    i <= strength
                      ? ['var(--danger)', 'var(--warning)', 'var(--accent)', 'var(--success)'][strength - 1]
                      : 'var(--border)',
                }"
              />
            </div>
            <p class="mt-1 text-xs text-muted">{{ t('settings.strength.' + strength) }}</p>
          </div>
          <div>
            <label class="label" for="conf">{{ t('settings.confirmPassword') }}</label>
            <input
              id="conf"
              v-model="confirm"
              class="input"
              type="password"
              autocomplete="new-password"
              required
            />
            <p v-if="mismatch" class="mt-1 text-sm text-danger">{{ t('login.mismatch') }}</p>
          </div>
          <ErrorBox :error="error" />
          <button class="btn btn-primary" type="submit" :disabled="busy || mismatch || !next">
            <KeyRound class="h-4 w-4" /> {{ t('settings.password') }}
          </button>
        </form>
      </section>

      <!-- Application (administrators) -->
      <section v-else-if="section === 'application'" class="card p-5">
        <form class="max-w-lg space-y-3" data-testid="branding-form" @submit.prevent="saveBranding">
          <h2 class="section-title">{{ t('settings.appName') }}</h2>
          <p class="text-sm text-muted">{{ t('settings.appNameHint') }}</p>
          <label class="sr-only" for="app-title">{{ t('settings.appName') }}</label>
          <input
            id="app-title"
            v-model="brandTitle"
            class="input"
            maxlength="40"
            :placeholder="t('app.name')"
            data-testid="branding-title"
          />
          <ErrorBox :error="brandError" />
          <button class="btn btn-primary" type="submit" :disabled="brandBusy" data-testid="branding-save">
            <Save class="h-4 w-4" /> {{ t('common.save') }}
          </button>
        </form>
      </section>

      <!-- About -->
      <section v-else class="card p-5">
        <h2 class="section-title">{{ appTitle }}</h2>
        <p class="mb-4 text-sm text-muted">{{ t('settings.aboutText') }}</p>
        <dl class="grid gap-x-8 gap-y-3 text-sm sm:grid-cols-[160px_1fr]">
          <dt class="text-muted">{{ t('settings.version') }}</dt>
          <dd class="font-mono">{{ auth.me?.build.version }}</dd>
          <dt class="text-muted">Commit</dt>
          <dd class="font-mono">{{ auth.me?.build.gitCommit }}</dd>
          <dt class="text-muted">{{ t('settings.buildDate') }}</dt>
          <dd class="font-mono">{{ auth.me?.build.buildDate }}</dd>
          <dt class="text-muted">{{ t('recordings.title') }}</dt>
          <dd>{{ auth.me?.recording ? t('common.enabled') : t('common.disabled') }}</dd>
          <dt class="text-muted">{{ t('settings.source') }}</dt>
          <dd>
            <a
              class="text-accent"
              href="https://github.com/jniltinho/go-guacamole"
              target="_blank"
              rel="noopener"
              >github.com/jniltinho/go-guacamole</a
            >
          </dd>
        </dl>
      </section>
    </PageTabs>
  </div>
</template>

<style scoped>
.chip {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.15rem 0.55rem;
  font-size: 0.78rem;
  border: 1px solid var(--border);
  background: var(--surface-2);
}
.theme-card {
  border: 1px solid var(--border);
  background: var(--surface);
  cursor: pointer;
  transition:
    border-color var(--dur-1) var(--ease-out),
    box-shadow var(--dur-1) var(--ease-out);
}
.theme-card:hover {
  border-color: var(--muted);
}
.theme-card.selected {
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}
.preview {
  display: flex;
  height: 4.25rem;
  border-bottom: 1px solid var(--border);
  background: var(--bg);
  overflow: hidden;
}
.pv-side {
  width: 22%;
  background: var(--sidebar);
}
.pv-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  padding: 0.4rem;
}
.pv-bar {
  height: 0.55rem;
  background: var(--surface);
  border: 1px solid var(--border);
}
.pv-cards {
  display: flex;
  flex: 1;
  gap: 0.3rem;
}
.pv-cards > span {
  flex: 1;
  background: var(--surface);
  border: 1px solid var(--border);
  box-shadow: var(--shadow);
}
.pv-btn {
  width: 40%;
  height: 0.55rem;
  background: var(--accent);
}
</style>
