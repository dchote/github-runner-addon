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
      Mount a durable cache (default /cache). Prefer a Docker volume on HAOS when workflows do not
      need sibling Docker/Buildx host binds of that path; use a host path for dedicated disks and
      sibling access. Same volume name or host path on multiple runners shares the cache.
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
        @update:model-value="onCacheType"
      />
      <v-text-field
        v-if="cacheType === 'volume'"
        :model-value="cacheVolumeName"
        class="mb-4"
        label="Cache volume name (optional)"
        hint="Empty uses gha-runner-<name>-cache; reuse the same name to share. Named volumes are not visible to sibling host binds of the mount path."
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
        hint="Docker host path. For sibling Docker/Buildx that use the mount path on the host, set this equal to Cache mount path (commonly both /cache)."
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
        hint="Container path (default /cache). Prefer matching Host path for sibling Docker."
        persistent-hint
        variant="outlined"
        density="comfortable"
        hide-details="auto"
        autocomplete="off"
        style="max-width: 20rem"
        :disabled="disabled"
        @update:model-value="$emit('update:cacheTarget', $event)"
      />
      <v-alert
        v-if="cachePathWarning"
        class="mb-4"
        type="warning"
        variant="tonal"
        density="comfortable"
      >
        {{ cachePathWarning }}
      </v-alert>
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

    <v-text-field
      :model-value="workdirHostPath"
      class="mb-2"
      label="Workdir host path (optional)"
      :hint="workdirHint"
      persistent-hint
      variant="outlined"
      density="comfortable"
      hide-details="auto"
      autocomplete="off"
      :disabled="disabled"
      @update:model-value="$emit('update:workdirHostPath', $event)"
    />
    <p class="text-body-small brand-text-muted mb-0">
      Sibling Docker jobs need a real host directory same-path bind (not a Docker volume
      <code>_data</code> path). Ensure the path is writable by the runner user (often
      <code>chown -R 1000:1000</code>). Changing this and applying reconfigures the agent
      (<code>workFolder</code>) — token or PAT required.
    </p>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { cacheSiblingPathWarning, defaultWorkdirHostPath } from '@/utils/runnerConfig'

const sockItems = [
  { title: 'Use global default', value: null },
  { title: 'Yes (mount host socket)', value: true },
  { title: 'No', value: false },
]

const cacheTypeItems = [
  { title: 'Docker volume', value: 'volume' },
  { title: 'Host path (bind)', value: 'bind' },
]

const props = defineProps({
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
  workdirHostPath: { type: String, default: '' },
  runnerName: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
})

const emit = defineEmits([
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
  'update:workdirHostPath',
])

const workdirHint = computed(() => {
  const n = String(props.runnerName || '').trim()
  const example = n ? defaultWorkdirHostPath(n) : '/srv/gha-work/<normalized-name>'
  return `Empty uses ${example} on the Docker host`
})

const cachePathWarning = computed(() =>
  cacheSiblingPathWarning({
    enabled: props.cacheEnabled,
    type: props.cacheType,
    hostPath: props.cacheHostPath,
    target: props.cacheTarget,
  }),
)

function onCacheType(next) {
  emit('update:cacheType', next)
  if (next === 'bind' && !String(props.cacheHostPath || '').trim()) {
    emit('update:cacheHostPath', String(props.cacheTarget || '').trim() || '/cache')
  }
}
</script>
