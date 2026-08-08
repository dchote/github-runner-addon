/** Status → Vuetify color for chips/summary (single source of truth). */
export function statusColor(status) {
  if (status === 'running' || status === 'idle') return 'success'
  if (status === 'busy') return 'warning'
  if (status === 'missing') return 'error'
  if (status === 'exited') return 'warning'
  if (status === 'unknown') return 'info'
  return 'info'
}

/**
 * Value shown in the Status column: job activity when the container is running,
 * otherwise Docker lifecycle status.
 */
export function displayStatus(runner) {
  if (!runner) return 'unknown'
  if (runner.running || runner.status === 'running') {
    return runner.job_state || 'unknown'
  }
  return runner.status || 'unknown'
}

/** Aggregate counts from an enriched runners list (avoids a second health round-trip). */
export function countByStatus(runners) {
  const counts = { running: 0, exited: 0, missing: 0, unknown: 0, busy: 0, total: 0 }
  const list = runners || []
  counts.total = list.length
  for (const r of list) {
    const s = r?.status || 'unknown'
    if (counts[s] != null) counts[s] += 1
    else counts.unknown += 1
    if (r?.job_state === 'busy') counts.busy += 1
  }
  return counts
}
