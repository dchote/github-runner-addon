<template>
  <div>
    <p class="text-body-medium mb-1"><strong>URL:</strong> {{ runner.url }}</p>
    <p class="text-body-medium mb-1"><strong>Scope:</strong> {{ runner.scope }}</p>
    <p class="text-body-medium mb-1"><strong>Container:</strong> {{ runner.container_name }}</p>
    <p class="text-body-medium mb-1"><strong>Volume:</strong> {{ runner.volume_name }}</p>
    <p class="text-body-medium mb-1">
      <strong>Cache:</strong> {{ cacheLabel }}
    </p>
    <p class="text-body-medium mb-1">
      <strong>Workdir:</strong> {{ workdirLabel }}
    </p>
    <p class="text-body-medium mb-1"><strong>Image:</strong> {{ runner.image }}</p>
    <p class="text-body-medium mb-1"><strong>Status:</strong> {{ runner.status }}</p>
    <p class="text-body-medium mb-1">
      <strong>Labels:</strong>
      <span v-if="labels.length">{{ labels.join(', ') }}</span>
      <span v-else class="brand-text-muted">—</span>
    </p>
    <p class="text-body-medium mb-1">
      <strong>CPU limit:</strong>
      {{ runner.cpu_limit ? `${runner.cpu_limit} cores` : 'unlimited' }}
    </p>
    <p class="text-body-medium mb-1">
      <strong>Memory limit:</strong>
      {{ runner.memory_limit_mb ? `${runner.memory_limit_mb} MiB` : 'unlimited' }}
    </p>
    <p class="text-body-medium mb-1">
      <strong>Network:</strong> {{ runner.network_mode || 'default' }}
    </p>
    <p class="text-body-medium mb-1">
      <strong>Docker socket:</strong> {{ sockLabel }}
    </p>
    <p v-if="extraEnvKeys.length" class="text-body-medium mb-1">
      <strong>Extra env:</strong> {{ extraEnvKeys.join(', ') }}
    </p>
    <p class="text-body-medium mb-1">
      <strong>Created:</strong> {{ formatTime(runner.created_at) }}
    </p>
    <p class="text-body-medium mb-0"><strong>ID:</strong> {{ runner.id }}</p>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  runner: { type: Object, required: true },
})

const labels = computed(() => props.runner?.labels || [])
const extraEnvKeys = computed(() => Object.keys(props.runner?.extra_env || {}))
const sockLabel = computed(() => {
  const v = props.runner?.mount_docker_sock
  if (v === true) return 'mounted (override)'
  if (v === false) return 'not mounted (override)'
  return 'global default'
})

const cacheLabel = computed(() => {
  const c = props.runner?.cache
  if (!c?.enabled) return 'none'
  const target = c.target || '/cache'
  const ro = c.read_only ? ' (read-only)' : ''
  if (c.type === 'bind') {
    return `bind ${c.host_path || '?'} → ${target}${ro}`
  }
  const vol = c.volume_name || `${props.runner.container_name}-cache`
  return `volume ${vol} → ${target}${ro}`
})

const workdirLabel = computed(() => {
  const resolved = props.runner?.workdir_resolved
  const err = props.runner?.workdir_error
  const vol = props.runner?.work_volume_name || `${props.runner?.container_name || ''}-work`
  if (resolved) {
    return `${vol} → ${resolved}`
  }
  if (err) {
    return `volume ${vol} (unresolved: ${err})`
  }
  return `volume ${vol}`
})

function formatTime(v) {
  if (!v) return '—'
  try {
    return new Date(v).toLocaleString()
  } catch {
    return String(v)
  }
}
</script>
