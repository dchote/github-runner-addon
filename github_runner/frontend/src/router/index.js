import { createRouter, createWebHistory } from 'vue-router'
import { appBasePath } from '@/utils/api'

const routes = [
  {
    path: '/',
    name: 'runners',
    component: () => import('@/pages/RunnersPage.vue'),
  },
]

const router = createRouter({
  history: createWebHistory(appBasePath()),
  routes,
})

export default router
