<script setup>
import { ref, watch, onMounted } from 'vue'
import { Line } from 'vue-chartjs'
import { Chart as ChartJS, CategoryScale, LinearScale, PointElement, LineElement, Title, Tooltip, Filler } from 'chart.js'
import api from '../services/api'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Title, Tooltip, Filler)

const props = defineProps({
  site_id: { type: String, required: true },
  metric: { type: String, required: true } // 'signal', 'traffic', 'cpu'
})

const chartData = ref({
  labels: [],
  datasets: [{ data: [] }]
})
const loading = ref(true)

const options = {
  responsive: true,
  maintainAspectRatio: false,
  interaction: {
    mode: 'index',
    intersect: false,
  },
  plugins: {
    legend: {
      display: false
    },
    tooltip: {
      backgroundColor: 'rgba(5, 5, 5, 0.9)',
      titleColor: '#00FF41',
      bodyColor: '#00FF41',
      borderColor: '#00FF41',
      borderWidth: 1,
      cornerRadius: 0,
      displayColors: false,
    }
  },
  scales: {
    x: {
      grid: {
        color: 'rgba(17, 17, 17, 0.8)',
      },
      ticks: {
        color: '#00FF41',
        maxTicksLimit: 8
      }
    },
    y: {
      grid: {
        color: 'rgba(17, 17, 17, 0.8)',
      },
      ticks: {
        color: '#00FF41'
      }
    }
  },
  elements: {
    point: {
      radius: 0,
      hitRadius: 10,
      hoverRadius: 4
    },
    line: {
      tension: 0.3
    }
  }
}

const fetchData = async () => {
  loading.value = true
  try {
    const res = await api.get(`/sites/${props.site_id}/history?metric=${props.metric}`)
    let data = []
    if (res.data && Array.isArray(res.data.data)) {
      data = res.data.data
    } else if (Array.isArray(res.data)) {
      data = res.data
    }
    
    chartData.value = {
      labels: data.map(d => {
        const date = new Date(d.time)
        return `${date.getHours().toString().padStart(2, '0')}:${date.getMinutes().toString().padStart(2, '0')}`
      }),
      datasets: [
        {
          label: props.metric.toUpperCase(),
          data: data.map(d => d.value),
          borderColor: '#00FF41',
          backgroundColor: 'rgba(0, 255, 65, 0.1)',
          borderWidth: 2,
          fill: true,
        }
      ]
    }
  } catch (err) {
    console.error('Failed to load chart data', err)
    chartData.value = { labels: [], datasets: [{ data: [] }] }
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})

watch(() => props.metric, () => {
  fetchData()
})
</script>

<template>
  <div class="h-64 relative w-full">
    <div v-if="loading" class="absolute inset-0 flex items-center justify-center text-neon-green font-mono glitch-anim z-10">
      > FETCHING_CHRONOS_DATA...
    </div>
    <Line v-if="!loading && chartData.labels.length > 0" :data="chartData" :options="options" />
    <div v-else-if="!loading" class="absolute inset-0 flex items-center justify-center text-neon-red font-mono z-10">
      > INSUFFICIENT_DATA_FOR_TIMELINE
    </div>
  </div>
</template>
