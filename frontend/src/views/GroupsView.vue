<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import { api } from '../api/client'
import type { List, UserGroup } from '../api/types'
import EmptyState from '../components/EmptyState.vue'
import ErrorBox from '../components/ErrorBox.vue'
import PageHeader from '../components/PageHeader.vue'
import Panel from '../components/Panel.vue'

const { t } = useI18n()
const router = useRouter()
const groups = ref<UserGroup[]>([])
const name = ref('')
const error = ref<unknown>(null)

async function load() {
  groups.value = (await api.get<List<UserGroup>>('/user-groups')).items
}
async function create() {
  error.value = null
  try {
    const g = await api.post<UserGroup>('/user-groups', { name: name.value })
    router.push(`/groups/${g.id}`)
  } catch (e) {
    error.value = e
  }
}
onMounted(load)
</script>

<template>
  <div class="page-fill page">
    <PageHeader :title="t('groups.title')" :subtitle="t('groups.subtitle')">
      <form class="flex gap-2" @submit.prevent="create">
        <input
          v-model="name"
          class="input"
          :placeholder="t('common.name')"
          required
          data-testid="group-name"
        />
        <button class="btn btn-primary" type="submit">+ {{ t('groups.new') }}</button>
      </form>
    </PageHeader>
    <ErrorBox :error="error" />
    <Panel fill>
      <table v-if="groups.length" class="table">
        <thead>
          <tr>
            <th>{{ t('common.name') }}</th>
            <th>{{ t('groups.members') }}</th>
            <th>{{ t('common.status') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="g in groups" :key="g.id">
            <td>
              <RouterLink :to="`/groups/${g.id}`" class="font-semibold text-accent">{{ g.name }}</RouterLink>
            </td>
            <td>{{ t('groups.memberCount', { n: g.memberCount }) }}</td>
            <td>
              <span :class="g.disabled ? 'text-danger' : 'text-success'">{{
                g.disabled ? t('common.disabled') : t('common.enabled')
              }}</span>
            </td>
          </tr>
        </tbody>
      </table>
      <EmptyState v-else :text="t('common.empty')" />
    </Panel>
  </div>
</template>
