<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { api } from '../../api/client'
import ErrorBox from '../ErrorBox.vue'
import Modal from '../Modal.vue'

// New or renamed/moved connection folder (Connection groups). parents lists the folders it may go under (the
// caller leaves out the folder itself and its descendants).
const props = defineProps<{
  folder?: { id: string; name: string; parentId: string }
  parents: { id: string; label: string }[]
}>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()
const form = reactive({ name: props.folder?.name ?? '', parentId: props.folder?.parentId ?? '' })
const busy = ref(false)
const error = ref<unknown>(null)
const valid = computed(() => form.name.trim() !== '')

async function submit() {
  if (!valid.value) return
  busy.value = true
  error.value = null
  try {
    const body = { name: form.name.trim(), parentId: form.parentId || null }
    if (props.folder) await api.put(`/connection-groups/${props.folder.id}`, body)
    else await api.post('/connection-groups', body)
    emit('saved')
  } catch (e) {
    error.value = e
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Modal
    :title="folder ? t('connectionGroups.rename') : t('connectionGroups.new')"
    :ok="folder ? t('common.save') : t('common.create')"
    :busy="busy"
    :locked="busy"
    :disabled="!valid"
    testid="folder-form-dialog"
    @close="emit('close')"
    @submit="submit"
  >
    <div class="space-y-4">
      <div>
        <label class="label" for="folder-name">{{ t('common.name') }} *</label>
        <input
          id="folder-name"
          v-model="form.name"
          class="input"
          required
          maxlength="128"
          autofocus
          data-testid="folder-name"
        />
      </div>
      <div>
        <label class="label" for="folder-parent">{{ t('connectionGroups.parent') }}</label>
        <select id="folder-parent" v-model="form.parentId" class="input" data-testid="folder-parent">
          <option value="">{{ t('common.root') }}</option>
          <option v-for="p in parents" :key="p.id" :value="p.id">{{ p.label }}</option>
        </select>
      </div>
    </div>
    <ErrorBox :error="error" class="mt-3" />
  </Modal>
</template>
