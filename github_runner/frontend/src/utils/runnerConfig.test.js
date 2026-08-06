import { describe, expect, it } from 'vitest'
import { formatExtraEnv, parseExtraEnvText, buildRuntimePayload } from './runnerConfig.js'

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
  })
})
