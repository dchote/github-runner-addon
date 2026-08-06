<template>
  <div
    class="page-loader"
    :class="`page-loader--${variant}`"
    role="status"
    aria-live="polite"
    :aria-label="label || 'Loading'"
  >
    <v-progress-linear
      indeterminate
      color="primary"
      rounded
      height="4"
      class="page-loader__bar mb-3"
    />
    <p v-if="label" class="text-body-medium brand-text-muted mb-0">{{ label }}</p>
  </div>
</template>

<script setup>
defineProps({
  label: { type: String, default: 'Loading…' },
  /** page = centered full pane; compact = tight strip */
  variant: {
    type: String,
    default: 'page',
    validator: (v) => ['page', 'compact'].includes(v),
  },
})
</script>

<style scoped>
/* PageLoader spacing; bar width keeps the strip readable in wide cards. */
.page-loader__bar {
  width: min(320px, 100%);
  margin-inline: auto;
}

.page-loader--page {
  padding: 48px 16px;
  text-align: center;
  max-width: 420px;
  margin: 0 auto;
}

.page-loader--compact {
  padding: 16px 0;
  text-align: center;
}
</style>
