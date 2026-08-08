import { describe, expect, it } from 'vitest'
import { countByStatus, displayStatus, isRunnerBusy, statusColor } from './runnerStatus.js'

describe('runnerStatus', () => {
  it('maps status colors', () => {
    expect(statusColor('running')).toBe('success')
    expect(statusColor('idle')).toBe('success')
    expect(statusColor('busy')).toBe('warning')
    expect(statusColor('exited')).toBe('warning')
    expect(statusColor('missing')).toBe('error')
    expect(statusColor('unknown')).toBe('info')
  })

  it('counts by status and busy jobs', () => {
    expect(
      countByStatus([
        { status: 'running', job_state: 'idle' },
        { status: 'running', job_state: 'busy' },
        { status: 'missing' },
      ]),
    ).toEqual({ running: 2, exited: 0, missing: 1, unknown: 0, busy: 1, total: 3 })
  })

  it('displayStatus prefers job_state when running', () => {
    expect(displayStatus({ status: 'running', running: true, job_state: 'idle' })).toBe('idle')
    expect(displayStatus({ status: 'running', running: true, job_state: 'busy' })).toBe('busy')
    expect(displayStatus({ status: 'running', running: true })).toBe('unknown')
    expect(displayStatus({ status: 'exited', running: false })).toBe('exited')
    expect(displayStatus({ status: 'missing' })).toBe('missing')
  })

  it('isRunnerBusy is true only for job_state busy', () => {
    expect(isRunnerBusy({ job_state: 'busy' })).toBe(true)
    expect(isRunnerBusy({ job_state: 'idle' })).toBe(false)
    expect(isRunnerBusy({ job_state: 'unknown' })).toBe(false)
    expect(isRunnerBusy(null)).toBe(false)
  })
})
