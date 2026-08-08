<template>
  <StandardDialog
    :model-value="modelValue"
    title="Edit runner"
    :fullscreen="mobile"
    max-width="560"
    :persistent="!!submittingAction"
    :close-disabled="!!submittingAction"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <v-alert
      v-if="error"
      class="mb-4"
      type="error"
      variant="tonal"
      density="comfortable"
    >
      {{ error }}
    </v-alert>
    <p class="text-body-medium brand-text-muted mb-4">
      Name and GitHub URL cannot be changed — create a new runner instead. Apply recreates the
      container. If the agent workdir does not match the host bind, apply clears
      <code>.runner</code> and reconfigures (token or PAT required).
    </p>
    <v-form ref="form" @submit.prevent="onFormSubmit">
      <p class="text-body-medium mb-4">
        <strong>{{ runner?.name }}</strong>
        <span class="brand-text-muted"> — {{ runner?.url }}</span>
      </p>
      <RunnerConfigFields
        v-model:labels="labelChips"
        v-model:image="image"
        v-model:cpu-limit="cpuLimit"
        v-model:memory-limit-mb="memoryLimitMb"
        v-model:network-mode="networkMode"
        v-model:extra-env-text="extraEnvText"
        v-model:mount-docker-sock="mountDockerSock"
        v-model:cache-enabled="cacheEnabled"
        v-model:cache-type="cacheType"
        v-model:cache-volume-name="cacheVolumeName"
        v-model:cache-host-path="cacheHostPath"
        v-model:cache-target="cacheTarget"
        v-model:cache-read-only="cacheReadOnly"
        v-model:workdir-host-path="workdirHostPath"
        :runner-name="runner?.name || ''"
        :disabled="!!submittingAction"
      />
      <v-switch
        v-model="apply"
        class="mb-2"
        color="primary"
        density="comfortable"
        hide-details
        label="Apply now (recreate container)"
        :disabled="!!submittingAction"
      />
      <v-text-field
        v-if="apply && !patConfigured"
        v-model="token"
        class="mb-2"
        label="Registration token (if volume missing or workdir reconfigure needed)"
        type="password"
        autocomplete="off"
        :disabled="!!submittingAction"
      />
    </v-form>

    <template #actions>
      <v-btn color="primary" variant="tonal" :disabled="!!submittingAction" @click="close">
        Cancel
      </v-btn>
      <v-btn
        color="primary"
        variant="tonal"
        :loading="submittingAction === 'save'"
        :disabled="apply || (!!submittingAction && submittingAction !== 'save')"
        @click="submit(false)"
      >
        Save
      </v-btn>
      <v-btn
        color="primary"
        variant="elevated"
        :loading="submittingAction === 'apply'"
        :disabled="!!submittingAction && submittingAction !== 'apply'"
        @click="submit(true)"
      >
        Save &amp; apply
      </v-btn>
    </template>
  </StandardDialog>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useStore } from 'vuex'
import StandardDialog from '@/components/common/StandardDialog.vue'
import RunnerConfigFields from '@/components/RunnerConfigFields.vue'
import { useMobile } from '@/composables/useMobile'
import { buildRuntimePayload, cacheFromRunner, formatExtraEnv } from '@/utils/runnerConfig'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  runner: { type: Object, default: null },
})
const emit = defineEmits(['update:modelValue'])

const store = useStore()
const mobile = useMobile()
const form = ref(null)

const labelChips = ref([])
const image = ref('')
const cpuLimit = ref(0)
const memoryLimitMb = ref(0)
const networkMode = ref('')
const extraEnvText = ref('')
const mountDockerSock = ref(null)
const cacheEnabled = ref(false)
const cacheType = ref('volume')
const cacheVolumeName = ref('')
const cacheHostPath = ref('')
const cacheTarget = ref('/cache')
const cacheReadOnly = ref(false)
const workdirHostPath = ref('')
const apply = ref(false)
const token = ref('')
const submittingAction = ref(null)
const error = ref('')

const open = computed(() => props.modelValue)
const patConfigured = computed(() => store.state.githubPatConfigured)

watch(open, (v) => {
  if (!v || !props.runner) return
  labelChips.value = [...(props.runner.labels || [])]
  image.value = props.runner.image || ''
  cpuLimit.value = props.runner.cpu_limit || 0
  memoryLimitMb.value = props.runner.memory_limit_mb || 0
  networkMode.value = props.runner.network_mode || ''
  extraEnvText.value = formatExtraEnv(props.runner.extra_env)
  mountDockerSock.value =
    props.runner.mount_docker_sock === true || props.runner.mount_docker_sock === false
      ? props.runner.mount_docker_sock
      : null
  const cache = cacheFromRunner(props.runner)
  cacheEnabled.value = cache.enabled
  cacheType.value = cache.type
  cacheVolumeName.value = cache.volumeName
  cacheHostPath.value = cache.hostPath
  cacheTarget.value = cache.target
  cacheReadOnly.value = cache.readOnly
  workdirHostPath.value = props.runner.workdir_host_path || ''
  apply.value = false
  token.value = ''
  error.value = ''
})

function close() {
  if (!submittingAction.value) emit('update:modelValue', false)
}

function onFormSubmit() {
  // Apply toggle disables Save; Enter should match the available primary action.
  submit(!!apply.value)
}

async function submit(forceApply) {
  error.value = ''
  if (!props.runner?.id || submittingAction.value) return
  // Save is disabled when apply toggle is on — do not apply via the Save path.
  if (!forceApply && apply.value) return
  const shouldApply = !!forceApply
  submittingAction.value = shouldApply ? 'apply' : 'save'
  try {
    const runtime = buildRuntimePayload({
      labels: labelChips.value,
      image: image.value,
      cpuLimit: cpuLimit.value,
      memoryLimitMb: memoryLimitMb.value,
      networkMode: networkMode.value,
      extraEnvText: extraEnvText.value,
      mountDockerSock: mountDockerSock.value,
      cacheEnabled: cacheEnabled.value,
      cacheType: cacheType.value,
      cacheVolumeName: cacheVolumeName.value,
      cacheHostPath: cacheHostPath.value,
      cacheTarget: cacheTarget.value,
      cacheReadOnly: cacheReadOnly.value,
      workdirHostPath: workdirHostPath.value,
    })
    const payload = {
      labels: runtime.labels,
      image: runtime.image,
      cpu_limit: runtime.cpu_limit,
      memory_limit_mb: runtime.memory_limit_mb,
      network_mode: runtime.network_mode,
      extra_env: runtime.extra_env,
      cache: runtime.cache,
      workdir_host_path: runtime.workdir_host_path,
      apply: shouldApply,
    }
    if (runtime.mount_docker_sock === true || runtime.mount_docker_sock === false) {
      payload.mount_docker_sock = runtime.mount_docker_sock
    } else {
      payload.reset_mount_docker_sock = true
    }
    if (shouldApply && token.value.trim()) payload.token = token.value.trim()
    await store.dispatch('patchRunner', { id: props.runner.id, payload })
    emit('update:modelValue', false)
  } catch (e) {
    error.value = e.message || String(e)
  } finally {
    submittingAction.value = null
  }
}
</script>
