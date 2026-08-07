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

export function cacheFromRunner(runner) {
  const c = runner?.cache
  if (!c || !c.enabled) {
    return {
      enabled: false,
      type: 'volume',
      volumeName: '',
      hostPath: '',
      target: '/cache',
      readOnly: false,
    }
  }
  return {
    enabled: true,
    type: c.type === 'bind' ? 'bind' : 'volume',
    volumeName: c.volume_name || '',
    hostPath: c.host_path || '',
    target: c.target || '/cache',
    readOnly: !!c.read_only,
  }
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
  cacheEnabled = false,
  cacheType = 'volume',
  cacheVolumeName = '',
  cacheHostPath = '',
  cacheTarget = '/cache',
  cacheReadOnly = false,
  persistWorkdir = false,
  workdirHostPath = '',
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

  payload.persist_workdir = !!persistWorkdir
  payload.workdir_host_path = String(workdirHostPath || '').trim()
  if (cacheEnabled) {
    const type = cacheType === 'bind' ? 'bind' : 'volume'
    const cache = {
      enabled: true,
      type,
      target: String(cacheTarget || '/cache').trim() || '/cache',
      read_only: !!cacheReadOnly,
    }
    if (type === 'bind') {
      cache.host_path = String(cacheHostPath || '').trim()
    } else {
      const vn = String(cacheVolumeName || '').trim()
      if (vn) cache.volume_name = vn
    }
    payload.cache = cache
  } else {
    payload.cache = { enabled: false }
  }
  return payload
}
