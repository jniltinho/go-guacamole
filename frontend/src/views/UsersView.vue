<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'

import { api } from '../api/client'
import type { List, User } from '../api/types'
import EmptyState from '../components/EmptyState.vue'
import Skeleton from '../components/Skeleton.vue'
import ErrorBox from '../components/ErrorBox.vue'
import PageHeader from '../components/PageHeader.vue'
import Pager from '../components/Pager.vue'
import Panel from '../components/Panel.vue'

const { t, d } = useI18n()
const users = ref<User[]>([])
const loaded = ref(false)
const total = ref(0)
const offset = ref(0)
const q = ref('')
const limit = 25
const error = ref<unknown>(null)

async function load() {
  try {
    const r = await api.get<List<User>>('/users', { q: q.value, limit, offset: offset.value })
    users.value = r.items
    loaded.value = true
    total.value = r.total
  } catch (e) {
    error.value = e
    loaded.value = true // stop the skeleton: the error box tells what happened
  }
}
watch(offset, load)
watch(q, () => {
  offset.value = 0
  load()
})
onMounted(load)
</script>

<template>
  <div class="page-fill page">
    <PageHeader :title="t('users.title')" :subtitle="t('users.subtitle')">
      <RouterLink to="/users/new" class="btn btn-primary">+ {{ t('users.new') }}</RouterLink>
    </PageHeader>
    <ErrorBox :error="error" />
    <Panel fill>
      <template #toolbar>
        <div class="border-b border-line p-3">
          <input v-model.lazy="q" type="search" class="input max-w-xs" :placeholder="t('common.search')" />
        </div>
      </template>
      <table v-if="users.length" class="table">
        <thead>
          <tr>
            <th>{{ t('users.username') }}</th>
            <th>{{ t('users.fullName') }}</th>
            <th>{{ t('users.email') }}</th>
            <th>{{ t('users.groups') }}</th>
            <th>{{ t('common.status') }}</th>
            <th>{{ t('users.created') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="u in users" :key="u.id">
            <td>
              <RouterLink :to="`/users/${u.id}`" class="font-semibold text-accent">{{
                u.username
              }}</RouterLink>
            </td>
            <td>{{ u.fullName ?? '—' }}</td>
            <td class="text-muted">{{ u.email ?? '—' }}</td>
            <td class="text-muted">{{ u.groups?.join(', ') || '—' }}</td>
            <td>
              <span :class="u.disabled ? 'text-danger' : 'text-success'">{{
                u.disabled ? t('common.disabled') : t('common.enabled')
              }}</span>
            </td>
            <td class="text-muted">{{ d(u.createdAt, 'short') }}</td>
          </tr>
        </tbody>
      </table>
      <Skeleton v-else-if="!loaded" />
      <EmptyState v-else :text="t('common.noResults')" />
      <template #footer><Pager v-model="offset" :total="total" :limit="limit" /></template>
    </Panel>
  </div>
</template>
