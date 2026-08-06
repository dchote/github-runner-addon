/**
 * Resolve browser-facing app base (HA ingress or `/`).
 * Prefers `<base href>` injected by the SPA handler; falls back to location.
 */
export function appBasePath() {
  const attr = document.querySelector('base')?.getAttribute('href')
  if (attr && attr !== './') {
    return attr.endsWith('/') ? attr : `${attr}/`
  }
  const match = window.location.pathname.match(/^(.*?\/api\/hassio_ingress\/[^/]+)/)
  if (match) return `${match[1]}/`
  return '/'
}

/** Absolute URL for an API or app path (leading slash optional). */
export function resolveURL(path) {
  const clean = String(path || '').replace(/^\//, '')
  return new URL(clean, `${window.location.origin}${appBasePath()}`).toString()
}

/** WebSocket URL for the addon `/ws` endpoint. */
export function resolveWSURL(path = 'ws') {
  const url = new URL(resolveURL(path))
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.toString()
}

async function parseEnvelope(res) {
  const body = await res.json().catch(() => ({}))
  if (!res.ok || body.result === 'error') {
    const msg = body?.error?.message || res.statusText || 'Request failed'
    const err = new Error(msg)
    err.code = body?.error?.code
    err.status = res.status
    throw err
  }
  return body.data
}

export const api = {
  async get(path) {
    const res = await fetch(resolveURL(path))
    return parseEnvelope(res)
  },
  async post(path, body) {
    const res = await fetch(resolveURL(path), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: body == null ? undefined : JSON.stringify(body),
    })
    return parseEnvelope(res)
  },
  async patch(path, body) {
    const res = await fetch(resolveURL(path), {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: body == null ? undefined : JSON.stringify(body),
    })
    return parseEnvelope(res)
  },
  async delete(path) {
    const res = await fetch(resolveURL(path), { method: 'DELETE' })
    return parseEnvelope(res)
  },
}
