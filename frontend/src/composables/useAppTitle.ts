import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

// The application name an administrator set (Settings → Application); empty means the default name.
const custom = ref('')

export function setAppTitle(title?: string) {
  custom.value = (title ?? '').trim()
}

export function useAppTitle() {
  const { t } = useI18n()
  return {
    custom,
    title: computed(() => custom.value || t('app.name')),
  }
}
