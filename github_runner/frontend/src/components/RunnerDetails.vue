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
    <p v-if="runner.workdir_mismatch" class="text-body-small text-error mb-1">
      Agent workFolder does not match the host bind — Save &amp; apply (reconfigure) required.
    </p>
    <v-alert
      v-for="(w, i) in warnings"
      :key="`warn-${i}`"
      class="mb-2"
      type="warning"
      variant="tonal"
      density="comfortable"
    >
      {{ w }}
    </v-alert>
    <p class="text-body-medium mb-1"><strong>Image:</strong> {{ runner.image }}</p>
    <p class="text-body-medium mb-1"><strong>Container status:</strong> {{ runner.status }}</p>
    <p v-if="runner.running" class="text-body-medium mb-1">
      <strong>Job status:</strong> {{ runner.job_state || 'unknown' }}
    </p>
    <template v-if="currentJob">
      <p class="text-body-medium font-weight-medium mb-1 mt-2">Current job</p>
      <p class="text-body-medium mb-1"><strong>Repository:</strong> {{ field(currentJob.repository) }}</p>
      <p class="text-body-medium mb-1"><strong>Workflow:</strong> {{ field(currentJob.workflow) }}</p>
      <p class="text-body-medium mb-1"><strong>Job:</strong> {{ field(currentJob.job) }}</p>
      <p class="text-body-medium mb-1">
        <strong>Run:</strong>
        {{ runLabel }}
      </p>
      <p class="text-body-medium mb-1"><strong>Event:</strong> {{ field(currentJob.event) }}</p>
      <p class="text-body-medium mb-1"><strong>Actor:</strong> {{ field(currentJob.actor) }}</p>
      <p class="text-body-medium mb-1"><strong>Ref:</strong> {{ field(currentJob.ref) }}</p>
      <p class="text-body-medium mb-1"><strong>SHA:</strong> {{ shaLabel }}</p>
      <p class="text-body-medium mb-1"><strong>Updated:</strong> {{ formatTime(currentJob.updated_at) }}</p>
    </template>
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
const warnings = computed(() =>
  Array.isArray(props.runner?.warnings) ? props.runner.warnings.filter(Boolean) : [],
)
const extraEnvKeys = computed(() => Object.keys(props.runner?.extra_env || {}))
const currentJob = computed(() => {
  if (props.runner?.job_state !== 'busy') return null
  return props.runner?.current_job || null
})
const runLabel = computed(() => {
  const j = currentJob.value
  if (!j) return '—'
  const num = j.run_number || '—'
  const id = j.run_id ? ` (#${j.run_id})` : ''
  const attempt = j.run_attempt && j.run_attempt !== '1' ? ` attempt ${j.run_attempt}` : ''
  return `${num}${id}${attempt}`
})
const shaLabel = computed(() => {
  const sha = currentJob.value?.sha
  if (!sha) return '—'
  return sha.length > 12 ? `${sha.slice(0, 12)}…` : sha
})
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
  const effective = props.runner?.workdir_effective || props.runner?.workdir_host_path
  const agent = props.runner?.workdir_agent
  const err = props.runner?.workdir_error
  if (err && !agent) {
    return `${effective || '—'} (agent: ${err})`
  }
  if (agent && effective) {
    return `${effective} (agent workFolder: ${agent})`
  }
  if (effective) {
    return `${effective} (agent workFolder: unknown)`
  }
  return '—'
})

function field(v) {
  return v || '—'
}

function formatTime(v) {
  if (!v) return '—'
  try {
    return new Date(v).toLocaleString()
  } catch {
    return String(v)
  }
}
</script>
