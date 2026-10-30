<script setup lang="ts">
import { Folder, Pencil, Plus, Trash2 } from '@lucide/vue'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { ApiError, api } from '../api/client'
import type { ConnectionGroup } from '../api/types'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import ErrorBox from '../components/ErrorBox.vue'
import PageHeader from '../components/PageHeader.vue'
import Panel from '../components/Panel.vue'
import FolderFormDialog from '../components/dialogs/FolderFormDialog.vue'

const { t } = useI18n()
const tree = ref<ConnectionGroup | null>(null)
const error = ref<unknown>(null)
// the folder dialog: null closed, {} new, {folder} rename or move
const editing = ref<{ folder?: { id: string; name: string; parentId: string } } | null>(null)
const del = ref<{ g: ConnectionGroup; recursive: boolean } | null>(null)

interface Row {
  g: ConnectionGroup
  depth: number
  count: number
}

function count(g: ConnectionGroup): number {
  return (g.connections?.length ?? 0) + (g.groups ?? []).reduce((n, c) => n + count(c), 0)
}

const rows = computed<Row[]>(() => {
  const out: Row[] = []
  const walk = (g: ConnectionGroup, depth: number) => {
    for (const c of g.groups ?? []) {
      out.push({ g: c, depth, count: count(c) })
      walk(c, depth + 1)
    }
  }
  if (tree.value) walk(tree.value, 0)
  return out
})

async function load() {
  tree.value = await api.get<ConnectionGroup>('/connection-groups/tree')
}

// parent choices: every folder except the edited one and its descendants (no cycles)
const parents = computed(() => {
  const skip = editing.value?.folder?.id
  const out: { id: string; label: string }[] = []
  let hidden = -1
  for (const r of rows.value) {
    if (hidden >= 0 && r.depth > hidden) continue
    hidden = -1
    if (r.g.id === skip) {
      hidden = r.depth
      continue
    }
    out.push({ id: r.g.id, label: '— '.repeat(r.depth) + r.g.name })
  }
  return out
})

function saved() {
  editing.value = null
  load()
}

async function remove() {
  const d = del.value
  del.value = null
  if (!d) return
  try {
    await api.del(`/connection-groups/${d.g.id}`, d.recursive ? { recursive: true } : undefined)
    load()
  } catch (err) {
    if (err instanceof ApiError && err.code === 'GROUP_NOT_EMPTY') del.value = { g: d.g, recursive: true }
    else error.value = err
  }
}
onMounted(load)
</script>

<template>
  <div class="page-fill page">
    <PageHeader :title="t('connectionGroups.title')" :subtitle="t('connectionGroups.subtitle')">
      <button class="btn btn-primary" data-testid="new-folder" @click="editing = {}">
        <Plus class="h-4 w-4" /> {{ t('connectionGroups.new') }}
      </button>
    </PageHeader>
    <ErrorBox :error="error" />
    <Panel fill>
      <table class="table">
        <thead>
          <tr>
            <th>{{ t('common.name') }}</th>
            <th>{{ t('nav.connections') }}</th>
            <th class="text-right">{{ t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in rows" :key="r.g.id">
            <td :style="{ paddingLeft: `${r.depth * 1.5 + 0.75}rem` }">
              <Folder class="mr-2 inline h-4 w-4 text-accent" />{{ r.g.name }}
            </td>
            <td class="text-muted">{{ t('connectionGroups.connections', { n: r.count }) }}</td>
            <td class="text-right">
              <div v-if="r.g.canEdit" class="inline-flex gap-2">
                <RouterLink :to="{ path: '/connections/new', query: { parentId: r.g.id } }" class="btn btn-sm"
                  >+ {{ t('connections.new') }}</RouterLink
                >
                <button
                  class="btn btn-sm"
                  :aria-label="t('common.edit')"
                  @click="editing = { folder: { id: r.g.id, name: r.g.name, parentId: r.g.parentId ?? '' } }"
                >
                  <Pencil class="h-4 w-4" />
                </button>
                <button
                  class="btn btn-sm"
                  :aria-label="t('common.delete')"
                  @click="del = { g: r.g, recursive: r.count > 0 || !!r.g.groups?.length }"
                >
                  <Trash2 class="h-4 w-4" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </Panel>
    <FolderFormDialog
      v-if="editing"
      :folder="editing.folder"
      :parents="parents"
      @close="editing = null"
      @saved="saved"
    />
    <ConfirmDialog
      :open="!!del"
      danger
      :title="t('common.delete')"
      :message="
        del
          ? t('connectionGroups.deleteConfirm', { name: del.g.name }) +
            (del.recursive ? ' ' + t('connectionGroups.deleteRecursive') : '')
          : ''
      "
      :type-to-confirm="del?.recursive ? del.g.name : undefined"
      @confirm="remove"
      @cancel="del = null"
    />
  </div>
</template>
