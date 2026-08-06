/** Parse KEY=value lines into an object; empty → {}. */
export function parseExtraEnvText(text) {
  const out = {}
  for (const line of String(text || '').split(/\r?\n/)) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) continue
    const eq = trimmed.indexOf('=')
    if (eq <= 0) {
      throw new Error(`Invalid extra env line (expected KEY=value): ${trimmed}`)
    }
    const key = trimmed.slice(0, eq).trim()
    const value = trimmed.slice(eq + 1)
    if (!key) throw new Error('Empty extra env key')
    out[key] = value
  }
  return out
}

export function formatExtraEnv(env) {
  if (!env || typeof env !== 'object') return ''
  return Object.entries(env)
    .map(([k, v]) => `${k}=${v}`)
    .join('\n')
}

export function normalizeLabelList(chips) {
  return (chips || [])
    .map((s) => String(s).trim())
    .filter(Boolean)
}

/** Build create/patch payload fields from shared form state. */
export function buildRuntimePayload({
  labels,
  image,
  cpuLimit,
  memoryLimitMb,
  networkMode,
  extraEnvText,
  mountDockerSock,
}) {
  const payload = {}
  const labelList = normalizeLabelList(labels)
  if (labelList.length) payload.labels = labelList
  else payload.labels = []
  if (image != null) payload.image = String(image).trim()
  payload.cpu_limit = Number(cpuLimit) > 0 ? Number(cpuLimit) : 0
  payload.memory_limit_mb = Number(memoryLimitMb) > 0 ? Number(memoryLimitMb) : 0
  payload.network_mode = String(networkMode || '').trim()
  payload.extra_env = parseExtraEnvText(extraEnvText)
  if (mountDockerSock === true || mountDockerSock === false) {
    payload.mount_docker_sock = mountDockerSock
  } else {
    payload.mount_docker_sock = null
  }
  return payload
}
