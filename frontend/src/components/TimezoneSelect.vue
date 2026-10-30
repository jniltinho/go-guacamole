<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

// IANA time zone picker grouped by region (as the Java client's region + zone pair); empty = the server's zone.
// A stored zone the browser does not list (an import) stays selectable.
const model = defineModel<string>({ default: '' })
defineProps<{ id?: string }>()
const { t } = useI18n()
const ZONES = Intl.supportedValuesOf('timeZone')
const groups = computed(() => {
  const zones = model.value && !ZONES.includes(model.value) ? [model.value, ...ZONES] : ZONES
  const by = new Map<string, string[]>()
  for (const z of zones) {
    const region = z.includes('/') ? z.slice(0, z.indexOf('/')) : 'Other'
    by.set(region, [...(by.get(region) ?? []), z])
  }
  return [...by.entries()]
})
</script>

<template>
  <select :id="id" v-model="model" class="input">
    <option value="">{{ t('users.timezoneDefault') }}</option>
    <optgroup v-for="[region, zones] in groups" :key="region" :label="region">
      <option v-for="z in zones" :key="z" :value="z">
        {{ z.replaceAll('_', ' ') }}
      </option>
    </optgroup>
  </select>
</template>
