import { Comment, Fragment } from 'vue'

function normalizeVnodes(raw) {
  if (raw == null) return []
  return Array.isArray(raw) ? raw : [raw]
}

function vnodeHasRenderableContent(vnode) {
  if (vnode == null || vnode === false) return false
  if (Array.isArray(vnode)) return vnode.some(vnodeHasRenderableContent)
  if (typeof vnode === 'string' || typeof vnode === 'number') return String(vnode).trim().length > 0
  if (typeof vnode !== 'object') return false
  if (vnode.type === Comment) return false
  if (vnode.type === Fragment) return normalizeVnodes(vnode.children).some(vnodeHasRenderableContent)
  return true
}

/**
 * True if a (non-scoped) slot function renders visible content.
 * Used to show footers only when conditional slot content is present.
 */
export function isRenderableSlot(slotFn) {
  if (typeof slotFn !== 'function') return false
  try {
    return normalizeVnodes(slotFn()).some(vnodeHasRenderableContent)
  } catch {
    return false
  }
}
