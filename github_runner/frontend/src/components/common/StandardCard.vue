<template>
  <v-card :class="['standard-page-card', cardClass]" :elevation="elevation" :loading="loading">
    <div v-if="showHeader" :class="['standard-card-header', headerClass]">
      <div
        class="standard-card-header__primary d-flex align-center min-width-0"
        :class="{ 'flex-grow-1': hasTitleAppend }"
      >
        <slot name="header">
          <span :class="['standard-card-title', titleClass]">{{ title }}</span>
        </slot>
      </div>
      <div v-if="hasTitleAppend" class="standard-card-header__append d-flex align-center flex-shrink-0">
        <slot name="titleAppend" />
      </div>
    </div>
    <v-divider v-if="showHeader" />

    <!-- Chrome order: header → tabs → toolbar → alerts → body → actions -->
    <div v-if="hasTabs" class="standard-card-tabs">
      <slot name="tabs" />
    </div>
    <div v-if="hasToolbar()" class="standard-card-toolbar">
      <slot name="toolbar" />
    </div>
    <div v-if="hasAlerts()" class="standard-card-alerts page-alert-stack">
      <slot name="alerts" />
    </div>

    <v-card-text
      :class="[
        contentClass ? '' : 'standard-card-body',
        hasToolbar() ? 'standard-card-content-with-toolbar' : '',
        hasAlerts() ? 'standard-card-content-with-alerts' : '',
        contentClass,
      ]"
    >
      <slot />
    </v-card-text>

    <template v-if="hasActions">
      <v-divider />
      <div class="standard-card-actions pa-4 d-flex justify-end align-center flex-wrap" style="gap: 8px">
        <slot name="actions" />
      </div>
    </template>
  </v-card>
</template>

<script setup>
import { useSlots, computed } from 'vue'
import { isRenderableSlot } from '@/utils/slotContentRenderable'

const props = defineProps({
  title: { type: String, default: '' },
  cardClass: { type: String, default: '' },
  contentClass: { type: String, default: '' },
  titleClass: { type: String, default: 'text-title-medium font-weight-bold' },
  headerClass: { type: String, default: '' },
  elevation: { type: [Number, String], default: undefined },
  /** Vuetify card loading overlay (initial data fetch) */
  loading: { type: [Boolean, String], default: false },
})

const slots = useSlots()
const hasTitleAppend = computed(() => !!slots.titleAppend)
const showHeader = computed(() => !!slots.header || !!props.title || hasTitleAppend.value)
const hasTabs = computed(() => !!slots.tabs)
// Evaluate slot content each render (not cached on slot fn identity) so parent
// `v-if` on `#toolbar` / `#alerts` correctly hides chrome.
function hasToolbar() {
  return isRenderableSlot(slots.toolbar)
}
function hasAlerts() {
  return isRenderableSlot(slots.alerts)
}
const hasActions = computed(() => isRenderableSlot(slots.actions))
</script>
