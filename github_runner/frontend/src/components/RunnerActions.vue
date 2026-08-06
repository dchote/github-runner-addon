<template>
  <div class="runner-actions d-flex flex-nowrap align-center" style="gap: 6px">
    <v-btn
      v-if="canStart"
      color="primary"
      variant="elevated"
      size="small"
      prepend-icon="mdi-play"
      :loading="loading"
      @click="$emit('start')"
    >
      Start
    </v-btn>
    <v-btn
      v-if="runner.running"
      color="primary"
      variant="elevated"
      size="small"
      prepend-icon="mdi-stop"
      :loading="loading"
      @click="$emit('stop')"
    >
      Stop
    </v-btn>
    <v-btn
      v-if="canRestart"
      color="primary"
      variant="tonal"
      size="small"
      prepend-icon="mdi-restart"
      :loading="loading"
      @click="$emit('restart')"
    >
      Restart
    </v-btn>
    <v-btn
      color="primary"
      variant="tonal"
      size="small"
      prepend-icon="mdi-reload"
      :loading="loading"
      @click="$emit('recreate')"
    >
      Recreate
    </v-btn>
    <v-btn
      color="primary"
      :variant="primaryLifecycle ? 'tonal' : 'elevated'"
      size="small"
      prepend-icon="mdi-console"
      @click="$emit('logs')"
    >
      Logs
    </v-btn>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  runner: { type: Object, required: true },
  loading: { type: Boolean, default: false },
})

defineEmits(['start', 'stop', 'restart', 'recreate', 'logs'])

const canStart = computed(
  () => props.runner && !props.runner.running && props.runner.status !== 'missing',
)

const canRestart = computed(() => props.runner && props.runner.status !== 'missing')

/** True when Start or Stop is shown as the elevated primary lifecycle action. */
const primaryLifecycle = computed(() => canStart.value || !!props.runner?.running)
</script>

<style scoped>
.runner-actions {
  width: 100%;
  min-width: 0;
}

.runner-actions :deep(.v-btn) {
  flex: 0 1 auto;
  min-width: 0;
}
</style>
