<script setup lang="ts">
import { ShieldCheck, Trash2, UsersRound, X, SquarePen } from '@lucide/vue'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import { api } from '../api/client'
import type { List, User, UserGroup } from '../api/types'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import ErrorBox from '../components/ErrorBox.vue'
import PageHeader from '../components/PageHeader.vue'
import PageTabs from '../components/PageTabs.vue'
import Panel from '../components/Panel.vue'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const id = route.params.id as string
const group = ref<UserGroup | null>(null)
const name = ref('')
const disabled = ref(false)
const members = ref<string[]>([])
const users = ref<User[]>([])
const pick = ref('')
const error = ref<unknown>(null)
const notice = ref('')
const confirmDelete = ref(false)

type Tab = 'details' | 'members'
const tab = ref<Tab>('details')
const tabs = computed(() => [
  { id: 'details' as Tab, label: t('groups.details'), icon: SquarePen },
  { id: 'members' as Tab, label: `${t('groups.members')} (${members.value.length})`, icon: UsersRound },
])

const byId = computed(() => new Map(users.value.map((u) => [u.id, u])))
const candidates = computed(() => users.value.filter((u) => !members.value.includes(u.id)))

onMounted(async () => {
  const r = await api.get<{ group: UserGroup; members: string[] }>(`/user-groups/${id}`)
  group.value = r.group
  name.value = r.group.name
  disabled.value = r.group.disabled
  members.value = r.members
  users.value = (await api.get<List<User>>('/users', { limit: 500 })).items
})

async function save() {
  error.value = null
  notice.value = ''
  try {
    await api.put(`/user-groups/${id}`, { name: name.value, disabled: disabled.value })
    await api.put(`/user-groups/${id}/members`, { members: members.value })
    notice.value = t('common.saved')
  } catch (e) {
    error.value = e
  }
}

function add() {
  if (pick.value) members.value.push(pick.value)
  pick.value = ''
}

async function remove() {
  confirmDelete.value = false
  try {
    await api.del(`/user-groups/${id}`)
    router.push('/groups')
  } catch (e) {
    error.value = e
  }
}
</script>

<template>
  <div class="page-fill page">
    <PageHeader :title="group?.name ?? ''">
      <RouterLink :to="{ path: '/permissions', query: { subject: `group:${id}` } }" class="btn">
        <ShieldCheck class="h-4 w-4" /> {{ t('nav.permissions') }}
      </RouterLink>
      <button class="btn btn-danger" @click="confirmDelete = true">
        <Trash2 class="h-4 w-4" /> {{ t('common.delete') }}
      </button>
    </PageHeader>
    <form class="flex min-h-0 flex-1 flex-col gap-4" @submit.prevent="save">
      <PageTabs v-model="tab" name="group" :tabs="tabs" :label="t('groups.details')">
        <Panel v-if="tab === 'details'" :title="t('groups.details')">
          <div class="grid gap-4 p-4 md:grid-cols-2">
            <div>
              <label class="label">{{ t('common.name') }}</label>
              <input v-model="name" class="input" required />
            </div>
            <label class="flex items-center gap-2 pt-5 text-sm"
              ><input v-model="disabled" type="checkbox" /> {{ t('common.disabled') }}</label
            >
          </div>
        </Panel>
        <Panel v-else :title="t('groups.members')">
          <div class="flex gap-2 border-b border-line p-3">
            <select v-model="pick" class="input max-w-xs" data-testid="member-pick">
              <option value="">{{ t('groups.addMember') }}…</option>
              <option v-for="u in candidates" :key="u.id" :value="u.id">{{ u.username }}</option>
            </select>
            <button type="button" class="btn" :disabled="!pick" @click="add">
              + {{ t('groups.addMember') }}
            </button>
          </div>
          <ul class="divide-y divide-line">
            <li v-for="m in members" :key="m" class="flex items-center justify-between px-4 py-2">
              <span
                >{{ byId.get(m)?.username ?? m }}
                <span class="text-sm text-muted">{{ byId.get(m)?.fullName }}</span></span
              >
              <button
                type="button"
                class="btn btn-sm"
                :aria-label="t('common.delete')"
                @click="members = members.filter((x) => x !== m)"
              >
                <X class="h-4 w-4" />
              </button>
            </li>
          </ul>
        </Panel>
      </PageTabs>
      <ErrorBox :error="error" class="shrink-0" />
      <div class="flex shrink-0 items-center justify-end gap-2">
        <p v-if="notice" class="mr-auto text-sm text-success" role="status">{{ notice }}</p>
        <RouterLink to="/groups" class="btn">{{ t('common.back') }}</RouterLink>
        <button class="btn btn-primary" type="submit">{{ t('common.save') }}</button>
      </div>
    </form>
    <ConfirmDialog
      :open="confirmDelete"
      danger
      :title="t('common.delete')"
      :message="t('groups.deleteConfirm', { name })"
      :type-to-confirm="name"
      @confirm="remove"
      @cancel="confirmDelete = false"
    />
  </div>
</template>
