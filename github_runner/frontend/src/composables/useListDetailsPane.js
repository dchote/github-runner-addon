import { computed, ref, watch } from 'vue'
import { useDisplay } from 'vuetify'

/**
 * Responsive list + details selection (desktop split / mobile dialog).
 */
export function useListDetailsPane() {
  const { mdAndUp } = useDisplay()
  const selectedId = ref(null)
  const detailsOpen = ref(false)
  const isDesktop = computed(() => mdAndUp.value)

  function select(id) {
    if (!id) {
      clear()
      return
    }
    if (selectedId.value === id) {
      clear()
      return
    }
    selectedId.value = id
    detailsOpen.value = !isDesktop.value
  }

  function clear() {
    selectedId.value = null
    detailsOpen.value = false
  }

  watch(isDesktop, (desktop) => {
    if (desktop) {
      detailsOpen.value = false
    } else if (selectedId.value) {
      detailsOpen.value = true
    }
  })

  return { selectedId, detailsOpen, isDesktop, select, clear }
}
