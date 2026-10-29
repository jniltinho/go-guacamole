<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { api } from '../api/client'
import type { AuditEvent, List } from '../api/types'
import EmptyState from '../components/EmptyState.vue'
import PageHeader from '../components/PageHeader.vue'
import Pager from '../components/Pager.vue'
import Panel from '../components/Panel.vue'

const { t, te, d } = useI18n()
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

function describe(e: AuditEvent) {
  const k = 'activity.' + e.action.replaceAll('.', '_')
  return te(k) ? t(k, { actor: e.actorName, target: e.targetName }) : e.action
}
</script>

<template>
  <div class="mx-auto max-w-[2200px]">
    <PageHeader :title="t('audit.title')" :subtitle="t('audit.subtitle')" />
    <Panel>
      <div class="flex flex-wrap gap-2 border-b border-line p-3">
        <input v-model.lazy="actor" class="input max-w-xs" :placeholder="t('audit.filterActor')" />
        <input v-model.lazy="action" class="input max-w-xs" :placeholder="t('audit.filterAction')" />
      </div>
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
            <td>{{ e.actorName || '—' }}</td>
            <td>
              <div>{{ describe(e) }}</div>
              <code class="text-xs text-muted">{{ e.action }}</code>
            </td>
            <td>
              {{ e.targetName }} <span class="text-xs text-muted">{{ e.targetType }}</span>
            </td>
            <td :class="e.outcome === 'success' ? 'text-success' : 'text-danger'">{{ e.outcome }}</td>
            <td class="font-mono text-xs">{{ e.clientIp }}</td>
          </tr>
        </tbody>
      </table>
      <EmptyState v-else :text="t('common.noResults')" />
      <Pager v-model="offset" :total="total" :limit="limit" />
    </Panel>
  </div>
</template>
