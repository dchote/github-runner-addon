import { describe, expect, it } from 'vitest'
import {
  formatExtraEnv,
  parseExtraEnvText,
  buildRuntimePayload,
  cacheFromRunner,
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
    expect(p.persist_workdir).toBe(false)
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
      persistWorkdir: true,
    })
    expect(p.cache).toEqual({
      enabled: true,
      type: 'volume',
      target: '/cache',
      read_only: true,
      volume_name: 'shared-cache',
    })
    expect(p.persist_workdir).toBe(true)
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
})
