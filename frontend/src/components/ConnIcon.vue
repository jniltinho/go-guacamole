<script setup lang="ts">
import { AppWindow, Monitor, SquareTerminal } from '@lucide/vue'

defineProps<{ protocol: string }>()
</script>

<template>
  <SquareTerminal v-if="protocol === 'ssh'" />
  <AppWindow v-else-if="protocol === 'rdp'" />
  <Monitor v-else />
</template>
