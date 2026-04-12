import { createRouter, createWebHistory } from 'vue-router'
import GlobalDashboard from '../views/GlobalDashboard.vue'
import SiteDashboard from '../views/SiteDashboard.vue'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      redirect: '/global'
    },
    {
      path: '/global',
      name: 'global',
      component: GlobalDashboard
    },
    {
      path: '/site/:site_id',
      name: 'site',
      component: SiteDashboard,
      props: true
    }
  ]
})

export default router
