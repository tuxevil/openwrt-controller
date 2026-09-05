// useTopology — encapsulates the v-network-graph configuration and
// the hierarchical / tree L2 topology layout.
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import * as vNG from 'v-network-graph'
import { ForceLayout } from 'v-network-graph/lib/force-layout'
import 'v-network-graph/lib/style.css'
import api from '../services/api'

export function computeHierarchicalLayout(nodesMap, edgesMap) {
  const positions = {}

  // Level 0: WAN Root (Starlink Satellite)
  const wanId = Object.keys(nodesMap).find((id) => nodesMap[id].type === 'wan')
  if (wanId) positions[wanId] = { x: 0, y: -260 }

  // Level 1: Core Gateway Router (mr8300)
  const gatewayId = 'e8:9f:80:14:69:c5'
  positions[gatewayId] = { x: 0, y: -80 }

  // Level 2: Secondary Access Points & Distribution nodes
  const apNodes = Object.keys(nodesMap).filter(
    (id) => id !== 'starlink' && id !== gatewayId && ['router', 'gateway', 'ap'].includes(nodesMap[id].type) && nodesMap[id].type !== 'gateway'
  )

  const apSpacing = 420
  const apStartX = -((apNodes.length - 1) * apSpacing) / 2
  apNodes.forEach((apId, idx) => {
    positions[apId] = { x: apStartX + idx * apSpacing, y: 130 }
  })

  // Level 3: Clients grouped directly under their AP / Gateway
  const childrenOf = {}
  childrenOf[gatewayId] = []
  apNodes.forEach((apId) => {
    childrenOf[apId] = []
  })

  Object.values(edgesMap).forEach((edge) => {
    if (edge.type === 'wan') return
    const targetNode = nodesMap[edge.target]
    if (targetNode && targetNode.type === 'client') {
      const parentId = edge.source
      if (childrenOf[parentId] && !childrenOf[parentId].includes(edge.target)) {
        childrenOf[parentId].push(edge.target)
      }
    }
  })

  // Distribute Gateway direct clients (arranged in clean rows below Gateway)
  const gwClients = childrenOf[gatewayId] || []
  const gwCols = 6
  const colSpacing = 85
  const rowSpacing = 75
  gwClients.forEach((clientId, idx) => {
    const col = idx % gwCols
    const row = Math.floor(idx / gwCols)
    const currentCols = Math.min(gwClients.length, gwCols)
    const startX = -((currentCols - 1) * colSpacing) / 2
    positions[clientId] = { x: startX + col * colSpacing, y: 300 + row * rowSpacing }
  })

  // Distribute Secondary APs clients below each respective AP
  apNodes.forEach((apId) => {
    const apPos = positions[apId] || { x: 0, y: 130 }
    const clients = childrenOf[apId] || []
    const spacing = 80
    const startX = apPos.x - ((clients.length - 1) * spacing) / 2
    clients.forEach((clientId, idx) => {
      positions[clientId] = { x: startX + idx * spacing, y: 300 }
    })
  })

  // Any remaining unplaced nodes
  Object.keys(nodesMap).forEach((id, idx) => {
    if (!positions[id]) {
      positions[id] = { x: (idx % 6) * 90 - 250, y: 500 }
    }
  })

  return positions
}

export function useTopology(siteId) {
  const router = useRouter()
  const nodes = ref({})
  const edges = ref({})
  const layouts = ref({ nodes: {} })
  const selectedNodes = ref([])
  const selectedEdges = ref([])
  const layoutMode = ref('tree') // 'tree' (default Omada-style) or 'force' (d3 radar)

  const configs = reactive(
    vNG.defineConfigs({
      view: {
        layoutHandler: undefined, // undefined uses static hierarchical coordinates
        autoPanAndZoomOnLoad: 'fit-content',
        panEnabled: true,
        zoomEnabled: true,
        minZoomLevel: 0.1,
        maxZoomLevel: 10,
      },
      node: {
        normal: {
          type: 'circle',
          radius: (node) =>
            node.type === 'wan' || node.id === 'starlink'
              ? 28
              : node.type === 'gateway' || node.id === 'e8:9f:80:14:69:c5'
              ? 24
              : node.type === 'ap' || node.type === 'router'
              ? 19
              : 11,
          color: (node) => {
            if (node.has_alert) return '#ff0041'
            if (node.type === 'wan' || node.id === 'starlink') return '#ffd700'
            if (node.type === 'gateway' || node.id === 'e8:9f:80:14:69:c5') return '#00ff41'
            if (node.type === 'ap' || node.type === 'router' || node.id === '78:8a:20:2c:c1:12' || node.id === '04:a1:51:96:a6:4d') return '#ffffff'
            return '#00ffff'
          },
          strokeWidth: 2,
          strokeColor: '#000000',
        },
        hover: {
          radius: (node) => (node.type === 'wan' || node.id === 'starlink' ? 30 : node.type === 'gateway' || node.type === 'router' || node.type === 'ap' ? 26 : 13),
          color: '#ffffff',
        },
        label: {
          visible: true,
          fontFamily: 'monospace',
          fontSize: 10,
          color: '#ffffff',
          margin: 4,
        },
      },
      edge: {
        normal: {
          width: 2,
          color: (edge) =>
            edge.type === 'wan' ? '#ffd700' : edge.type === 'wired' ? '#00ff41' : '#b026ff',
          dasharray: (edge) => (edge.type === 'wan' ? '6 3' : edge.type === 'wireless' ? '4 4' : '0'),
          animate: true,
          animationSpeed: 50,
        },
        hover: {
          width: 3,
          color: '#ffffff',
        },
      },
    }),
  )

  const selectedNode = computed(() =>
    selectedNodes.value.length === 1 ? nodes.value[selectedNodes.value[0]] : null,
  )

  function applyLayout() {
    if (layoutMode.value === 'tree') {
      configs.view.layoutHandler = undefined
      layouts.value = { nodes: computeHierarchicalLayout(nodes.value, edges.value) }
    } else {
      configs.view.layoutHandler = new ForceLayout()
    }
  }

  function setLayoutMode(mode) {
    layoutMode.value = mode
    applyLayout()
  }

  async function load() {
    try {
      const res = await api.getSiteTopology(siteId)
      const data = res.data?.data || { nodes: {}, edges: {} }
      nodes.value = data.nodes || {}
      edges.value = data.edges || {}
      applyLayout()
    } catch (err) {
      console.error('Failed to load topology', err)
    }
  }

  function goTerminal(mac) {
    router.push(`/site/${siteId}/ssh/${mac}`)
  }

  onMounted(load)
  watch(() => siteId, load)

  return {
    nodes,
    edges,
    layouts,
    configs,
    selectedNode,
    layoutMode,
    setLayoutMode,
    load,
    goTerminal,
  }
}
