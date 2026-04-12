<template>
  <div class="matrix-container">
    <div class="header">
      <h1 class="neon-purple">SENTINEL AI: <span class="neon-cyan">GLOBAL PULSE</span></h1>
      <p class="subtitle">Automated Multi-Device Anomaly Correlation</p>
    </div>

    <div v-if="loading" class="loading">
      <div class="spinner"></div>
      <p>Establishing neural link...</p>
    </div>

    <div v-else-if="insights.length === 0" class="empty">
      <p>No critical anomalies detected in the fleet.</p>
    </div>

    <div v-else class="timeline">
      <div v-for="insight in insights" :key="insight.id" class="insight-card" :class="'severity-' + insight.severity.toLowerCase()">
        <div class="card-header">
          <div class="correlation-badge neon-cyan-bg">{{ insight.correlation_id }}</div>
          <div class="timestamp">{{ new Date(insight.created_at).toLocaleString() }}</div>
        </div>
        
        <div class="metrics">
          <span class="severity-badge" :class="insight.severity.toLowerCase()">{{ insight.severity }} RISK</span>
        </div>

        <div class="diagnosis-block">
          <p class="diagnosis-text">{{ insight.diagnosis }}</p>
        </div>

        <div class="involved-devices">
          <h4>Correlated Vectors:</h4>
          <div class="device-tags">
            <span v-for="dev in insight.involved_devices" :key="dev" class="device-tag">
              <span class="connector"></span> {{ dev }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue';

const insights = ref([]);
const loading = ref(true);
let pollInterval = null;

const fetchInsights = async () => {
  try {
    const token = localStorage.getItem('jwt_token');
    const res = await fetch('/api/global/sentinel', {
      headers: { 'Authorization': `Bearer ${token}` }
    });
    if (!res.ok) throw new Error('Failed to fetch AI insights');
    const data = await res.json();
    if (data.status === 'success') {
      insights.value = data.insights || [];
    }
  } catch (e) {
    console.error('Sentinel fetch error:', e);
  } finally {
    loading.value = false;
  }
};

onMounted(() => {
  fetchInsights();
  pollInterval = setInterval(fetchInsights, 15000);
});

onUnmounted(() => {
  if (pollInterval) clearInterval(pollInterval);
});
</script>

<style scoped>
.matrix-container {
  padding: 2rem;
  background-color: #050510;
  min-height: 100vh;
  color: #fff;
  font-family: 'Courier New', Courier, monospace;
}

.header {
  text-align: center;
  margin-bottom: 3rem;
  border-bottom: 1px solid rgba(188, 19, 254, 0.3);
  padding-bottom: 2rem;
}

h1 {
  font-size: 2.5rem;
  margin: 0;
  letter-spacing: 2px;
}

.neon-purple {
  color: #bc13fe;
  text-shadow: 0 0 10px #bc13fe, 0 0 20px #bc13fe;
}

.neon-cyan {
  color: #00ffff;
  text-shadow: 0 0 10px #00ffff, 0 0 20px #00ffff;
}

.neon-cyan-bg {
  background-color: rgba(0, 255, 255, 0.1);
  border: 1px solid #00ffff;
  color: #00ffff;
  box-shadow: 0 0 8px rgba(0, 255, 255, 0.5);
}

.subtitle {
  color: #888;
  margin-top: 0.5rem;
  font-size: 1.1rem;
}

.loading, .empty {
  text-align: center;
  color: #00ffff;
  margin-top: 5rem;
  font-size: 1.2rem;
}

.spinner {
  width: 40px;
  height: 40px;
  border: 3px solid rgba(0, 255, 255, 0.3);
  border-top-color: #00ffff;
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin: 0 auto 1rem;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.timeline {
  display: flex;
  flex-direction: column;
  gap: 2rem;
  max-width: 900px;
  margin: 0 auto;
  position: relative;
}

.timeline::before {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  left: 20px;
  width: 2px;
  background: linear-gradient(to bottom, #bc13fe, #00ffff);
  opacity: 0.5;
}

.insight-card {
  background: rgba(10, 10, 25, 0.8);
  border: 1px solid rgba(188, 19, 254, 0.3);
  border-radius: 8px;
  padding: 1.5rem;
  margin-left: 50px;
  position: relative;
  transition: transform 0.2s, box-shadow 0.2s;
}

.insight-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 5px 15px rgba(188, 19, 254, 0.2);
}

.insight-card::before {
  content: '';
  position: absolute;
  left: -35px;
  top: 20px;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: #bc13fe;
  box-shadow: 0 0 10px #bc13fe;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.correlation-badge {
  padding: 0.25rem 0.75rem;
  border-radius: 4px;
  font-size: 0.85rem;
  font-weight: bold;
}

.timestamp {
  color: #aaa;
  font-size: 0.9rem;
}

.metrics {
  margin-bottom: 1rem;
}

.severity-badge {
  padding: 0.25rem 0.75rem;
  border-radius: 4px;
  font-weight: bold;
  font-size: 0.85rem;
  display: inline-block;
}

.severity-badge.critical {
  background: rgba(255, 0, 50, 0.1);
  color: #ff0032;
  border: 1px solid #ff0032;
  box-shadow: 0 0 10px rgba(255, 0, 50, 0.5);
}

.severity-badge.high {
  background: rgba(255, 100, 0, 0.1);
  color: #ff6400;
  border: 1px solid #ff6400;
}

.severity-badge.medium {
  background: rgba(255, 200, 0, 0.1);
  color: #ffc800;
  border: 1px solid #ffc800;
}

.severity-badge.low {
  background: rgba(0, 200, 255, 0.1);
  color: #00c8ff;
  border: 1px solid #00c8ff;
}

.diagnosis-block {
  background: rgba(0, 0, 0, 0.4);
  padding: 1rem;
  border-left: 3px solid #bc13fe;
  margin-bottom: 1.5rem;
}

.diagnosis-text {
  margin: 0;
  line-height: 1.6;
  white-space: pre-wrap;
  color: #eee;
}

.involved-devices h4 {
  margin: 0 0 0.5rem 0;
  color: #00ffff;
  font-size: 0.9rem;
  text-transform: uppercase;
}

.device-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.device-tag {
  background: rgba(188, 19, 254, 0.1);
  border: 1px solid rgba(188, 19, 254, 0.5);
  color: #e0b0ff;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  font-size: 0.85rem;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.connector {
  width: 6px;
  height: 6px;
  background: #00ffff;
  border-radius: 50%;
  box-shadow: 0 0 5px #00ffff;
}
</style>
