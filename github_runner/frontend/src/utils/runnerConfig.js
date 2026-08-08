/** Match github_runner/internal/container/docker.NormalizeName for default workdir paths. */
export function normalizeRunnerName(name) {
  let n = String(name || '')
    .toLowerCase()
    .replace(/[^a-z0-9_-]/g, '-')
    .replace(/^[-_]+|[-_]+$/g, '')
  return n || 'runner'
}

export function defaultWorkdirHostPath(runnerName) {
  return `/srv/gha-work/${normalizeRunnerName(runnerName)}`
}

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

/**
 * Align with Go path.Clean for absolute cache paths (slashes, ".", "..").
 * Exported for tests.
 */
export function cleanCachePath(p) {
  let s = String(p || '').trim()
  if (!s) return ''
  const abs = s.startsWith('/')
  const parts = []
  for (const part of s.split('/')) {
    if (!part || part === '.') continue
    if (part === '..') {
      if (parts.length) parts.pop()
      continue
    }
    parts.push(part)
  }
  if (abs) {
    return '/' + parts.join('/')
  }
  return parts.join('/') || '.'
}

/** Match Go cacheType: omit/empty type defaults to volume (API); UI create still defaults bind. */
export function cacheTypeFromRecord(type) {
  const t = String(type || '')
    .toLowerCase()
    .trim()
  if (!t || t === 'volume') return 'volume'
  if (t === 'bind') return 'bind'
  return t
}

export function cacheFromRunner(runner) {
  const c = runner?.cache
  if (!c || !c.enabled) {
    return {
      enabled: false,
      type: 'bind',
      volumeName: '',
      hostPath: '',
      target: '/cache',
      readOnly: false,
    }
  }
  const type = cacheTypeFromRecord(c.type)
  const hostPath = type === 'bind' ? cleanCachePath(c.host_path || '') : ''
  // Bind is always same-path; prefer host_path as the displayed target.
  const target =
    type === 'bind'
      ? hostPath || cleanCachePath(c.target || '')
      : cleanCachePath(c.target || '') || '/cache'
  return {
    enabled: true,
    type,
    volumeName: type === 'volume' ? c.volume_name || '' : '',
    hostPath,
    target,
    readOnly: !!c.read_only,
  }
}

/**
 * Soft advisory when cache mount will miss sibling Docker / Buildx type=local.
 * Keep wording in sync with github_runner/internal/runner cacheSiblingWarnings.
 * Bind mounts are always same-path — only named volumes warn.
 */
export function cacheSiblingPathWarning({
  enabled = false,
  type = 'bind',
  target = '/cache',
} = {}) {
  if (!enabled || type !== 'volume') return ''
  const tg = cleanCachePath(target) || '/cache'
  return (
    `cache uses a named volume mounted at "${tg}"; sibling Docker and Buildx type=local ` +
    `that bind-mount that path on the Docker host will not see this volume. Prefer a host bind ` +
    `(same-path) and use $RUNNER_CACHE in workflows when sibling visibility is required.`
  )
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
  cacheType = 'bind',
  cacheVolumeName = '',
  cacheHostPath = '',
  cacheTarget = '/cache',
  cacheReadOnly = false,
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
  payload.workdir_host_path = String(workdirHostPath || '').trim()

  if (cacheEnabled) {
    const type = cacheType === 'volume' ? 'volume' : 'bind'
    const cache = {
      enabled: true,
      type,
      read_only: !!cacheReadOnly,
    }
    if (type === 'bind') {
      const hp = cleanCachePath(cacheHostPath)
      cache.host_path = hp
      // Same-path: API also coerces target = host_path.
      cache.target = hp
    } else {
      cache.target = cleanCachePath(cacheTarget) || '/cache'
      const vn = String(cacheVolumeName || '').trim()
      if (vn) cache.volume_name = vn
    }
    payload.cache = cache
  } else {
    payload.cache = { enabled: false }
  }
  return payload
}
