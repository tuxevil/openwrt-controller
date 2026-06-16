// useTopology — encapsulates the v-network-graph configuration and
// the topology API load. Extracted from Topology.vue so the view is
// just a shell that mounts <v-network-graph> and renders the
// inspect sidebar.
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import * as vNG from 'v-network-graph'
import { ForceLayout } from 'v-network-graph/lib/force-layout'
import 'v-network-graph/lib/style.css'
import api from '../services/api'

export function useTopology(siteIdRef) {
  const router = useRouter()
  const nodes = ref({})
  const edges = ref({})
  const layouts = ref({ nodes: {} })
  const selectedNodes = ref([])
  const selectedEdges = ref([])

  const configs = reactive(
    vNG.defineConfigs({
      view: {
        layoutHandler: new ForceLayout(),
        autoPanAndZoomOnLoad: 'fit-content',
        panEnabled: true,
        zoomEnabled: true,
        minZoomLevel: 0.1,
        maxZoomLevel: 10,
      },
      node: {
        normal: {
          type: 'circle',
          radius: (node) => (node.type === 'router' ? 24 : 12),
          color: (node) => (node.has_alert ? '#ff0041' : (node.type === 'router' ? '#00ff41' : '#00bfff')),
          strokeWidth: 2,
          strokeColor: '#000000',
        },
        hover: {
          radius: (node) => (node.type === 'router' ? 26 : 14),
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
          color: (edge) => (edge.type === 'wired' ? '#00ff41' : '#00bfff'),
          dasharray: (edge) => (edge.type === 'wireless' ? '4 4' : '0'),
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

  async function load() {
    try {
      const res = await api.getSiteTopology(siteIdRef.value)
      const data = res.data?.data || { nodes: {}, edges: {} }
      nodes.value = data.nodes || {}
      edges.value = data.edges || {}
    } catch (err) {
      console.error('Failed to load topology', err)
    }
  }

  function goTerminal(mac) {
    router.push(`/site/${siteIdRef.value}/ssh/${mac}`)
  }

  onMounted(load)
  watch(siteIdRef, load)

  return { nodes, edges, layouts, configs, selectedNode, load, goTerminal }
}
