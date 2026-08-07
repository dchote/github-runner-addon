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
      class="mb-4"
      label="Mount Docker socket"
      :items="sockItems"
      variant="outlined"
      density="comfortable"
      hide-details="auto"
      style="max-width: 20rem"
      :disabled="disabled"
      @update:model-value="$emit('update:mountDockerSock', $event)"
    />

    <v-switch
      :model-value="cacheEnabled"
      class="mb-2"
      color="primary"
      density="comfortable"
      hide-details
      label="Persistent cache"
      :disabled="disabled"
      @update:model-value="$emit('update:cacheEnabled', $event)"
    />
    <p class="text-body-small brand-text-muted mb-3">
      Mount a durable cache (default /cache). Prefer a Docker volume on HAOS; use a host path for
      dedicated disks. Same volume name or host path on multiple runners shares the cache.
    </p>
    <template v-if="cacheEnabled">
      <v-select
        :model-value="cacheType"
        class="mb-4"
        label="Cache storage"
        :items="cacheTypeItems"
        variant="outlined"
        density="comfortable"
        hide-details="auto"
        style="max-width: 20rem"
        :disabled="disabled"
        @update:model-value="$emit('update:cacheType', $event)"
      />
      <v-text-field
        v-if="cacheType === 'volume'"
        :model-value="cacheVolumeName"
        class="mb-4"
        label="Cache volume name (optional)"
        hint="Empty uses gha-runner-<name>-cache; reuse the same name to share"
        persistent-hint
        variant="outlined"
        density="comfortable"
        hide-details="auto"
        autocomplete="off"
        :disabled="disabled"
        @update:model-value="$emit('update:cacheVolumeName', $event)"
      />
      <v-text-field
        v-else
        :model-value="cacheHostPath"
        class="mb-4"
        label="Host path"
        hint="Absolute path on the Docker host (not paths inside the addon)"
        persistent-hint
        variant="outlined"
        density="comfortable"
        hide-details="auto"
        autocomplete="off"
        :disabled="disabled"
        @update:model-value="$emit('update:cacheHostPath', $event)"
      />
      <v-text-field
        :model-value="cacheTarget"
        class="mb-4"
        label="Cache mount path"
        hint="Container path (default /cache)"
        persistent-hint
        variant="outlined"
        density="comfortable"
        hide-details="auto"
        autocomplete="off"
        style="max-width: 20rem"
        :disabled="disabled"
        @update:model-value="$emit('update:cacheTarget', $event)"
      />
      <v-switch
        :model-value="cacheReadOnly"
        class="mb-4"
        color="primary"
        density="comfortable"
        hide-details
        label="Read-only cache"
        :disabled="disabled"
        @update:model-value="$emit('update:cacheReadOnly', $event)"
      />
      <p class="text-body-small brand-text-muted mb-3">
        Mount the cache read-only inside the runner (workflows cannot write to it).
      </p>
    </template>

    <p class="text-body-small brand-text-muted mb-0">
      Job workdir is managed automatically: a per-runner Docker volume is created and
      same-path bind-mounted so sibling
      <code>docker run -v $GITHUB_WORKSPACE</code>
      works when the Docker socket is mounted. Recreate keeps registration and this workdir —
      no manual host path or re-registration required.
    </p>
  </div>
</template>

<script setup>
const sockItems = [
  { title: 'Use global default', value: null },
  { title: 'Yes (mount host socket)', value: true },
  { title: 'No', value: false },
]

const cacheTypeItems = [
  { title: 'Docker volume', value: 'volume' },
  { title: 'Host path (bind)', value: 'bind' },
]

defineProps({
  labels: { type: Array, default: () => [] },
  image: { type: String, default: '' },
  cpuLimit: { type: Number, default: 0 },
  memoryLimitMb: { type: Number, default: 0 },
  networkMode: { type: String, default: '' },
  extraEnvText: { type: String, default: '' },
  mountDockerSock: { type: [Boolean, null], default: null },
  cacheEnabled: { type: Boolean, default: false },
  cacheType: { type: String, default: 'volume' },
  cacheVolumeName: { type: String, default: '' },
  cacheHostPath: { type: String, default: '' },
  cacheTarget: { type: String, default: '/cache' },
  cacheReadOnly: { type: Boolean, default: false },
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
  'update:cacheEnabled',
  'update:cacheType',
  'update:cacheVolumeName',
  'update:cacheHostPath',
  'update:cacheTarget',
  'update:cacheReadOnly',
])
</script>
