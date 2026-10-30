<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { api, ApiError } from '../api/client'
import type { ConnectionRecording, Tri } from '../api/types'
import ConfirmDialog from './ConfirmDialog.vue'
import ErrorBox from './ErrorBox.vue'
import Panel from './Panel.vue'
import RecordingPolicyForm from './RecordingPolicyForm.vue'

// Recording policy of one connection, saved through its dedicated endpoint
// (AUDIT/ADMINISTER); read-only for other editors.
const props = defineProps<{ connectionId: string; protocol: string; recording: ConnectionRecording }>()
const { t } = useI18n()

const cur = ref<ConnectionRecording>(props.recording)
const form = reactive({
  video: props.recording.video as Tri,
  text: props.recording.text as Tri,
  keys: props.recording.keys as Tri,
  videoDays: props.recording.retentionVideoDays,
  textDays: props.recording.retentionTextDays,
})
const error = ref<unknown>(null)
const notice = ref('')
const busy = ref(false)
const preview = ref<{ recordings: number; bytes: number; token: string } | null>(null)

// what "inherit" means for this connection (global defaults)
const inherited = computed(() => {
  const d = cur.value.defaults
  return {
    video: d.video,
    text: d.text,
    keys: d.keys,
    videoDays: d.retentionVideoDays,
    textDays: d.retentionTextDays,
  }
})

async function save(previewToken?: string) {
  busy.value = true
  error.value = null
  notice.value = ''
  try {
    cur.value = await api.put<ConnectionRecording>(`/connections/${props.connectionId}/recording`, {
      video: form.video,
      text: form.text,
      keys: form.keys,
      retentionVideoDays: form.videoDays,
      retentionTextDays: form.textDays,
      version: cur.value.version,
      previewToken,
    })
    notice.value = t('common.saved')
  } catch (e) {
    if (e instanceof ApiError && e.code === 'RETENTION_PREVIEW' && e.data) {
      preview.value = e.data as typeof preview.value
    } else error.value = e
  } finally {
    busy.value = false
  }
}

function confirmPreview() {
  const tok = preview.value?.token
  preview.value = null
  save(tok)
}
</script>

<template>
  <Panel :title="t('recordingPolicy.title')" data-testid="recording-panel">
    <form class="space-y-3 p-4" @submit.prevent="save()">
      <p class="text-xs text-muted">
        {{ recording.canEdit ? t('recordingPolicy.connectionHint') : t('recordingPolicy.readOnly') }}
      </p>
      <RecordingPolicyForm
        v-model:video="form.video"
        v-model:text="form.text"
        v-model:keys="form.keys"
        v-model:video-days="form.videoDays"
        v-model:text-days="form.textDays"
        tri
        :disabled="!recording.canEdit"
        :text-allowed="protocol === 'ssh'"
        :inherited="inherited"
      />
      <p class="text-xs text-muted">
        {{ t('recordingPolicy.effectiveNow') }}:
        <strong>{{
          (['video', 'text', 'keys'] as const)
            .filter((k) => cur.effective[k])
            .map((k) => t('recordingPolicy.kinds.' + k))
            .join(' · ') || t('recordingPolicy.nothing')
        }}</strong>
      </p>
      <ErrorBox :error="error" />
      <p v-if="error instanceof ApiError && error.field" class="text-sm text-danger">{{ error.message }}</p>
      <p v-if="notice" class="text-sm text-success" role="status">{{ notice }}</p>
      <div v-if="recording.canEdit" class="flex justify-end">
        <button class="btn btn-primary" type="submit" :disabled="busy" data-testid="save-recording">
          {{ t('common.save') }}
        </button>
      </div>
    </form>
    <ConfirmDialog
      :open="!!preview"
      danger
      :title="t('recordingPolicy.previewTitle')"
      :message="
        t('recordingPolicy.previewMessage', {
          n: preview?.recordings ?? 0,
          bytes: ((preview?.bytes ?? 0) / 1048576).toFixed(1),
        })
      "
      @confirm="confirmPreview"
      @cancel="preview = null"
    />
  </Panel>
</template>
