import { describe, expect, it } from 'vitest'
import { countByStatus, statusColor } from './runnerStatus.js'

describe('runnerStatus', () => {
  it('maps status colors', () => {
    expect(statusColor('running')).toBe('success')
    expect(statusColor('exited')).toBe('warning')
    expect(statusColor('missing')).toBe('error')
    expect(statusColor('unknown')).toBe('info')
  })

  it('counts by status', () => {
    expect(
      countByStatus([{ status: 'running' }, { status: 'running' }, { status: 'missing' }]),
    ).toEqual({ running: 2, exited: 0, missing: 1, unknown: 0, total: 3 })
  })
})
