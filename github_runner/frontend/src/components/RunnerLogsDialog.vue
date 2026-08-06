<template>
  <StandardDialog
    :model-value="modelValue"
    :title="dialogTitle"
    :fullscreen="mobile"
    max-width="960"
    fill-body
    content-padding="pa-0"
    @update:model-value="onOpenChange"
  >
    <div class="logs-dialog-toolbar d-flex align-center flex-wrap" style="gap: 6px">
      <v-switch
        v-model="follow"
        class="logs-dialog-toolbar__follow ms-0"
        color="primary"
        density="compact"
        hide-details
        label="Follow"
      />
      <div class="logs-dialog-toolbar__actions d-flex align-center flex-wrap ms-auto" style="gap: 6px">
        <v-btn color="primary" variant="tonal" size="small" @click="clearLines">Clear</v-btn>
        <v-btn
          color="primary"
          variant="tonal"
          size="small"
          :loading="connecting"
          @click="reconnect"
        >
          Reconnect
        </v-btn>
        <v-btn
          color="primary"
          variant="tonal"
          size="small"
          prepend-icon="mdi-download"
          @click="downloadLogs"
        >
          Download
        </v-btn>
      </div>
    </div>

    <v-alert
      v-if="error"
      type="error"
      variant="tonal"
      density="comfortable"
      class="ma-4 mb-0"
    >
      {{ error }}
    </v-alert>

    <div
      ref="viewport"
      class="log-viewport"
      role="log"
      aria-live="polite"
      aria-relevant="additions"
      @scroll="onScroll"
    >
      <div v-for="(line, i) in lines" :key="i" class="log-viewport__line" v-html="line.html" />
      <div v-if="!lines.length && !connecting" class="text-body-medium brand-text-muted pa-2">
        No log lines yet.
      </div>
    </div>

    <template #actions>
      <v-btn color="primary" variant="tonal" @click="close">Close</v-btn>
    </template>
  </StandardDialog>
</template>

<script setup>
import { AnsiUp } from 'ansi_up'
import { computed, nextTick, ref, watch } from 'vue'
import { useStore } from 'vuex'
import StandardDialog from '@/components/common/StandardDialog.vue'
import { useMobile } from '@/composables/useMobile'
import { api, resolveURL, resolveWSURL } from '@/utils/api'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  runnerId: { type: String, default: '' },
})

const emit = defineEmits(['update:modelValue'])

const store = useStore()
const mobile = useMobile()
const ansi = new AnsiUp()
ansi.use_classes = false
ansi.escape_html = true

const lines = ref([])
const plainLines = ref([])
const follow = ref(true)
const connecting = ref(false)
const error = ref('')
const viewport = ref(null)
let socket = null
let socketGen = 0
let stickToBottom = true
let reconnectTimer = null
let reconnectAttempt = 0

const runner = computed(() => store.state.runners.find((r) => r.id === props.runnerId))
const dialogTitle = computed(() =>
  runner.value?.name ? `Logs — ${runner.value.name}` : 'Container logs',
)

function close() {
  emit('update:modelValue', false)
}

function onOpenChange(open) {
  emit('update:modelValue', open)
}

function clearLines() {
  lines.value = []
  plainLines.value = []
}

function appendText(text) {
  const parts = String(text).split(/\r?\n/)
  for (const part of parts) {
    if (part === '' && parts.length === 1) continue
    plainLines.value.push(part)
    lines.value.push({ html: ansi.ansi_to_html(part) })
  }
  if (lines.value.length > 5000) {
    lines.value = lines.value.slice(-4000)
    plainLines.value = plainLines.value.slice(-4000)
  }
  if (follow.value && stickToBottom) {
    nextTick(() => {
      if (viewport.value) viewport.value.scrollTop = viewport.value.scrollHeight
    })
  }
}

function onScroll() {
  if (!viewport.value) return
  const el = viewport.value
  stickToBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40
}

function downloadLogs() {
  const name = runner.value?.name || props.runnerId || 'runner'
  const blob = new Blob([plainLines.value.join('\n') + '\n'], { type: 'text/plain;charset=utf-8' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = `${name}-logs.txt`
  a.click()
  URL.revokeObjectURL(a.href)
}

function clearReconnect() {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
}

function scheduleReconnect() {
  clearReconnect()
  if (!props.modelValue || !follow.value) return
  const delay = Math.min(30000, 1000 * 2 ** Math.min(reconnectAttempt, 4))
  reconnectAttempt += 1
  reconnectTimer = setTimeout(() => {
    streamLogs({ clear: false })
  }, delay)
}

function stopStream() {
  const closing = socket
  socket = null
  socketGen += 1
  clearReconnect()
  if (!closing) return
  try {
    if (closing.readyState === WebSocket.OPEN) {
      closing.send(JSON.stringify({ type: 'unsubscribe', channel: 'container_logs' }))
    }
    closing.close()
  } catch {
    /* ignore */
  }
}

async function ensureRunner() {
  if (!props.runnerId) return
  if (store.state.runners.some((r) => r.id === props.runnerId)) return
  const data = await api.get(`/api/v1/runners/${props.runnerId}`)
  if (data?.id) {
    const list = store.state.runners.slice()
    const idx = list.findIndex((r) => r.id === data.id)
    if (idx >= 0) list[idx] = data
    else list.push(data)
    store.commit('setRunners', list)
  }
}

async function loadSnapshot() {
  // Plain-text log stream (not JSON envelope) — fetch via resolveURL.
  const url = resolveURL(`/api/v1/runners/${props.runnerId}/logs?follow=0&tail=200`)
  const res = await fetch(url)
  if (!res.ok) {
    const ct = res.headers.get('content-type') || ''
    if (ct.includes('application/json')) {
      const body = await res.json().catch(() => ({}))
      throw new Error(body?.error?.message || res.statusText)
    }
    throw new Error(res.statusText || 'Failed to load logs')
  }
  const text = await res.text()
  if (text) appendText(text.replace(/\n$/, ''))
}

function connectWS() {
  return new Promise((resolve, reject) => {
    const gen = socketGen
    const ws = new WebSocket(resolveWSURL('/ws'))
    socket = ws
    let opened = false
    ws.onopen = () => {
      if (gen !== socketGen || socket !== ws) return
      opened = true
      reconnectAttempt = 0
      error.value = ''
      ws.send(
        JSON.stringify({
          type: 'subscribe',
          channel: 'container_logs',
          runner_id: props.runnerId,
          tail: '200',
        }),
      )
      resolve()
    }
    ws.onmessage = (ev) => {
      if (socket !== ws) return
      let msg
      try {
        msg = JSON.parse(ev.data)
      } catch {
        return
      }
      if (msg.type === 'log_line' && msg.line != null) {
        appendText(msg.line)
      } else if (msg.type === 'error') {
        error.value = msg.error || 'WebSocket log error'
      }
    }
    ws.onerror = () => {
      if (socket !== ws) return
      if (!opened) reject(new Error('WebSocket connection failed'))
    }
    ws.onclose = () => {
      if (socket === ws) {
        socket = null
        if (follow.value && props.modelValue) {
          error.value = 'Log stream disconnected — reconnecting…'
          scheduleReconnect()
        }
      }
    }
  })
}

async function streamLogs({ clear = true } = {}) {
  if (!props.runnerId) return
  stopStream()
  connecting.value = true
  if (clear) {
    error.value = ''
    clearLines()
  }
  try {
    await ensureRunner()
    if (follow.value) {
      await connectWS()
    } else {
      await loadSnapshot()
    }
  } catch (e) {
    error.value = e.message || String(e)
    if (follow.value && props.modelValue) scheduleReconnect()
  } finally {
    connecting.value = false
  }
}

function reconnect() {
  reconnectAttempt = 0
  streamLogs({ clear: true })
}

watch(
  () => props.modelValue,
  (open) => {
    if (open && props.runnerId) {
      reconnectAttempt = 0
      streamLogs({ clear: true })
    } else {
      stopStream()
      clearLines()
      error.value = ''
      reconnectAttempt = 0
    }
  },
)

watch(
  () => props.runnerId,
  (id) => {
    if (props.modelValue && id) streamLogs({ clear: true })
  },
)

watch(follow, () => {
  if (!props.modelValue) return
  streamLogs({ clear: true })
})
</script>

<style scoped>
.logs-dialog-toolbar {
  padding: 4px 12px;
  min-height: 36px;
  background: linear-gradient(
    180deg,
    rgba(var(--v-theme-on-surface), 0.04) 0%,
    rgba(var(--v-theme-on-surface), 0.02) 100%
  );
  flex-shrink: 0;
}

.logs-dialog-toolbar__follow {
  --v-input-control-height: 28px;
  margin-inline-end: 0;
  flex: 0 0 auto;
}

.logs-dialog-toolbar__follow :deep(.v-selection-control) {
  min-height: 28px;
}

.logs-dialog-toolbar__follow :deep(.v-label) {
  font-size: 0.8125rem;
}

.log-viewport {
  background: #0b1520;
  color: #d7e2ec;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  line-height: 1.45;
  flex: 1 1 auto;
  min-height: 320px;
  overflow: auto;
  padding: 12px;
  border-radius: 0;
}

.log-viewport__line {
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
