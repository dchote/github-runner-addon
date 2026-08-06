/** Status → Vuetify color for chips/summary (single source of truth). */
export function statusColor(status) {
  if (status === 'running') return 'success'
  if (status === 'missing') return 'error'
  if (status === 'exited') return 'warning'
  return 'info'
}

/** Aggregate counts from an enriched runners list (avoids a second health round-trip). */
export function countByStatus(runners) {
  const counts = { running: 0, exited: 0, missing: 0, unknown: 0, total: 0 }
  const list = runners || []
  counts.total = list.length
  for (const r of list) {
    const s = r?.status || 'unknown'
    if (counts[s] != null) counts[s] += 1
    else counts.unknown += 1
  }
  return counts
}
