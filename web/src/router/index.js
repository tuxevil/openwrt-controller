import { createRouter, createWebHistory } from 'vue-router'
import GlobalDashboard from '../views/GlobalDashboard.vue'
import SiteDashboard from '../views/SiteDashboard.vue'

import ClientList from '../views/ClientList.vue'
import SiteSettings from '../views/SiteSettings.vue'
import LogConsole from '../views/LogConsole.vue'
import WirelessManager from '../views/WirelessManager.vue'

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
      name: 'site-dashboard',
      component: SiteDashboard,
      props: true
    },
    {
      path: '/site/:site_id/clients',
      name: 'site-clients',
      component: ClientList,
      props: true
    },
    {
      path: '/site/:site_id/settings',
      name: 'site-settings',
      component: SiteSettings,
      props: true
    },
    {
      path: '/site/:site_id/logs',
      name: 'site-logs',
      component: LogConsole,
      props: true
    },
    {
      path: '/site/:site_id/wireless',
      name: 'site-wireless',
      component: WirelessManager,
      props: true
    }
  ]
})

export default router
