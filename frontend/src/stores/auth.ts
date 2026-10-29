import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { api, ApiError, setCsrfToken } from '../api/client'
import type { Me, Preferences } from '../api/types'

export const useAuth = defineStore('auth', () => {
  const me = ref<Me | null>(null)
  const loaded = ref(false)

  const user = computed(() => me.value?.user ?? null)
  const isAdmin = computed(() => me.value?.isAdmin ?? false)

  function has(perm: string) {
    return !!me.value && (me.value.isAdmin || me.value.permissions.includes(perm))
  }

  function apply(m: Me | null) {
    me.value = m
    setCsrfToken(m?.csrfToken ?? '')
  }

  async function load() {
    try {
      apply(await api.get<Me>('/me'))
    } catch (e) {
      if (!(e instanceof ApiError) || e.status !== 401) throw e
      apply(null)
    } finally {
      loaded.value = true
    }
  }

  async function login(username: string, password: string, newPassword?: string, totpCode?: string) {
    apply(await api.post<Me>('/auth/login', { username, password, newPassword, totpCode }))
  }

  async function logout() {
    try {
      await api.post('/auth/logout')
    } finally {
      apply(null)
    }
  }

  async function savePreferences(p: Partial<Preferences>) {
    if (!me.value) return
    const next = { ...me.value.preferences, ...p }
    me.value.preferences = await api.put<Preferences>('/me/preferences', next)
  }

  return { me, loaded, user, isAdmin, has, load, login, logout, savePreferences, clear: () => apply(null) }
})
