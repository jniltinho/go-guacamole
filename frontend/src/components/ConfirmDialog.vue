<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

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
const input = ref<HTMLInputElement>()
const ok = computed(() => !props.typeToConfirm || typed.value === props.typeToConfirm)

watch(
  () => props.open,
  async (o) => {
    typed.value = ''
    if (o) {
      await nextTick()
      input.value?.focus()
    }
  },
)
</script>

<template>
  <Teleport to="body">
    <Transition name="fade">
      <div
        v-if="open"
        class="fixed inset-0 z-[100] flex items-center justify-center bg-[var(--overlay)] p-4"
        role="dialog"
        aria-modal="true"
        @keydown.esc="emit('cancel')"
      >
        <div class="card w-full max-w-md shadow-elev-4">
          <div class="card-title px-4 py-3">
            <h2 class="section-title">{{ title }}</h2>
          </div>
          <div class="space-y-3 p-4">
            <p>{{ message }}</p>
            <div v-if="typeToConfirm">
              <label class="label">{{ t('common.typeToConfirm', { name: typeToConfirm }) }}</label>
              <input ref="input" v-model="typed" class="input" data-testid="confirm-input" />
            </div>
          </div>
          <div class="flex justify-end gap-2 border-t border-line p-3">
            <button class="btn" @click="emit('cancel')">{{ t('common.cancel') }}</button>
            <button
              class="btn"
              :class="danger ? 'btn-danger' : 'btn-primary'"
              :disabled="!ok"
              data-testid="confirm-ok"
              @click="emit('confirm')"
            >
              {{ t('common.confirm') }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
