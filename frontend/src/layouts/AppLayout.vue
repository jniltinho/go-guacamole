<script setup lang="ts">
import { ref, watch } from 'vue'

import AppSidebar from '../components/AppSidebar.vue'
import AppTopbar from '../components/AppTopbar.vue'
import { useAuth } from '../stores/auth'

const auth = useAuth()
const collapsed = ref(auth.me?.preferences.sidebarCollapsed ?? false)
const mobileOpen = ref(false)

watch(collapsed, (v) => auth.savePreferences({ sidebarCollapsed: v }).catch(() => {}))
</script>

<template>
  <div class="app-h flex overflow-hidden">
    <AppSidebar v-model:collapsed="collapsed" :mobile-open="mobileOpen" @navigate="mobileOpen = false" />
    <div
      v-if="mobileOpen"
      class="fixed inset-0 z-30 bg-black/40 lg:hidden"
      aria-hidden="true"
      @click="mobileOpen = false"
    />
    <div class="flex min-w-0 flex-1 flex-col">
      <AppTopbar @toggle-menu="mobileOpen = !mobileOpen" />
      <main class="min-h-0 flex-1 overflow-y-auto p-4 md:p-6">
        <slot />
      </main>
    </div>
  </div>
</template>
