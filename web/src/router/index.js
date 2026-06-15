import { createRouter, createWebHistory } from 'vue-router'
import GlobalDashboard from '../views/GlobalDashboard.vue'
import SiteDashboard from '../views/SiteDashboard.vue'
import ClientList from '../views/ClientList.vue'
import LogConsole from '../views/LogConsole.vue'
import WirelessManager from '../views/WirelessManager.vue'
import Terminal from '../views/Terminal.vue'
import Incidents from '../views/Incidents.vue'
import Topology from '../views/Topology.vue'
import EchoLocation from '../views/EchoLocation.vue'
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
import IdentityMatrix from '../views/IdentityMatrix.vue'
import EdgeNexus from '../views/EdgeNexus.vue'
import OmadaMigrator from '../views/OmadaMigrator.vue'
import AdvancedTelemetry from '../views/AdvancedTelemetry.vue'

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
      component: () => import('../views/SiteSettings.vue'),
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
      path: '/site/:site_id/echolocation',
      name: 'site-echolocation',
      component: EchoLocation,
      props: true
    },
    {
      path: '/site/:site_id/advanced-telemetry',
      name: 'site-advanced-telemetry',
      component: AdvancedTelemetry,
      props: true
    },
    {
      path: '/site/:site_id/rf',
      name: 'site-rf',
      component: RFIntelligence,
      props: true
    },
    {
      path: '/site/:site_id/flow-radar',
      name: 'site-flow-radar',
      component: () => import('../views/FlowRadar.vue'),
      props: true
    },
    {
      path: '/site/:site_id/vault',
      name: 'site-vault',
      component: Vault,
      props: true
    },
    {
      path: '/site/:site_id/threat-shield',
      name: 'site-threat-shield',
      component: () => import('../views/ThreatShield.vue'),
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
    },
    {
      path: '/global/identity',
      name: 'global-identity',
      component: IdentityMatrix
    },
    {
      path: '/global/panopticon',
      name: 'global-panopticon',
      component: () => import('../views/PanopticonView.vue')
    },
    {
      path: '/global/radius',
      name: 'global-radius',
      component: () => import('../views/RadiusMatrix.vue')
    },
    {
      path: '/site/:site_id/edge-nexus',
      name: 'site-edge-nexus',
      component: EdgeNexus,
      props: true
    },
    {
      path: '/site/:site_id/device/:device_id/uci',
      name: 'site-uci',
      component: () => import('../views/UciOps.vue'),
      props: true
    },
    {
      path: '/site/:site_id/device/:device_id/central-config',
      name: 'site-central-config-device',
      component: () => import('../views/CentralConfig.vue'),
      props: true
    },
    {
      path: '/site/:site_id/central-config',
      name: 'site-central-config',
      component: () => import('../views/CentralConfig.vue'),
      props: true
    },
    {
      path: '/site/:site_id/orchestrator',
      redirect: to => `/site/${to.params.site_id}/site-settings`
    },
    {
      path: '/site/:site_id/migration',
      name: 'site-omada-migrator',
      component: OmadaMigrator,
      props: true
    },
    {
      path: '/site/:site_id/site-settings',
      name: 'site-settings-unified',
      component: () => import('../views/SiteSettings.vue'),
      props: true
    },
    {
      path: '/landlord',
      name: 'landlord',
      component: () => import('../views/LandlordDashboard.vue')
    },
    {
      path: '/map',
      name: 'map',
      component: () => import('../views/MapView.vue')
    },
    {
      path: '/webhooks',
      name: 'webhooks',
      component: () => import('../views/WebhooksView.vue')
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
