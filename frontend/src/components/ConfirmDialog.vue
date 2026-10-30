<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import Modal from './Modal.vue'

// Confirmation in the theme's dialog (Modal.vue); typeToConfirm asks to type a name before a destructive action.
const props = defineProps<{
  open: boolean
  title: string
  message: string
  typeToConfirm?: string
  danger?: boolean
}>()
const emit = defineEmits<{ confirm: []; cancel: [] }>()
const { t } = useI18n()
const typed = ref('')
const ok = computed(() => !props.typeToConfirm || typed.value === props.typeToConfirm)
watch(
  () => props.open,
  () => (typed.value = ''),
)
</script>

<template>
  <Modal
    v-if="open"
    :title="title"
    :ok="t('common.confirm')"
    :danger="danger"
    :disabled="!ok"
    @close="emit('cancel')"
    @submit="emit('confirm')"
  >
    <p class="text-sm whitespace-pre-line">{{ message }}</p>
    <div v-if="typeToConfirm" class="mt-3">
      <label class="label" for="confirm-typed">{{
        t('common.typeToConfirm', { name: typeToConfirm })
      }}</label>
      <input id="confirm-typed" v-model="typed" class="input" autofocus data-testid="confirm-input" />
    </div>
  </Modal>
</template>
