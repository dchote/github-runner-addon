import { describe, expect, it } from 'vitest'
import {
  formatExtraEnv,
  parseExtraEnvText,
  buildRuntimePayload,
  cacheFromRunner,
  cacheSiblingPathWarning,
  normalizeRunnerName,
  defaultWorkdirHostPath,
} from './runnerConfig.js'

describe('runnerConfig', () => {
  it('parses and formats extra env', () => {
    expect(parseExtraEnvText('FOO=bar\n# c\nBAZ=1')).toEqual({ FOO: 'bar', BAZ: '1' })
    expect(formatExtraEnv({ FOO: 'bar' })).toBe('FOO=bar')
  })

  it('rejects bad extra env lines', () => {
    expect(() => parseExtraEnvText('NOTVALID')).toThrow(/Invalid/)
  })

  it('builds runtime payload', () => {
    const p = buildRuntimePayload({
      labels: ['self-hosted', 'linux'],
      image: 'img:1',
      cpuLimit: 1,
      memoryLimitMb: 512,
      networkMode: 'bridge',
      extraEnvText: 'A=b',
      mountDockerSock: true,
    })
    expect(p.labels).toEqual(['self-hosted', 'linux'])
    expect(p.image).toBe('img:1')
    expect(p.cpu_limit).toBe(1)
    expect(p.memory_limit_mb).toBe(512)
    expect(p.mount_docker_sock).toBe(true)
    expect(p.extra_env).toEqual({ A: 'b' })
    expect(p.cache).toEqual({ enabled: false })
    expect(p.workdir_host_path).toBe('')
  })

  it('normalizes runner names for default workdir', () => {
    expect(normalizeRunnerName('My Runner')).toBe('my-runner')
    expect(defaultWorkdirHostPath('My Runner')).toBe('/srv/gha-work/my-runner')
  })

  it('includes workdir host path', () => {
    const p = buildRuntimePayload({
      labels: [],
      image: '',
      cpuLimit: 0,
      memoryLimitMb: 0,
      networkMode: '',
      extraEnvText: '',
      mountDockerSock: null,
      workdirHostPath: '/srv/gha-work/lab',
    })
    expect(p.workdir_host_path).toBe('/srv/gha-work/lab')
  })

  it('builds cache volume payload', () => {
    const p = buildRuntimePayload({
      labels: [],
      image: '',
      cpuLimit: 0,
      memoryLimitMb: 0,
      networkMode: '',
      extraEnvText: '',
      mountDockerSock: null,
      cacheEnabled: true,
      cacheType: 'volume',
      cacheVolumeName: 'shared-cache',
      cacheTarget: '/cache',
      cacheReadOnly: true,
    })
    expect(p.cache).toEqual({
      enabled: true,
      type: 'volume',
      target: '/cache',
      read_only: true,
      volume_name: 'shared-cache',
    })
  })

  it('builds cache bind payload', () => {
    const p = buildRuntimePayload({
      labels: [],
      image: '',
      cpuLimit: 0,
      memoryLimitMb: 0,
      networkMode: '',
      extraEnvText: '',
      mountDockerSock: null,
      cacheEnabled: true,
      cacheType: 'bind',
      cacheHostPath: '/srv/runner-cache',
      cacheTarget: '/cache',
    })
    expect(p.cache).toEqual({
      enabled: true,
      type: 'bind',
      target: '/cache',
      read_only: false,
      host_path: '/srv/runner-cache',
    })
  })

  it('reads cache from runner record', () => {
    expect(cacheFromRunner({}).enabled).toBe(false)
    const c = cacheFromRunner({
      cache: {
        enabled: true,
        type: 'bind',
        host_path: '/srv/cache',
        target: '/cache',
        read_only: true,
      },
    })
    expect(c.enabled).toBe(true)
    expect(c.type).toBe('bind')
    expect(c.hostPath).toBe('/srv/cache')
    expect(c.readOnly).toBe(true)
  })

  it('warns when cache bind host path differs from target', () => {
    expect(
      cacheSiblingPathWarning({
        enabled: true,
        type: 'bind',
        hostPath: '/cache',
        target: '/cache',
      }),
    ).toBe('')
    expect(
      cacheSiblingPathWarning({
        enabled: true,
        type: 'bind',
        hostPath: '/cache/',
        target: '/cache',
      }),
    ).toBe('')
    const msg = cacheSiblingPathWarning({
      enabled: true,
      type: 'bind',
      hostPath: '/srv/runner-cache',
      target: '/cache',
    })
    expect(msg).toMatch(/same-path/)
    expect(msg).toContain('"/srv/runner-cache"')
    expect(msg).toContain('"/cache"')
  })

  it('warns when cache uses a named volume (sibling host bind miss)', () => {
    const msg = cacheSiblingPathWarning({
      enabled: true,
      type: 'volume',
      hostPath: '',
      target: '/cache',
    })
    expect(msg).toMatch(/named volume/)
    expect(msg).toContain('"/cache"')
  })
})
