<script setup lang="ts">
import { ArrowUp, File, Folder, RotateCw, Upload } from '@lucide/vue'
import type Guacamole from 'guacamole-common-js'
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { listDir, type FileEntry } from '../lib/transfer'

// One filesystem exposed by guacd (SFTP or the RDP drive), browsable from the
// session menu: open folders, download files, upload into the current folder.
const props = defineProps<{ object: Guacamole.Object; name: string }>()
const emit = defineEmits<{
  download: [path: string]
  upload: [files: File[], dir: string, done: () => void]
}>()
const { t } = useI18n()

const path = ref('/')
const entries = ref<FileEntry[]>([])
const loading = ref(false)
const error = ref('')
const picker = ref<HTMLInputElement>()

async function load() {
  loading.value = true
  error.value = ''
  try {
    entries.value = await listDir(props.object, path.value)
  } catch (e) {
    error.value = (e as Error).message
    entries.value = []
  } finally {
    loading.value = false
  }
}

function up() {
  const i = path.value.replace(/\/+$/, '').lastIndexOf('/')
  path.value = i <= 0 ? '/' : path.value.slice(0, i)
}

function open(e: FileEntry) {
  if (e.dir) path.value = e.path
  else emit('download', e.path)
}

function picked() {
  const files = picker.value?.files
  if (files?.length) emit('upload', Array.from(files), path.value, load)
  if (picker.value) picker.value.value = ''
}

watch(path, load)
onMounted(load)
</script>

<template>
  <div class="border border-white/20" data-testid="file-browser">
    <div class="flex items-center gap-1 border-b border-white/20 bg-white/5 px-2 py-1 text-xs">
      <span class="font-semibold">{{ name }}</span>
      <span class="min-w-0 flex-1 truncate font-mono text-white/60" :title="path">{{ path }}</span>
      <button class="p-1 hover:bg-white/10" :title="t('files.up')" :disabled="path === '/'" @click="up">
        <ArrowUp class="h-3.5 w-3.5" />
      </button>
      <button class="p-1 hover:bg-white/10" :title="t('files.refresh')" @click="load">
        <RotateCw class="h-3.5 w-3.5" :class="{ 'animate-spin': loading }" />
      </button>
      <button class="p-1 hover:bg-white/10" :title="t('files.upload')" @click="picker?.click()">
        <Upload class="h-3.5 w-3.5" />
      </button>
      <input ref="picker" type="file" multiple class="hidden" data-testid="file-upload" @change="picked" />
    </div>
    <p v-if="error" class="p-2 text-xs text-red-300">{{ error }}</p>
    <ul class="max-h-64 overflow-y-auto text-sm">
      <li v-for="e in entries" :key="e.path">
        <button
          class="flex w-full items-center gap-2 px-2 py-1 text-left hover:bg-white/10"
          :title="e.dir ? e.path : t('files.download')"
          @click="open(e)"
        >
          <component :is="e.dir ? Folder : File" class="h-3.5 w-3.5 shrink-0 text-white/60" />
          <span class="truncate">{{ e.name }}</span>
        </button>
      </li>
      <li v-if="!loading && !error && !entries.length" class="px-2 py-1 text-xs text-white/50">
        {{ t('files.empty') }}
      </li>
    </ul>
  </div>
</template>
