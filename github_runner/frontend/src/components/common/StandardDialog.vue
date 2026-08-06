<template>
  <v-dialog
    :fullscreen="fullscreen"
    :max-width="maxWidth"
    :model-value="modelValue"
    :persistent="persistent"
    :scrim="scrim"
    transition="dialog-transition"
    @update:model-value="handleUpdate"
  >
    <v-card
      class="standard-dialog-card"
      :class="{
        'dialog-card-layout': hasActions || fillBody,
        'dialog-card-layout--fullscreen': (hasActions || fillBody) && fullscreen,
        'standard-dialog-card--fill': fillBody,
        'standard-dialog-card--fill-fullscreen': fillBody && fullscreen,
      }"
    >
      <template v-if="showHeaderSection">
        <v-card-title class="standard-card-header d-flex align-center justify-space-between">
          <slot name="header">
            <span class="text-title-medium font-weight-bold">{{ title }}</span>
          </slot>
          <v-btn
            v-if="showClose"
            :disabled="closeDisabled"
            class="standard-card-header-action"
            icon="mdi-close"
            size="small"
            variant="text"
            aria-label="Close"
            @click="handleClose"
          />
        </v-card-title>
        <v-divider />
      </template>

      <v-card-text
        :class="[
          contentPadding,
          { 'dialog-card-content': hasActions || fillBody },
          { 'dialog-card-content--edge-scroll': scrollEdge },
          { 'standard-dialog-content--fill': fillBody },
        ]"
      >
        <slot />
      </v-card-text>

      <template v-if="hasActions">
        <v-divider />
        <v-card-actions class="dialog-card-actions pa-4 justify-end flex-wrap" style="gap: 8px">
          <slot name="actions" />
        </v-card-actions>
      </template>
    </v-card>
  </v-dialog>
</template>

<script setup>
import { computed, useSlots } from 'vue'
import { isRenderableSlot } from '@/utils/slotContentRenderable'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  title: { type: String, default: '' },
  maxWidth: { type: [String, Number], default: '600' },
  fullscreen: { type: Boolean, default: false },
  persistent: { type: Boolean, default: false },
  showClose: { type: Boolean, default: true },
  closeDisabled: { type: Boolean, default: false },
  showHeader: { type: Boolean, default: true },
  scrim: { type: [Boolean, String], default: true },
  contentPadding: { type: String, default: 'standard-card-body' },
  scrollEdge: { type: Boolean, default: false },
  /** Flex column layout so the card body delegates vertical scrolling to a child. */
  fillBody: { type: Boolean, default: false },
})

const emit = defineEmits(['update:modelValue', 'close'])
const slots = useSlots()
const hasActions = computed(() => isRenderableSlot(slots.actions))
const showHeaderSection = computed(() => props.showHeader || !!slots.header)

function handleUpdate(value) {
  emit('update:modelValue', value)
  if (!value) emit('close')
}

function handleClose() {
  emit('update:modelValue', false)
  emit('close')
}
</script>

<style scoped>
.standard-dialog-card--fill {
  display: flex;
  flex-direction: column;
  max-height: calc(100vh - 48px);
}

.standard-dialog-card--fill-fullscreen {
  max-height: none;
  height: 100%;
}

.standard-dialog-content--fill {
  flex: 1 1 auto;
  min-height: 0;
  max-height: none;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
</style>
