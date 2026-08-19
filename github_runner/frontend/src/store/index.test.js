import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/utils/api', () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    patch: vi.fn(),
    delete: vi.fn(),
  },
}))

import { api } from '@/utils/api'
import store from './index.js'

describe('runner store', () => {
  beforeEach(() => {
    store.commit('setError', null)
    store.commit('setRunners', [])
    vi.mocked(api.get).mockReset()
    vi.mocked(api.post).mockReset()
  })

  it('does not clear mutation errors on a successful list poll', async () => {
    store.commit('setError', 'recreate failed')
    api.get.mockResolvedValue({ runners: [{ id: 'a' }] })
    await store.dispatch('fetchRunners')
    expect(store.state.error).toBe('recreate failed')
    expect(store.state.runners).toEqual([{ id: 'a' }])
  })

  it('sets error when list poll fails', async () => {
    api.get.mockRejectedValue(new Error('network'))
    await expect(store.dispatch('fetchRunners')).rejects.toThrow('network')
    expect(store.state.error).toBe('network')
  })

  it('clears error after a successful mutate', async () => {
    store.commit('setError', 'old')
    api.post.mockResolvedValue({})
    api.get.mockResolvedValue({ runners: [] })
    await store.dispatch('startRunner', 'id1')
    expect(store.state.error).toBeNull()
  })

  it('refreshes the list without clearing error when mutate fails', async () => {
    api.post.mockRejectedValue(new Error('create failed'))
    api.get.mockResolvedValue({ runners: [{ id: 'kept' }] })
    await expect(store.dispatch('createRunner', {})).rejects.toThrow('create failed')
    expect(store.state.error).toBe('create failed')
    expect(store.state.runners).toEqual([{ id: 'kept' }])
    expect(api.get).toHaveBeenCalledWith('/api/v1/runners')
  })
})
