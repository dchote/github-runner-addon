import { beforeEach, describe, expect, it } from 'vitest'
import { appBasePath, resolveURL, resolveWSURL } from './api.js'

describe('api URL helpers', () => {
  beforeEach(() => {
    document.head.innerHTML = ''
    window.history.replaceState({}, '', '/')
  })

  it('defaults to root base', () => {
    expect(appBasePath()).toBe('/')
    expect(resolveURL('/api/v1/health')).toBe(`${window.location.origin}/api/v1/health`)
  })

  it('uses base href when present', () => {
    const base = document.createElement('base')
    base.setAttribute('href', '/api/hassio_ingress/tok/')
    document.head.appendChild(base)
    expect(appBasePath()).toBe('/api/hassio_ingress/tok/')
    expect(resolveURL('api/v1/runners')).toContain('/api/hassio_ingress/tok/api/v1/runners')
  })

  it('maps resolveWSURL to ws protocol', () => {
    const url = resolveWSURL('/ws')
    expect(url.startsWith('ws://') || url.startsWith('wss://')).toBe(true)
    expect(url.endsWith('/ws')).toBe(true)
  })
})
