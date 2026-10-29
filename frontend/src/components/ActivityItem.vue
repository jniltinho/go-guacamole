<script setup lang="ts">
import { CircleStop, FileText, Film, LogIn, Play, Settings, ShieldAlert, UserPlus, Users } from '@lucide/vue'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type { AuditEvent } from '../api/types'
import { relative } from '../lib/format'

const props = defineProps<{ event: AuditEvent }>()
const { t, te } = useI18n()

const icon = computed(() => {
  const a = props.event.action
  if (a === 'tunnel.open') return { c: Play, bg: 'var(--success)' }
  if (a === 'tunnel.close' || a === 'tunnel.kill') return { c: CircleStop, bg: 'var(--danger)' }
  if (a.startsWith('auth.login.failure') || a.startsWith('auth.login.denied'))
    return { c: ShieldAlert, bg: 'var(--warning)' }
  if (a.startsWith('auth.')) return { c: LogIn, bg: 'var(--accent)' }
  if (a === 'user.create') return { c: UserPlus, bg: 'var(--accent)' }
  if (a.startsWith('user_group') || a.startsWith('permissions')) return { c: Users, bg: 'var(--proto-vnc)' }
  if (a.startsWith('connection')) return { c: Settings, bg: 'var(--proto-ssh)' }
  if (a.startsWith('recording')) return { c: Film, bg: 'var(--proto-vnc)' }
  return { c: FileText, bg: 'var(--muted)' }
})

const text = computed(() => {
  const k = 'activity.' + props.event.action.replaceAll('.', '_')
  const args = { actor: props.event.actorName, target: props.event.targetName }
  return te(k) ? t(k, args) : `${props.event.action} ${props.event.targetName}`
})
const meta = computed(() => {
  const m = props.event.metadata ?? {}
  if (props.event.action.startsWith('tunnel.') && m.protocol)
    return `${m.ip ?? props.event.clientIp} (${String(m.protocol).toUpperCase()})`
  return props.event.actorName ? t('activity.by', { actor: props.event.actorName }) : props.event.clientIp
})
</script>

<template>
  <li class="flex items-start gap-3 px-4 py-3">
    <span
      class="flex h-8 w-8 shrink-0 items-center justify-center text-white"
      :style="{ background: icon.bg }"
    >
      <component :is="icon.c" class="h-4 w-4" />
    </span>
    <span class="min-w-0 flex-1">
      <span class="block truncate text-sm font-medium">{{ text }}</span>
      <span class="block truncate text-xs text-muted">{{ meta }}</span>
    </span>
    <span class="shrink-0 text-xs text-muted">{{ relative((k, n) => t(k, { n }, n), event.at) }}</span>
  </li>
</template>
