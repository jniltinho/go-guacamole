<script setup lang="ts">
import { Folder } from '@lucide/vue'
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import { api } from '../api/client'
import type { Connection, ConnectionGroup, Grants, List, Source, User, UserGroup } from '../api/types'
import ErrorBox from '../components/ErrorBox.vue'
import PageHeader from '../components/PageHeader.vue'
import Panel from '../components/Panel.vue'
import ProtocolBadge from '../components/ProtocolBadge.vue'

const SYSTEM = ['ADMINISTER', 'CREATE_CONNECTION', 'CREATE_USER', 'AUDIT']
const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const users = ref<User[]>([])
const groups = ref<UserGroup[]>([])
const conns = ref<Connection[]>([])
const folders = ref<ConnectionGroup[]>([])
const subject = ref((route.query.subject as string) ?? '')
const system = ref<string[]>([])
const connSel = ref<string[]>([])
const folderSel = ref<string[]>([])
const effective = ref<{ id: string; name: string; sources: Source[] }[]>([])
const error = ref<unknown>(null)
const notice = ref('')

const isUser = computed(() => subject.value.startsWith('user:'))

async function loadSubject() {
  notice.value = ''
  if (!subject.value) return
  router.replace({ query: { subject: subject.value } })
  try {
    const r = await api.get<{
      direct: Grants
      effective?: { connections: { id: string; name: string; sources: Source[] }[] }
    }>('/permissions', { subject: subject.value })
    system.value = r.direct.system
    connSel.value = r.direct.connections.map(String)
    folderSel.value = r.direct.connectionGroups.map(String)
    effective.value = r.effective?.connections ?? []
    error.value = null
  } catch (e) {
    error.value = e
  }
}

async function save() {
  try {
    await api.put('/permissions', {
      subject: subject.value,
      system: system.value,
      connections: connSel.value,
      connectionGroups: folderSel.value,
    })
    notice.value = t('permissions.saved')
    loadSubject()
  } catch (e) {
    error.value = e
  }
}

function sourceText(s: Source) {
  if (s.via === 'admin') return t('connections.source.admin')
  let txt =
    s.via === 'group' ? t('connections.source.group', { group: s.group }) : t('connections.source.direct')
  if (s.folder) txt += ' ' + t('connections.source.folder', { folder: s.folder })
  return txt
}

watch(subject, loadSubject)
onMounted(async () => {
  const [u, g, c, f] = await Promise.all([
    api.get<List<User>>('/users', { limit: 500 }),
    api.get<List<UserGroup>>('/user-groups'),
    api.get<List<Connection>>('/connections', { limit: 500 }),
    api.get<List<ConnectionGroup>>('/connection-groups'),
  ])
  users.value = u.items
  groups.value = g.items
  conns.value = c.items
  folders.value = f.items
  loadSubject()
})
</script>

<template>
  <div class="mx-auto max-w-[1400px]">
    <PageHeader class="mb-4" :title="t('permissions.title')" :subtitle="t('permissions.subtitle')">
      <select v-model="subject" class="input min-w-72" data-testid="perm-subject">
        <option value="">{{ t('permissions.selectSubject') }}</option>
        <optgroup :label="t('nav.users')">
          <option v-for="u in users" :key="u.id" :value="`user:${u.id}`">{{ u.username }}</option>
        </optgroup>
        <optgroup :label="t('nav.groups')">
          <option v-for="g in groups" :key="g.id" :value="`group:${g.id}`">{{ g.name }}</option>
        </optgroup>
      </select>
    </PageHeader>
    <ErrorBox :error="error" class="mb-4" />
    <div v-if="subject" class="grid gap-5 xl:grid-cols-3">
      <div class="space-y-5 xl:col-span-2">
        <Panel :title="t('permissions.system')">
          <div class="grid gap-2 p-4 md:grid-cols-2">
            <label v-for="p in SYSTEM" :key="p" class="flex items-center gap-2 text-sm">
              <input v-model="system" type="checkbox" :value="p" /> {{ t('permissions.perm.' + p) }}
            </label>
          </div>
        </Panel>
        <Panel :title="t('permissions.folders')">
          <div class="grid gap-2 p-4 md:grid-cols-2">
            <label v-for="f in folders" :key="f.id" class="flex items-center gap-2 text-sm">
              <input v-model="folderSel" type="checkbox" :value="f.id" />
              <Folder class="h-4 w-4 text-accent" /> {{ f.name }}
            </label>
          </div>
        </Panel>
        <Panel :title="t('permissions.connections')">
          <div class="grid gap-2 p-4 md:grid-cols-2">
            <label v-for="c in conns" :key="c.id" class="flex items-center gap-2 text-sm">
              <input v-model="connSel" type="checkbox" :value="c.id" :data-testid="'perm-conn-' + c.name" />
              <ProtocolBadge :protocol="c.protocol" /> {{ c.name }}
              <span class="text-xs text-muted">{{ c.groupPath.join(' / ') }}</span>
            </label>
          </div>
        </Panel>
        <p v-if="notice" class="text-sm text-success" role="status">{{ notice }}</p>
        <div class="flex justify-end">
          <button class="btn btn-primary" data-testid="perm-save" @click="save">
            {{ t('common.save') }}
          </button>
        </div>
      </div>
      <Panel v-if="isUser" :title="t('permissions.effective')">
        <ul class="divide-y divide-line">
          <li v-for="e in effective" :key="e.id" class="px-4 py-2">
            <div class="font-medium">{{ e.name }}</div>
            <div v-for="(s, i) in e.sources" :key="i" class="text-xs text-muted">{{ sourceText(s) }}</div>
          </li>
          <li v-if="!effective.length" class="px-4 py-6 text-center text-muted">{{ t('common.none') }}</li>
        </ul>
      </Panel>
    </div>
  </div>
</template>
