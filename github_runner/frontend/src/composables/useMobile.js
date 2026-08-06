import { computed } from 'vue'
import { useDisplay } from 'vuetify'

/** Mobile breakpoint helper aligned with UI form/dialog rules. */
export function useMobile() {
  const { mobile } = useDisplay()
  return computed(() => mobile.value)
}
