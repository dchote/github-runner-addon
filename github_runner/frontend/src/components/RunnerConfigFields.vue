<template>
  <div>
    <v-combobox
      :model-value="labels"
      class="mb-4"
      label="Labels"
      hint="Defaults to self-hosted, linux when empty"
      persistent-hint
      multiple
      chips
      closable-chips
      variant="outlined"
      density="comfortable"
      hide-details="auto"
      autocomplete="off"
      :disabled="disabled"
      @update:model-value="$emit('update:labels', $event)"
    />
    <v-text-field
      :model-value="image"
      class="mb-4"
      label="Image override (optional)"
      hint="Tag or digest; leave blank for the global runner image"
      persistent-hint
      variant="outlined"
      density="comfortable"
      hide-details="auto"
      autocomplete="off"
      :disabled="disabled"
      @update:model-value="$emit('update:image', $event)"
    />
    <div class="d-flex flex-column flex-sm-row mb-4" style="column-gap: 16px; row-gap: 16px">
      <v-text-field
        :model-value="cpuLimit"
        label="CPU limit"
        type="number"
        min="0"
        step="0.1"
        hint="Cores; 0 = unlimited"
        persistent-hint
        variant="outlined"
        density="comfortable"
        hide-details="auto"
        autocomplete="off"
        style="max-width: 12rem"
        :disabled="disabled"
        @update:model-value="$emit('update:cpuLimit', Number($event) || 0)"
      />
      <v-text-field
        :model-value="memoryLimitMb"
        label="Memory limit (MiB)"
        type="number"
        min="0"
        step="64"
        hint="0 = unlimited"
        persistent-hint
        variant="outlined"
        density="comfortable"
        hide-details="auto"
        autocomplete="off"
        style="max-width: 12rem"
        :disabled="disabled"
        @update:model-value="$emit('update:memoryLimitMb', Number($event) || 0)"
      />
    </div>
    <v-text-field
      :model-value="networkMode"
      class="mb-4"
      label="Network mode (optional)"
      hint="e.g. bridge, host — leave blank for Docker default"
      persistent-hint
      variant="outlined"
      density="comfortable"
      hide-details="auto"
      autocomplete="off"
      style="max-width: 20rem"
      :disabled="disabled"
      @update:model-value="$emit('update:networkMode', $event)"
    />
    <v-textarea
      :model-value="extraEnvText"
      class="mb-4"
      label="Extra environment (optional)"
      hint="One KEY=value per line; reserved runner keys are rejected"
      persistent-hint
      variant="outlined"
      density="comfortable"
      hide-details="auto"
      autocomplete="off"
      rows="3"
      :disabled="disabled"
      @update:model-value="$emit('update:extraEnvText', $event)"
    />
    <v-select
      :model-value="mountDockerSock"
      class="mb-0"
      label="Mount Docker socket"
      :items="sockItems"
      variant="outlined"
      density="comfortable"
      hide-details="auto"
      style="max-width: 20rem"
      :disabled="disabled"
      @update:model-value="$emit('update:mountDockerSock', $event)"
    />
  </div>
</template>

<script setup>
const sockItems = [
  { title: 'Use global default', value: null },
  { title: 'Yes (mount host socket)', value: true },
  { title: 'No', value: false },
]

defineProps({
  labels: { type: Array, default: () => [] },
  image: { type: String, default: '' },
  cpuLimit: { type: Number, default: 0 },
  memoryLimitMb: { type: Number, default: 0 },
  networkMode: { type: String, default: '' },
  extraEnvText: { type: String, default: '' },
  mountDockerSock: { type: [Boolean, null], default: null },
  disabled: { type: Boolean, default: false },
})

defineEmits([
  'update:labels',
  'update:image',
  'update:cpuLimit',
  'update:memoryLimitMb',
  'update:networkMode',
  'update:extraEnvText',
  'update:mountDockerSock',
])
</script>
