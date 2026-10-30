<script setup lang="ts">
import { Folder, List, Network, Pencil, Star } from '@lucide/vue'
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import { api } from '../api/client'
import type { Connection, ConnectionGroup, List as ApiList } from '../api/types'
import ConnIcon from '../components/ConnIcon.vue'
import EmptyState from '../components/EmptyState.vue'
import Skeleton from '../components/Skeleton.vue'
import ErrorBox from '../components/ErrorBox.vue'
import PageHeader from '../components/PageHeader.vue'
import Pager from '../components/Pager.vue'
import Panel from '../components/Panel.vue'
import ProtocolBadge from '../components/ProtocolBadge.vue'
import StatusDot from '../components/StatusDot.vue'
import TreeNode from '../components/TreeNode.vue'
import { useConnect } from '../composables/useConnect'
import { PROTOCOLS } from '../lib/protocols'
import { useAuth } from '../stores/auth'

const { t } = useI18n()
const auth = useAuth()
const route = useRoute()
const router = useRouter()
const { connect } = useConnect()

const view = ref<'table' | 'tree'>((route.query.view as 'tree') === 'tree' ? 'tree' : 'table')
const q = ref((route.query.q as string) ?? '')
const protocol = ref('')
const groupId = ref('')
const onlyFav = ref(false)
const sort = ref('name')
const offset = ref(0)
const limit = 25
const items = ref<Connection[]>([])
const loaded = ref(false)
const total = ref(0)
const groups = ref<ConnectionGroup[]>([])
const tree = ref<ConnectionGroup | null>(null)
const error = ref<unknown>(null)

async function load() {
  try {
    if (view.value === 'tree') {
      tree.value = await api.get<ConnectionGroup>('/connection-groups/tree')
    } else {
      const res = await api.get<ApiList<Connection>>('/connections', {
        q: q.value,
        protocol: protocol.value,
        groupId: groupId.value,
        favorite: onlyFav.value || undefined,
        sort: sort.value,
        limit,
        offset: offset.value,
      })
      items.value = res.items
      loaded.value = true
      total.value = res.total
    }
    error.value = null
  } catch (e) {
    error.value = e
    loaded.value = true // stop the skeleton: the error box tells what happened
  }
}

async function toggleFav(c: Connection) {
  if (c.favorite) await api.del(`/favorites/${c.id}`)
  else await api.post('/favorites', { connectionId: c.id })
  c.favorite = !c.favorite
}

function sortBy(k: string) {
  sort.value = sort.value === k ? '-' + k : k
}

watch(
  () => route.query.q,
  (v) => (q.value = (v as string) ?? ''),
)
watch([q, protocol, groupId, onlyFav, sort], () => {
  offset.value = 0
  load()
})
watch([offset, view], () => {
  router.replace({ query: { ...route.query, view: view.value === 'tree' ? 'tree' : undefined } })
  load()
})
onMounted(async () => {
  load()
  groups.value = (await api.get<ApiList<ConnectionGroup>>('/connection-groups')).items
})

const SortIcon = (k: string) => (sort.value === k ? '▲' : sort.value === '-' + k ? '▼' : '')
</script>

<template>
  <div class="page-fill mx-auto w-full max-w-[2200px]">
    <PageHeader :title="t('connections.title')" :subtitle="t('connections.subtitle')">
      <div class="flex border border-line">
        <button
          class="px-3 py-1.5 text-sm"
          :class="view === 'table' ? 'bg-accent text-accent-fg' : 'bg-surface text-fg'"
          @click="view = 'table'"
        >
          <List class="inline h-4 w-4" /> {{ t('connections.viewTable') }}
        </button>
        <button
          class="px-3 py-1.5 text-sm"
          :class="view === 'tree' ? 'bg-accent text-accent-fg' : 'bg-surface text-fg'"
          @click="view = 'tree'"
        >
          <Network class="inline h-4 w-4" /> {{ t('connections.viewTree') }}
        </button>
      </div>
      <RouterLink v-if="auth.has('CREATE_CONNECTION')" to="/connections/new" class="btn btn-primary">
        + {{ t('connections.new') }}
      </RouterLink>
    </PageHeader>
    <ErrorBox :error="error" />

    <Panel fill v-if="view === 'table'">
      <template #toolbar>
        <div class="flex flex-wrap gap-2 border-b border-line p-3">
          <input
            v-model.lazy="q"
            class="input max-w-xs"
            type="search"
            :placeholder="t('common.search')"
            data-testid="conn-search"
          />
          <select v-model="protocol" class="input w-auto">
            <option value="">{{ t('connections.allProtocols') }}</option>
            <option v-for="p in PROTOCOLS" :key="p" :value="p">{{ p.toUpperCase() }}</option>
          </select>
          <select v-model="groupId" class="input w-auto">
            <option value="">{{ t('connections.allGroups') }}</option>
            <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option>
          </select>
          <label class="flex items-center gap-2 px-2 text-sm">
            <input v-model="onlyFav" type="checkbox" /> {{ t('connections.onlyFavorites') }}
          </label>
        </div>
      </template>
      <table v-if="items.length" class="table" data-testid="conn-table">
        <thead>
          <tr>
            <th class="w-8"></th>
            <th class="cursor-pointer" @click="sortBy('name')">
              {{ t('common.name') }} {{ SortIcon('name') }}
            </th>
            <th class="cursor-pointer" @click="sortBy('protocol')">
              {{ t('common.protocol') }} {{ SortIcon('protocol') }}
            </th>
            <th class="cursor-pointer" @click="sortBy('group')">
              {{ t('common.group') }} {{ SortIcon('group') }}
            </th>
            <th class="cursor-pointer" @click="sortBy('status')">
              {{ t('common.status') }} {{ SortIcon('status') }}
            </th>
            <th>{{ t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in items" :key="c.id">
            <td>
              <button
                :aria-label="c.favorite ? t('connections.removeFavorite') : t('connections.addFavorite')"
                @click="toggleFav(c)"
              >
                <Star class="h-4 w-4" :class="c.favorite ? 'fill-warning text-warning' : 'text-muted'" />
              </button>
            </td>
            <td>
              <div class="flex items-center gap-3">
                <ConnIcon :protocol="c.protocol" class="h-6 w-6 shrink-0 text-muted" />
                <div class="min-w-0">
                  <div class="conn-name truncate font-semibold">{{ c.name }}</div>
                  <div class="host truncate text-xs text-muted">
                    {{ c.hostname }}{{ c.port ? ':' + c.port : '' }}
                  </div>
                </div>
              </div>
            </td>
            <td><ProtocolBadge :protocol="c.protocol" /></td>
            <td class="text-muted">
              <Folder class="mr-1 inline h-4 w-4" />{{ c.groupPath.join(' / ') || t('common.root') }}
            </td>
            <td>
              <StatusDot :status="c.status" />
              <span v-if="c.activeSessions" class="ml-2 text-xs text-muted">{{
                t('connections.active', { n: c.activeSessions })
              }}</span>
            </td>
            <td>
              <div class="flex gap-2">
                <button class="btn btn-primary btn-sm" data-testid="connect" @click="connect(c)">
                  {{ t('connections.connect') }}
                </button>
                <RouterLink
                  v-if="c.canEdit"
                  :to="`/connections/${c.id}/edit`"
                  class="btn btn-sm"
                  :aria-label="t('common.edit')"
                >
                  <Pencil class="h-4 w-4" />
                </RouterLink>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
      <Skeleton v-else-if="!loaded" />
      <EmptyState v-else :text="t('common.noResults')" />
      <template #footer><Pager v-model="offset" :total="total" :limit="limit" /></template>
    </Panel>

    <Panel v-else>
      <div v-if="tree" class="p-2">
        <TreeNode :node="tree" root @connect="connect" />
      </div>
    </Panel>
  </div>
</template>
