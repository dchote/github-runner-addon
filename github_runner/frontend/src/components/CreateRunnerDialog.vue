<template>
  <StandardDialog
    :model-value="modelValue"
    title="Create runner"
    :fullscreen="mobile"
    max-width="520"
    :persistent="submitting"
    :close-disabled="submitting"
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
      <template v-if="patConfigured">
        A GitHub PAT is configured — registration tokens are minted automatically. You can still
        paste a one-time token to override.
      </template>
      <template v-else>
        Get a registration token from GitHub → Settings → Actions → Runners → New self-hosted
        runner. Tokens expire in about one hour.
      </template>
    </p>
    <v-form ref="form" @submit.prevent="submit">
      <v-text-field
        v-model="name"
        class="mb-4"
        label="Runner name"
        variant="outlined"
        density="comfortable"
        hide-details="auto"
        autocomplete="off"
        :rules="[requiredRule]"
        :disabled="submitting"
      />
      <v-text-field
        v-model="url"
        class="mb-4"
        label="GitHub project URL"
        hint="https://github.com/owner/repo or https://github.com/org"
        persistent-hint
        variant="outlined"
        density="comfortable"
        hide-details="auto"
        autocomplete="off"
        :rules="[requiredRule]"
        :disabled="submitting"
      />
      <v-text-field
        v-model="token"
        class="mb-4"
        :label="patConfigured ? 'Registration token (optional)' : 'Registration token'"
        type="password"
        variant="outlined"
        density="comfortable"
        hide-details="auto"
        autocomplete="off"
        :rules="patConfigured ? [] : [requiredRule]"
        :disabled="submitting"
      />

      <v-expansion-panels v-model="advancedOpen" class="mb-0" variant="accordion">
        <v-expansion-panel value="advanced" elevation="0" rounded="lg">
          <v-expansion-panel-title class="text-body-medium font-weight-medium px-3">
            Advanced
          </v-expansion-panel-title>
          <v-expansion-panel-text>
            <p class="text-body-small brand-text-muted mb-4">
              Optional labels, image, resources, and environment. Defaults work for most setups.
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
              v-model:persist-workdir="persistWorkdir"
              v-model:workdir-host-path="workdirHostPath"
              :disabled="submitting"
            />
          </v-expansion-panel-text>
        </v-expansion-panel>
      </v-expansion-panels>
    </v-form>

    <template #actions>
      <v-btn color="primary" variant="tonal" :disabled="submitting" @click="close">
        Cancel
      </v-btn>
      <v-btn color="primary" variant="elevated" :loading="submitting" @click="submit">
        Create
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
import { buildRuntimePayload } from '@/utils/runnerConfig'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
})
const emit = defineEmits(['update:modelValue'])

const store = useStore()
const mobile = useMobile()
const form = ref(null)

const name = ref('')
const url = ref('')
const token = ref('')
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
const persistWorkdir = ref(false)
const workdirHostPath = ref('')
const advancedOpen = ref([])
const submitting = ref(false)
const error = ref('')

const open = computed(() => props.modelValue)
const patConfigured = computed(() => store.state.githubPatConfigured)
const requiredRule = (v) => !!String(v || '').trim() || 'Required'

watch(open, (v) => {
  if (v) {
    name.value = ''
    url.value = ''
    token.value = ''
    labelChips.value = []
    image.value = ''
    cpuLimit.value = 0
    memoryLimitMb.value = 0
    networkMode.value = ''
    extraEnvText.value = ''
    mountDockerSock.value = null
    cacheEnabled.value = false
    cacheType.value = 'volume'
    cacheVolumeName.value = ''
    cacheHostPath.value = ''
    cacheTarget.value = '/cache'
    cacheReadOnly.value = false
    persistWorkdir.value = false
    workdirHostPath.value = ''
    advancedOpen.value = []
    error.value = ''
  }
})

function close() {
  if (!submitting.value) emit('update:modelValue', false)
}

async function submit() {
  error.value = ''
  const { valid } = (await form.value?.validate?.()) || { valid: false }
  if (!valid) {
    error.value = patConfigured.value
      ? 'Name and URL are required'
      : 'Name, URL, and token are required'
    return
  }
  submitting.value = true
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
      persistWorkdir: persistWorkdir.value,
      workdirHostPath: workdirHostPath.value,
    })
    const payload = {
      name: name.value.trim(),
      url: url.value.trim(),
    }
    const tok = token.value.trim()
    if (tok) payload.token = tok
    if (runtime.labels.length) payload.labels = runtime.labels
    if (runtime.image) payload.image = runtime.image
    if (runtime.cpu_limit) payload.cpu_limit = runtime.cpu_limit
    if (runtime.memory_limit_mb) payload.memory_limit_mb = runtime.memory_limit_mb
    if (runtime.network_mode) payload.network_mode = runtime.network_mode
    if (Object.keys(runtime.extra_env || {}).length) payload.extra_env = runtime.extra_env
    if (runtime.mount_docker_sock === true || runtime.mount_docker_sock === false) {
      payload.mount_docker_sock = runtime.mount_docker_sock
    }
    payload.cache = runtime.cache
    payload.persist_workdir = runtime.persist_workdir
    if (runtime.workdir_host_path) payload.workdir_host_path = runtime.workdir_host_path
    await store.dispatch('createRunner', payload)
    emit('update:modelValue', false)
  } catch (e) {
    error.value = e.message || String(e)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
:deep(.v-expansion-panel-title) {
  min-height: 48px;
}

:deep(.v-expansion-panel-text__wrapper) {
  padding-inline: 4px 4px;
  padding-block: 0 8px;
}
</style>
