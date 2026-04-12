import { createRouter, createWebHistory } from 'vue-router'
import GlobalDashboard from '../views/GlobalDashboard.vue'
import SiteDashboard from '../views/SiteDashboard.vue'
import ClientList from '../views/ClientList.vue'
import SiteSettings from '../views/SiteSettings.vue'
import LogConsole from '../views/LogConsole.vue'
import WirelessManager from '../views/WirelessManager.vue'
import Terminal from '../views/Terminal.vue'
import Incidents from '../views/Incidents.vue'
import Topology from '../views/Topology.vue'
import Orchestrator from '../views/Orchestrator.vue'
import RFIntelligence from '../views/RFIntelligence.vue'
import Vault from '../views/Vault.vue'
import Runbook from '../views/Runbook.vue'
import GlobalSettings from '../views/GlobalSettings.vue'
import Login from '../views/Login.vue'
import AgentMgmt from '../views/AgentMgmt.vue'
import VPNMatrix from '../views/VPNMatrix.vue'
import SecurityMatrix from '../views/SecurityMatrix.vue'
import BandwidthSentry from '../views/BandwidthSentry.vue'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: Login,
      meta: { public: true }
    },
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
      path: '/orchestrator',
      name: 'orchestrator',
      component: Orchestrator
    },
    {
      path: '/orchestrator/agent',
      name: 'agent-mgmt',
      component: AgentMgmt
    },
    {
      path: '/runbook',
      name: 'runbook',
      component: Runbook
    },
    {
      path: '/site/:site_id/incidents',
      name: 'incidents',
      component: Incidents,
      props: true
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
    },
    {
      path: '/site/:site_id/ssh/:device_id?',
      name: 'site-ssh',
      component: Terminal,
      props: true
    },
    {
      path: '/site/:site_id/topology',
      name: 'site-topology',
      component: Topology,
      props: true
    },
    {
      path: '/site/:site_id/rf',
      name: 'site-rf',
      component: RFIntelligence,
      props: true
    },
    {
      path: '/site/:site_id/vault',
      name: 'site-vault',
      component: Vault,
      props: true
    },
    {
      path: '/site/:site_id/vpn',
      name: 'site-vpn',
      component: VPNMatrix,
      props: true
    },
    {
      path: '/site/:site_id/bandwidth',
      name: 'site-bandwidth',
      component: BandwidthSentry,
      props: true
    },
    {
      path: '/global/sentinel',
      name: 'global-sentinel',
      component: SecurityMatrix
    },
    {
      path: '/global/settings',
      name: 'global-settings',
      component: GlobalSettings
    }
  ]
})

// Navigation guard: require JWT for all non-public routes
router.beforeEach((to, _from, next) => {
  if (to.meta.public) {
    return next()
  }
  const token = localStorage.getItem('jwt_token')
  if (!token) {
    return next('/login')
  }
  next()
})

export default router
