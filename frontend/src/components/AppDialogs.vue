<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { answerDialog, cancelDialogs, dialogState } from '../lib/dialogs'
import Modal from './Modal.vue'

// Renders the dialog opened through lib/dialogs (askConfirm, showAlert, askText, confirmTyped).
const { t } = useI18n()
const value = ref('')
const o = computed(() => dialogState.open)
watch(o, () => (value.value = ''))

const title = computed(() => {
  const d = o.value
  if (!d) return ''
  return d.opts.title || t(d.kind === 'alert' ? 'common.notice' : 'common.confirm')
})
const blocked = computed(() => {
  const d = o.value
  if (d?.kind === 'typed') return value.value !== d.expected
  if (d?.kind === 'text') return !value.value.trim()
  return false
})
function submit() {
  const d = o.value
  if (!d || blocked.value) return
  answerDialog(d.kind === 'text' ? value.value.trim() : d.kind === 'alert' ? undefined : true)
}
</script>

<template>
  <Modal
    v-if="o"
    :key="o.msg + o.kind"
    :title="title"
    :ok="o.kind === 'alert' ? null : o.opts.ok || t('common.confirm')"
    :danger="o.opts.danger || o.kind === 'typed'"
    :disabled="blocked"
    testid="app-dialog"
    @close="cancelDialogs"
    @submit="submit"
  >
    <p class="text-sm whitespace-pre-line">{{ o.msg }}</p>
    <div v-if="o.kind === 'text'" class="mt-3">
      <label v-if="o.opts.label" class="label" for="dlg-text">{{ o.opts.label }}</label>
      <input
        id="dlg-text"
        v-model="value"
        class="input"
        :maxlength="o.opts.maxlength"
        autofocus
        data-testid="dialog-input"
      />
    </div>
    <div v-else-if="o.kind === 'typed'" class="mt-3">
      <label class="label" for="dlg-typed">{{ t('common.typeToConfirm', { name: o.expected }) }}</label>
      <input id="dlg-typed" v-model="value" class="input" autofocus data-testid="confirm-input" />
    </div>
  </Modal>
</template>
