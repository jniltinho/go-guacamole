<script setup lang="ts">
import { ChevronDown, ChevronRight, Folder, FolderOpen } from '@lucide/vue'
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'

import type { Connection, ConnectionGroup } from '../api/types'
import ConnIcon from './ConnIcon.vue'
import ProtocolBadge from './ProtocolBadge.vue'
import StatusDot from './StatusDot.vue'

const props = defineProps<{ node: ConnectionGroup; root?: boolean; depth?: number }>()
const emit = defineEmits<{ connect: [c: Connection] }>()
const { t } = useI18n()
const open = ref(props.root || (props.depth ?? 0) < 1)
</script>

<template>
  <div>
    <button
      v-if="!root"
      class="flex w-full items-center gap-2 px-2 py-1.5 text-left font-semibold hover:bg-surface-2"
      :style="{ paddingLeft: `${(depth ?? 0) * 1.25 + 0.5}rem` }"
      :aria-expanded="open"
      @click="open = !open"
    >
      <component :is="open ? ChevronDown : ChevronRight" class="h-4 w-4 text-muted" />
      <component :is="open ? FolderOpen : Folder" class="h-4 w-4 text-accent" />
      {{ node.name }}
      <span class="text-xs font-normal text-muted">{{
        t('connectionGroups.connections', { n: node.connections?.length ?? 0 })
      }}</span>
    </button>
    <div v-if="open">
      <TreeNode
        v-for="g in node.groups"
        :key="g.id"
        :node="g"
        :depth="root ? 0 : (depth ?? 0) + 1"
        @connect="(c) => emit('connect', c)"
      />
      <div
        v-for="c in node.connections"
        :key="c.id"
        class="flex items-center gap-3 px-2 py-1.5 hover:bg-surface-2"
        :style="{ paddingLeft: `${(root ? 0 : (depth ?? 0) + 1) * 1.25 + 1.75}rem` }"
      >
        <ConnIcon :protocol="c.protocol" class="h-4 w-4 text-muted" />
        <span class="conn-name font-medium">{{ c.name }}</span>
        <span class="host text-xs text-muted">{{ c.hostname }}</span>
        <ProtocolBadge :protocol="c.protocol" />
        <StatusDot :status="c.status" class="ml-auto" />
        <button class="btn btn-primary btn-sm" @click="emit('connect', c)">
          {{ t('connections.connect') }}
        </button>
      </div>
    </div>
  </div>
</template>
