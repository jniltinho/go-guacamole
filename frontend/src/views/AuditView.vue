<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'

import { api } from '../api/client'
import type { AuditEvent, List } from '../api/types'
import EmptyState from '../components/EmptyState.vue'
import PageHeader from '../components/PageHeader.vue'
import Pager from '../components/Pager.vue'
import AccessLog from '../components/AccessLog.vue'
import Panel from '../components/Panel.vue'

const { t, te, d } = useI18n()
const route = useRoute()
const view = ref<'events' | 'access'>(route.query.tab === 'access' ? 'access' : 'events')
const items = ref<AuditEvent[]>([])
const total = ref(0)
const offset = ref(0)
const actor = ref('')
const action = ref('')
const limit = 50

async function load() {
  const r = await api.get<List<AuditEvent>>('/audit', {
    actor: actor.value,
    action: action.value,
    limit,
    offset: offset.value,
  })
  items.value = r.items
  total.value = r.total
}
watch(offset, load)
watch([actor, action], () => {
  offset.value = 0
  load()
})
onMounted(load)
watch(view, (v) => v === 'events' && load())

function describe(e: AuditEvent) {
  const k = 'activity.' + e.action.replaceAll('.', '_')
  return te(k) ? t(k, { actor: e.actorName, target: e.targetName }) : e.action
}
</script>

<template>
  <div class="page-fill mx-auto w-full max-w-[2200px]">
    <PageHeader :title="t('audit.title')" :subtitle="t('audit.subtitle')" />
    <div class="flex shrink-0 border-b border-line" role="tablist">
      <button
        v-for="k in ['events', 'access'] as const"
        :key="k"
        role="tab"
        :aria-selected="view === k"
        class="-mb-px border-b-2 px-4 py-2 text-sm font-medium"
        :class="
          view === k
            ? 'border-accent font-semibold text-on-bg'
            : 'border-transparent text-on-bg-muted hover:text-on-bg'
        "
        :data-testid="'audit-tab-' + k"
        @click="view = k"
      >
        {{ k === 'events' ? t('access.events') : t('access.title') }}
      </button>
    </div>
    <Panel v-if="view === 'events'" fill>
      <template #toolbar>
        <div class="flex flex-wrap gap-2 border-b border-line p-3">
          <input v-model.lazy="actor" class="input max-w-xs" :placeholder="t('audit.filterActor')" />
          <input v-model.lazy="action" class="input max-w-xs" :placeholder="t('audit.filterAction')" />
        </div>
      </template>
      <table v-if="items.length" class="table">
        <thead>
          <tr>
            <th>{{ t('audit.when') }}</th>
            <th>{{ t('audit.actor') }}</th>
            <th>{{ t('audit.action') }}</th>
            <th>{{ t('audit.target') }}</th>
            <th>{{ t('audit.outcome') }}</th>
            <th>IP</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="e in items" :key="e.id">
            <td class="whitespace-nowrap">{{ d(e.at, 'long') }}</td>
            <td class="whitespace-nowrap">{{ e.actorName || '—' }}</td>
            <td class="max-w-[28rem] truncate" :title="e.action">
              {{ describe(e) }} <code class="text-xs text-muted">{{ e.action }}</code>
            </td>
            <td class="whitespace-nowrap">
              {{ e.targetName }} <span class="text-xs text-muted">{{ e.targetType }}</span>
            </td>
            <td :class="e.outcome === 'success' ? 'text-success' : 'text-danger'">{{ e.outcome }}</td>
            <td class="font-mono text-xs">{{ e.clientIp }}</td>
          </tr>
        </tbody>
      </table>
      <EmptyState v-else :text="t('common.noResults')" />
      <template #footer><Pager v-model="offset" :total="total" :limit="limit" /></template>
    </Panel>
    <Panel v-else fill>
      <AccessLog full />
    </Panel>
  </div>
</template>
