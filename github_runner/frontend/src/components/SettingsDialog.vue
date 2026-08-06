<template>
  <StandardDialog
    :model-value="modelValue"
    title="Settings"
    :fullscreen="mobile"
    max-width="480"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <p class="text-body-medium brand-text-muted mb-4">
      Read-only view of the effective configuration. Change values in Home Assistant addon options
      or process environment variables, then restart the app.
    </p>

    <section class="settings-section mb-4">
      <div class="settings-row mb-3">
        <div class="text-body-small brand-text-muted mb-1">Version</div>
        <div class="text-body-large">{{ version || '—' }}</div>
      </div>

      <v-divider class="mb-3" />

      <div class="settings-row mb-1">
        <div class="d-flex align-start justify-space-between" style="gap: 8px">
          <div class="settings-row__main">
            <div class="text-body-small brand-text-muted mb-1">Runner image</div>
            <div class="text-body-medium settings-row__mono">{{ runnerImage || '—' }}</div>
          </div>
          <v-btn
            v-if="runnerImage"
            icon="mdi-content-copy"
            size="small"
            variant="text"
            aria-label="Copy runner image"
            @click="copyImage"
          />
        </div>
      </div>
      <p class="text-body-small brand-text-muted mb-3">
        Default image is
        <a
          class="settings-link"
          href="https://github.com/myoung34/docker-github-actions-runner"
          target="_blank"
          rel="noopener"
        >
          myoung34/github-runner
        </a>
        by Matt Young. This app orchestrates those containers.
      </p>

      <v-divider class="mb-3" />

      <div class="settings-row mb-1">
        <div class="text-body-small brand-text-muted mb-2">GitHub PAT</div>
        <v-chip
          size="small"
          label
          :color="patConfigured ? 'success' : undefined"
          :variant="patConfigured ? 'tonal' : 'outlined'"
        >
          {{ patConfigured ? 'Configured' : 'Not configured' }}
        </v-chip>
      </div>
      <p class="text-body-small brand-text-muted mb-3">
        When set, registration tokens are minted automatically and runners can be deregistered from
        GitHub on delete.
      </p>

      <v-divider class="mb-3" />

      <div class="settings-row mb-0">
        <div class="text-body-small brand-text-muted mb-1">Docker socket in runners</div>
        <div class="text-body-large mb-1">
          {{ mountDockerSock ? 'Enabled' : 'Disabled' }}
        </div>
        <p class="text-body-small brand-text-muted mb-0">
          <template v-if="mountDockerSock">
            Host <code class="settings-code">/var/run/docker.sock</code> is shared with runner
            containers so workflows can use Docker. That is root-equivalent on the host — disable
            in addon options if your jobs do not need it.
          </template>
          <template v-else>
            Runner containers do not receive the host Docker socket.
          </template>
        </p>
      </div>
    </section>


    <v-alert
      type="info"
      variant="tonal"
      density="compact"
      border="start"
      icon="mdi-lan-connect"
      class="mb-0"
    >
      <p class="text-body-small mb-0">
        Access is network trust (Home Assistant ingress or a private bind). Do not expose port
        8099 on a public interface.
      </p>
    </v-alert>

    <template #actions>
      <v-btn color="primary" variant="tonal" @click="$emit('update:modelValue', false)">
        Close
      </v-btn>
    </template>
  </StandardDialog>
</template>

<script setup>
import { computed } from 'vue'
import { useStore } from 'vuex'
import StandardDialog from '@/components/common/StandardDialog.vue'
import { useMobile } from '@/composables/useMobile'

defineProps({
  modelValue: { type: Boolean, default: false },
})
defineEmits(['update:modelValue'])

const store = useStore()
const mobile = useMobile()
const version = computed(() => store.state.appVersion)
const runnerImage = computed(() => store.state.runnerImage)
const patConfigured = computed(() => store.state.githubPatConfigured)
const mountDockerSock = computed(() => store.state.mountDockerSock)

async function copyImage() {
  const value = runnerImage.value
  if (!value || !navigator.clipboard?.writeText) return
  try {
    await navigator.clipboard.writeText(value)
  } catch {
    /* best-effort */
  }
}
</script>

<style scoped>
.settings-section {
  margin: 0;
}

.settings-row__main {
  min-width: 0;
  flex: 1 1 auto;
}

.settings-row__mono {
  word-break: break-all;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.875rem;
  line-height: 1.4;
}

.settings-link {
  color: rgb(var(--v-theme-primary));
  text-decoration: none;
}

.settings-link:hover {
  text-decoration: underline;
}

.settings-code {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.85em;
}

:deep(.v-alert) {
  align-items: flex-start;
}

:deep(.v-alert__prepend) {
  margin-inline-end: 12px;
  align-self: flex-start;
}
</style>

