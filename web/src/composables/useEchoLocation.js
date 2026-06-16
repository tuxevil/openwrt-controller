// useEchoLocation — encapsulates the d3-force simulation and the
// L2 topology API load. Extracted from EchoLocation.vue so the
// view is just a shell with the SVG container and the inspect
// sidebar.
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import * as d3 from 'd3'
import api from '../services/api'

const COLORS = {
  gateway: '#00ff41',
  ap: '#ffffff',
  client: '#00ffff',
  alert: '#ff003c',
  edgeWired: '#00ffff',
  edgeWireless: '#b026ff',
}

const nodeColor = (d) => {
  if (d.has_alert) return COLORS.alert
  if (d.type === 'gateway') return COLORS.gateway
  if (d.type === 'ap') return COLORS.ap
  return COLORS.client
}

const nodeRadius = (d) => {
  if (d.type === 'gateway') return 25
  if (d.type === 'ap') return 18
  return 10
}

const edgeColor = (d) => (d.type === 'wired' ? COLORS.edgeWired : COLORS.edgeWireless)

export function useEchoLocation(siteId) {
  const router = useRouter()
  const svgContainer = ref(null)
  const selectedNode = ref(null)
  let simulation = null

  async function load() {
    try {
      const res = await api.getSiteEchoLocation(siteId)
      const data = res.data?.data || { nodes: [], links: [] }
      render(data)
    } catch (err) {
      console.error('Failed to load EchoLocation graph', err)
    }
  }

  function render(graphData) {
    if (!svgContainer.value) return
    const width = svgContainer.value.clientWidth
    const height = svgContainer.value.clientHeight

    d3.select(svgContainer.value).selectAll('*').remove()

    const svg = d3.select(svgContainer.value)
      .append('svg')
      .attr('width', width)
      .attr('height', height)
      .call(d3.zoom().scaleExtent([0.1, 8]).on('zoom', (event) => g.attr('transform', event.transform)))

    const g = svg.append('g')

    simulation = d3.forceSimulation(graphData.nodes)
      .force('link', d3.forceLink(graphData.links).id((d) => d.id).distance(100))
      .force('charge', d3.forceManyBody().strength(-300))
      .force('center', d3.forceCenter(width / 2, height / 2))

    g.append('g')
      .attr('stroke-opacity', 0.8)
      .selectAll('line')
      .data(graphData.links)
      .join('line')
      .attr('stroke', edgeColor)
      .attr('stroke-dasharray', (d) => (d.type === 'wireless' ? '4 4' : 'none'))
      .attr('stroke-width', 2)

    const node = g.append('g')
      .selectAll('g')
      .data(graphData.nodes)
      .join('g')
      .attr('cursor', 'pointer')
      .call(d3.drag()
        .on('start', (event, d) => {
          if (!event.active) simulation.alphaTarget(0.3).restart()
          d.fx = d.x
          d.fy = d.y
        })
        .on('drag', (event, d) => {
          d.fx = event.x
          d.fy = event.y
        })
        .on('end', (event, d) => {
          if (!event.active) simulation.alphaTarget(0)
          d.fx = null
          d.fy = null
        }))
      .on('click', (event, d) => {
        selectedNode.value = d
      })

    node.append('circle')
      .attr('r', nodeRadius)
      .attr('fill', '#000')
      .attr('stroke', nodeColor)
      .attr('stroke-width', 3)

    node.append('text')
      .attr('dx', 15)
      .attr('dy', 4)
      .text((d) => d.name)
      .attr('font-family', 'monospace')
      .attr('font-size', '10px')
      .attr('fill', '#fff')

    simulation.on('tick', () => {
      g.selectAll('line')
        .attr('x1', (d) => d.source.x)
        .attr('y1', (d) => d.source.y)
        .attr('x2', (d) => d.target.x)
        .attr('y2', (d) => d.target.y)
      node.attr('transform', (d) => `translate(${d.x},${d.y})`)
    })
  }

  function goTerminal(mac) {
    router.push(`/site/${siteId}/ssh/${mac}`)
  }

  onMounted(() => {
    load()
    window.addEventListener('resize', load)
  })
  watch(() => siteId, load)
  onUnmounted(() => {
    if (simulation) simulation.stop()
    window.removeEventListener('resize', load)
  })

  return { svgContainer, selectedNode, load, goTerminal }
}
