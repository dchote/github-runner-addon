<template>
  <div class="runner-details-header">
    <span
      class="text-title-medium font-weight-bold text-truncate"
      :class="{ 'standard-card-title': cardTitle }"
    >
      {{ name }}
    </span>
    <div class="runner-details-header__actions">
      <v-btn
        class="standard-card-header-action"
        icon="mdi-pencil"
        size="small"
        variant="text"
        :aria-label="busy ? 'Edit runner (busy)' : 'Edit runner'"
        :disabled="busy || !!loadingAction"
        @click="$emit('edit')"
      />
      <v-btn
        class="standard-card-header-action"
        icon="mdi-delete"
        size="small"
        variant="text"
        :aria-label="busy ? 'Delete runner (busy)' : 'Delete runner'"
        :loading="loadingAction === 'delete'"
        :disabled="busy || (!!loadingAction && loadingAction !== 'delete')"
        @click="$emit('delete')"
      />
    </div>
  </div>
</template>

<script setup>
defineProps({
  name: { type: String, required: true },
  loadingAction: { type: String, default: null },
  busy: { type: Boolean, default: false },
  cardTitle: { type: Boolean, default: false },
})
defineEmits(['edit', 'delete'])
</script>

<style scoped>
.runner-details-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
  width: 100%;
  flex: 1 1 auto;
}

.runner-details-header__actions {
  display: flex;
  align-items: center;
  gap: 2px;
  flex-shrink: 0;
  margin-left: auto;
}
</style>
