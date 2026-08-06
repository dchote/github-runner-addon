<template>
  <div class="empty-state">
    <v-icon v-if="icon" :icon="icon" class="empty-state__icon" />
    <h3 v-if="title" class="text-title-large empty-state__title">{{ title }}</h3>
    <p v-if="copy" class="text-body-medium empty-state__copy">{{ copy }}</p>
    <div v-if="$slots.actions" class="d-flex justify-center flex-wrap" style="gap: 8px">
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup>
defineProps({
  icon: { type: String, default: 'mdi-inbox-outline' },
  title: { type: String, default: 'Nothing here yet' },
  copy: { type: String, default: '' },
})
</script>
