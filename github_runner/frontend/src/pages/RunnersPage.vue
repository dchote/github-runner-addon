<template>
  <div>
    <header class="page-content__intro">
      <h1 class="text-headline-small font-weight-bold mb-1">Runners</h1>
      <p class="text-body-medium brand-text-muted mb-0">
        Manage local GitHub Actions self-hosted runner containers for one or more projects.
      </p>
    </header>

    <PageLoader v-if="initialLoading" label="Loading runners…" />

    <template v-else>
      <div class="status-summary d-flex flex-wrap mb-4" style="gap: 8px">
        <v-chip
          v-for="item in summaryChips"
          :key="item.key"
          size="small"
          label
          variant="tonal"
          :color="item.color"
        >
          {{ item.label }} {{ item.value }}
        </v-chip>
      </div>

      <v-alert
        v-if="!storeReadable"
        class="mb-4"
        type="error"
        variant="tonal"
        density="comfortable"
      >
        Runner store is not readable{{ storeError ? `: ${storeError}` : '' }}.
      </v-alert>

      <v-alert
        v-if="orphans.length"
        class="mb-4"
        type="warning"
        variant="tonal"
        density="comfortable"
      >
        <div class="text-body-medium font-weight-medium mb-1">
          Orphan managed containers ({{ orphans.length }})
        </div>
        <p class="text-body-small mb-2">
          Docker containers labeled by this addon with no matching store record. Remove them
          manually if leftover from a failed delete.
        </p>
        <ul class="text-body-small mb-0 pl-4">
          <li v-for="o in orphans" :key="o.container_name">
            {{ o.container_name }}
            <span class="brand-text-muted">
              ({{ o.status }}{{ o.running ? ', running' : '' }}
              <template v-if="o.runner_id"> · id {{ o.runner_id }}</template>)
            </span>
          </li>
        </ul>
      </v-alert>

      <StandardCard title="Local runners" content-class="pa-0" :loading="loading">
        <template #titleAppend>
          <v-btn
            color="white"
            variant="outlined"
            prepend-icon="mdi-refresh"
            :loading="loading"
            @click="refresh"
          >
            Refresh
          </v-btn>
          <v-btn
            color="white"
            variant="flat"
            class="text-primary"
            prepend-icon="mdi-plus"
            @click="createOpen = true"
          >
            Create runner
          </v-btn>
        </template>

        <template v-if="error" #alerts>
          <v-alert
            type="error"
            variant="tonal"
            density="comfortable"
            closable
            @click:close="store.commit('setError', null)"
          >
            {{ error }}
          </v-alert>
        </template>

        <EmptyState
          v-if="!runners.length && !error"
          icon="mdi-github"
          title="No runners yet"
          copy="Create a runner with a name, GitHub project URL, and registration token (or a configured PAT)."
        />

        <v-row v-else-if="runners.length" no-gutters class="runners-layout">
          <v-col cols="12" :md="selected && isDesktop ? 8 : 12">
            <v-data-table
              :headers="headers"
              :items="runners"
              item-value="id"
              class="runners-table"
              hover
              :row-props="rowProps"
              @click:row="onRowClick"
            >
              <template #item.status="{ item }">
                <v-chip size="small" label :color="statusColor(item.status)" variant="tonal">
                  {{ item.status }}
                </v-chip>
              </template>
              <template #item.labels="{ item }">
                <span class="text-body-small">{{ (item.labels || []).join(', ') }}</span>
              </template>
            </v-data-table>
          </v-col>

          <v-col v-if="selected && isDesktop" cols="12" md="4" class="runner-details-pane">
            <div class="standard-card-header">
              <RunnerDetailsHeader
                :name="selected.name"
                card-title
                :loading="actionLoading"
                @edit="editOpen = true"
                @delete="deleteOpen = true"
              />
            </div>
            <div class="pa-4">
              <RunnerDetails :runner="selected" />
            </div>
            <div class="standard-card-actions pa-4">
              <RunnerActions
                :runner="selected"
                :loading="actionLoading"
                @start="runAction('startRunner')"
                @stop="runAction('stopRunner')"
                @restart="runAction('restartRunner')"
                @recreate="openRecreate"
                @logs="openLogs"
              />
            </div>
          </v-col>
        </v-row>
      </StandardCard>
    </template>

    <CreateRunnerDialog v-model="createOpen" />
    <EditRunnerDialog v-model="editOpen" :runner="selected" />

    <StandardDialog
      :model-value="detailsOpen"
      :title="selected?.name || 'Runner'"
      fullscreen
      max-width="560"
      @update:model-value="onDetailsDialog"
    >
      <template v-if="selected" #header>
        <RunnerDetailsHeader
          :name="selected.name"
          :loading="actionLoading"
          @edit="editOpen = true"
          @delete="deleteOpen = true"
        />
      </template>
      <RunnerDetails v-if="selected" :runner="selected" />
      <template #actions>
        <RunnerActions
          v-if="selected"
          :runner="selected"
          :loading="actionLoading"
          @start="runAction('startRunner')"
          @stop="runAction('stopRunner')"
          @restart="runAction('restartRunner')"
          @recreate="openRecreate"
          @logs="openLogs"
        />
      </template>
    </StandardDialog>

    <StandardDialog v-model="deleteOpen" title="Delete runner" max-width="480">
      <p class="text-body-medium">
        Remove <strong>{{ selected?.name }}</strong> container and volume?
        <template v-if="patConfigured">
          The runner will also be deregistered from GitHub when possible.
        </template>
        <template v-else>
          This does not deregister the runner from GitHub — remove it there manually if needed.
        </template>
      </p>
      <template #actions>
        <v-btn color="primary" variant="tonal" @click="deleteOpen = false">Cancel</v-btn>
        <v-btn
          color="error"
          variant="elevated"
          prepend-icon="mdi-delete"
          :loading="actionLoading"
          @click="confirmDelete"
        >
          Delete
        </v-btn>
      </template>
    </StandardDialog>

    <StandardDialog v-model="recreateOpen" title="Recreate runner" max-width="480">
      <p class="text-body-medium brand-text-muted mb-4">
        Rebuild the container from stored config. The registration volume is kept when present. If
        the volume is missing, a registration token or configured PAT is required.
      </p>
      <v-text-field
        v-if="!patConfigured"
        v-model="recreateToken"
        label="Registration token (required if data volume is missing)"
        type="password"
        autocomplete="off"
      />
      <v-alert v-else type="info" variant="tonal" density="comfortable">
        A PAT is configured — a registration token will be minted when the volume cannot be reused.
      </v-alert>
      <template #actions>
        <v-btn color="primary" variant="tonal" @click="recreateOpen = false">Cancel</v-btn>
        <v-btn
          color="primary"
          variant="elevated"
          prepend-icon="mdi-reload"
          :loading="actionLoading"
          @click="confirmRecreate"
        >
          Recreate
        </v-btn>
      </template>
    </StandardDialog>

    <RunnerLogsDialog v-model="logsOpen" :runner-id="logsRunnerId" />
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useStore } from 'vuex'
import StandardCard from '@/components/common/StandardCard.vue'
import StandardDialog from '@/components/common/StandardDialog.vue'
import PageLoader from '@/components/common/PageLoader.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import CreateRunnerDialog from '@/components/CreateRunnerDialog.vue'
import EditRunnerDialog from '@/components/EditRunnerDialog.vue'
import RunnerDetails from '@/components/RunnerDetails.vue'
import RunnerDetailsHeader from '@/components/RunnerDetailsHeader.vue'
import RunnerActions from '@/components/RunnerActions.vue'
import RunnerLogsDialog from '@/components/RunnerLogsDialog.vue'
import { useListDetailsPane } from '@/composables/useListDetailsPane'
import { countByStatus, statusColor } from '@/utils/runnerStatus'

const store = useStore()
const route = useRoute()
const router = useRouter()
const createOpen = ref(false)
const editOpen = ref(false)
const deleteOpen = ref(false)
const recreateOpen = ref(false)
const recreateToken = ref('')
const logsOpen = ref(false)
const logsRunnerId = ref('')
const actionLoading = ref(false)
const { selectedId, detailsOpen, isDesktop, select, clear } = useListDetailsPane()
let pollTimer

const headers = [
  { title: 'Name', key: 'name', sortable: true },
  { title: 'Project URL', key: 'url', sortable: true },
  { title: 'Status', key: 'status', sortable: true },
  { title: 'Labels', key: 'labels', sortable: false },
]

const runners = computed(() => store.state.runners)
const counts = computed(() => countByStatus(runners.value))
const summaryChips = computed(() => [
  { key: 'running', label: 'Running', value: counts.value.running, color: statusColor('running') },
  { key: 'exited', label: 'Exited', value: counts.value.exited, color: statusColor('exited') },
  { key: 'missing', label: 'Missing', value: counts.value.missing, color: statusColor('missing') },
  { key: 'total', label: 'Total', value: counts.value.total, color: statusColor('unknown') },
])
const initialLoading = computed(() => store.state.initialLoading)
const loading = computed(() => store.state.loading)
const error = computed(() => store.state.error)
const patConfigured = computed(() => store.state.githubPatConfigured)
const orphans = computed(() => store.state.orphans || [])
const storeReadable = computed(() => store.state.storeReadable !== false)
const storeError = computed(() => store.state.storeError)
const selected = computed(() => runners.value.find((r) => r.id === selectedId.value) || null)

function rowProps({ item }) {
  const row = item?.raw ?? item
  return {
    class: row?.id === selectedId.value ? 'selected-row' : '',
    tabindex: 0,
    'aria-selected': row?.id === selectedId.value ? 'true' : 'false',
    onKeydown: (e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault()
        if (row?.id) select(row.id)
      }
    },
  }
}

function onRowClick(_e, ctx) {
  const item = ctx?.item?.raw ?? ctx?.item
  if (item?.id) select(item.id)
}

function onDetailsDialog(open) {
  detailsOpen.value = open
  if (!open && !isDesktop.value) clear()
}

function openLogs() {
  if (!selected.value?.id) return
  logsRunnerId.value = selected.value.id
  logsOpen.value = true
}

function openRecreate() {
  recreateToken.value = ''
  recreateOpen.value = true
}

async function refresh() {
  try {
    await store.dispatch('fetchRunners')
    await store.dispatch('fetchHealth')
  } catch {
    /* error surfaced via store */
  }
}

async function runAction(action) {
  if (!selected.value) return
  actionLoading.value = true
  try {
    await store.dispatch(action, selected.value.id)
  } catch {
    /* error surfaced via store */
  } finally {
    actionLoading.value = false
  }
}

async function confirmRecreate() {
  if (!selected.value) return
  actionLoading.value = true
  try {
    await store.dispatch('recreateRunner', {
      id: selected.value.id,
      token: recreateToken.value.trim() || undefined,
    })
    recreateOpen.value = false
  } catch {
    /* error surfaced via store */
  } finally {
    actionLoading.value = false
  }
}

async function confirmDelete() {
  if (!selected.value) return
  actionLoading.value = true
  try {
    const id = selected.value.id
    await store.dispatch('deleteRunner', id)
    if (selectedId.value === id) clear()
    deleteOpen.value = false
  } catch {
    /* error surfaced via store */
  } finally {
    actionLoading.value = false
  }
}

watch(selectedId, (id) => {
  const q = { ...route.query }
  if (id) q.id = id
  else delete q.id
  router.replace({ query: q }).catch(() => {})
})

onMounted(async () => {
  await Promise.all([
    store.dispatch('fetchRunners', { initial: true }).catch(() => {}),
    store.dispatch('fetchHealth').catch(() => {}),
  ])
  const fromQuery = typeof route.query.id === 'string' ? route.query.id : ''
  if (fromQuery && runners.value.some((r) => r.id === fromQuery)) {
    select(fromQuery)
  }
  pollTimer = setInterval(() => {
    store.dispatch('fetchRunners').catch(() => {})
    store.dispatch('fetchHealth').catch(() => {})
  }, 10000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<style scoped>
.runner-details-pane {
  border-inline-start: 1px solid rgba(var(--v-theme-on-surface), 0.12);
  min-width: 0;
}
.runners-table :deep(th) {
  white-space: nowrap;
}
.runners-table :deep(tr) {
  cursor: pointer;
}
</style>
