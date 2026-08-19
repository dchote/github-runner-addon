import { createStore } from 'vuex'
import { api } from '@/utils/api'

async function mutateRunner({ commit, dispatch }, fn, { refreshHealth = false } = {}) {
  try {
    const result = await fn()
    commit('setError', null)
    await dispatch('fetchRunners')
    if (refreshHealth) await dispatch('fetchHealth')
    return result
  } catch (e) {
    commit('setError', e.message || String(e))
    await dispatch('fetchRunners').catch(() => {})
    throw e
  }
}

export default createStore({
  state: {
    runners: [],
    initialLoading: true,
    loading: false,
    error: null,
    dockerAvailable: null,
    dockerEngine: 'Docker',
    dockerError: null,
    githubPatConfigured: false,
    runnerImage: '',
    mountDockerSock: false,
    appVersion: '',
    orphans: [],
    storeReadable: true,
    storeError: null,
  },
  mutations: {
    setRunners(state, runners) {
      state.runners = runners || []
    },
    upsertRunner(state, runner) {
      if (!runner?.id) return
      const i = state.runners.findIndex((r) => r.id === runner.id)
      if (i >= 0) state.runners.splice(i, 1, { ...state.runners[i], ...runner })
      else state.runners.push(runner)
    },
    setHealth(state, health) {
      state.dockerAvailable = health?.docker?.available ?? null
      state.dockerEngine = health?.docker?.engine || 'Docker'
      state.dockerError = health?.docker?.error || null
      state.githubPatConfigured = !!health?.github_pat_configured
      state.runnerImage = health?.runner_image || ''
      state.mountDockerSock = !!health?.mount_docker_sock
      state.appVersion = health?.version || ''
      state.orphans = health?.orphans || []
      state.storeReadable = health?.store?.readable !== false
      state.storeError = health?.store?.error || null
    },
    setInitialLoading(state, v) {
      state.initialLoading = v
    },
    setLoading(state, v) {
      state.loading = v
    },
    setError(state, err) {
      state.error = err
    },
  },
  actions: {
    async fetchHealth({ commit }) {
      const data = await api.get('/api/v1/health')
      commit('setHealth', data)
      return data
    },
    async fetchRunners({ commit }, { initial = false } = {}) {
      if (initial) commit('setInitialLoading', true)
      else commit('setLoading', true)
      try {
        const data = await api.get('/api/v1/runners')
        commit('setRunners', data.runners || [])
      } catch (e) {
        commit('setError', e.message || String(e))
        throw e
      } finally {
        commit('setInitialLoading', false)
        commit('setLoading', false)
      }
    },
    async fetchRunner({ commit }, id) {
      if (!id) return null
      const runner = await api.get(`/api/v1/runners/${id}`)
      commit('upsertRunner', runner)
      return runner
    },
    createRunner(ctx, payload) {
      return mutateRunner(ctx, () => api.post('/api/v1/runners', payload), { refreshHealth: true })
    },
    startRunner(ctx, id) {
      return mutateRunner(ctx, () => api.post(`/api/v1/runners/${id}/start`))
    },
    stopRunner(ctx, id) {
      return mutateRunner(ctx, () => api.post(`/api/v1/runners/${id}/stop`))
    },
    restartRunner(ctx, id) {
      return mutateRunner(ctx, () => api.post(`/api/v1/runners/${id}/restart`))
    },
    recreateRunner(ctx, { id, token } = {}) {
      const body = token ? { token } : {}
      return mutateRunner(ctx, () => api.post(`/api/v1/runners/${id}/recreate`, body), {
        refreshHealth: true,
      })
    },
    recreateMissingRunners(ctx, { token } = {}) {
      const body = token ? { token } : {}
      return mutateRunner(ctx, () => api.post('/api/v1/runners/recreate-missing', body), {
        refreshHealth: true,
      })
    },
    patchRunner(ctx, { id, payload } = {}) {
      return mutateRunner(ctx, () => api.patch(`/api/v1/runners/${id}`, payload), {
        refreshHealth: !!payload?.apply,
      })
    },
    deleteRunner(ctx, id) {
      return mutateRunner(ctx, () => api.delete(`/api/v1/runners/${id}`), { refreshHealth: true })
    },
  },
})
