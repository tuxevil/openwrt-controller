<template>
  <div class="vpn-matrix">
    <div class="header-panel">
      <h1>[SECURE_TUNNEL] MATRIX</h1>
      <p class="subtitle">WireGuard Overlay Network</p>
    </div>

    <!-- Endpoint configuration -->
    <div class="endpoint-config panel">
      <h3>MASTER ENDPOINT</h3>
      <div class="input-group">
        <input 
          v-model="endpoint" 
          type="text" 
          placeholder="Ex: 203.0.113.10:51820"
          class="vantablack-input" 
        />
        <button @click="saveEndpoint" class="btn-cobalt">SAVE</button>
      </div>
      <div class="key-display" v-if="pubKey">
        <span class="label">Controller PubKey:</span>
        <code>{{ pubKey }}</code>
      </div>
    </div>

    <!-- Radar / Peer List -->
    <div class="radar-container panel">
      <h3>ACTIVE PEERS</h3>
      <div class="grid-display">
        <div 
          v-for="peer in peers" 
          :key="peer.id" 
          class="peer-card"
          :class="{'online': peer.status === 'ONLINE'}"
        >
          <div class="peer-header">
            <span class="status-indicator"></span>
            <span class="peer-name">{{ peer.name || peer.id }}</span>
          </div>
          <div class="peer-details">
            <p><strong>WG IP:</strong> {{ peer.wg_ip || 'PENDING' }}</p>
            <p class="key-truncate" :title="peer.wg_pubkey"><strong>PUB:</strong> {{ peer.wg_pubkey || 'WAITING GENERATION' }}</p>
          </div>
          <div class="peer-actions">
             <button 
                class="btn-cobalt block-btn" 
                :disabled="!peer.wg_ip"
                @click="openLuci(peer.wg_ip)"
             >
               OPEN LUCI
             </button>
          </div>
        </div>
        <div v-if="peers.length === 0" class="empty-state">
           NO PEERS DETECTED IN OVERLAY
        </div>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'VPNMatrix',
  props: ['site_id'],
  data() {
    return {
      endpoint: '',
      pubKey: '',
      peers: [],
      pollInterval: null
    }
  },
  async mounted() {
    this.refreshData();
    this.pollInterval = setInterval(this.refreshData, 10000);
  },
  beforeUnmount() {
    clearInterval(this.pollInterval);
  },
  methods: {
    async refreshData() {
      try {
        const configRes = await fetch(`/api/sites/${this.site_id}/vpn`, {
          headers: { 'Authorization': `Bearer ${localStorage.getItem('jwt_token')}` }
        });
        if (configRes.ok) {
          const config = await configRes.json();
          this.endpoint = config.endpoint || '';
          this.pubKey = config.pubkey || '';
        }

        const peersRes = await fetch(`/api/sites/${this.site_id}/vpn/peers`, {
          headers: { 'Authorization': `Bearer ${localStorage.getItem('jwt_token')}` }
        });
        if (peersRes.ok) {
          this.peers = await peersRes.json();
        }
      } catch (e) {
        console.error("VPN Matrix Error", e);
      }
    },
    async saveEndpoint() {
      try {
        await fetch(`/api/sites/${this.site_id}/vpn/endpoint`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${localStorage.getItem('jwt_token')}`
          },
          body: JSON.stringify({ endpoint: this.endpoint })
        });
        alert('Endpoint updated. Devices will configure upon next check-in.');
        this.refreshData();
      } catch (e) {
        alert('Failed to save endpoint');
      }
    },
    openLuci(ip) {
      if (!ip) return;
      window.open(`http://${ip}`, '_blank');
    }
  }
}
</script>

<style scoped>
/* Vantablack theme with Cobalt Blue accents */
.vpn-matrix {
  background-color: #050505;
  color: #e0e0e0;
  padding: 2rem;
  font-family: 'Space Mono', 'Courier New', Courier, monospace;
}

.header-panel h1 {
  color: #0047AB; /* Cobalt Blue */
  margin: 0 0 0.5rem 0;
  letter-spacing: 2px;
  text-shadow: 0 0 10px rgba(0, 71, 171, 0.5);
}

.subtitle {
  color: #888;
  font-size: 0.9rem;
  letter-spacing: 1px;
}

.panel {
  background: #0f0f0f;
  border: 1px solid #1a1a1a;
  border-radius: 8px;
  padding: 1.5rem;
  margin-top: 1.5rem;
  box-shadow: inset 0 0 20px rgba(0, 0, 0, 0.8);
}

h3 {
  color: #0047AB;
  border-bottom: 1px solid #1a1a1a;
  padding-bottom: 0.5rem;
  margin-top: 0;
  font-size: 1.1rem;
}

.input-group {
  display: flex;
  gap: 1rem;
  margin-bottom: 1rem;
}

.vantablack-input {
  background-color: #000;
  border: 1px solid #333;
  color: #0047AB;
  padding: 0.8rem 1rem;
  border-radius: 4px;
  flex: 1;
  font-family: inherit;
  transition: all 0.3s ease;
}

.vantablack-input:focus {
  outline: none;
  border-color: #0047AB;
  box-shadow: 0 0 8px rgba(0, 71, 171, 0.5);
}

.btn-cobalt {
  background-color: transparent;
  color: #0047AB;
  border: 1px solid #0047AB;
  padding: 0.8rem 2rem;
  border-radius: 4px;
  cursor: pointer;
  font-weight: bold;
  letter-spacing: 1px;
  transition: all 0.3s ease;
  min-width: 120px;
}

.btn-cobalt:hover:not(:disabled) {
  background-color: #0047AB;
  color: #fff;
  box-shadow: 0 0 15px rgba(0, 71, 171, 0.6);
}

.btn-cobalt:disabled {
  border-color: #333;
  color: #555;
  cursor: not-allowed;
}

.block-btn {
  width: 100%;
}

.key-display {
  background: #000;
  padding: 1rem;
  border-radius: 4px;
  border-left: 3px solid #0047AB;
  display: flex;
  align-items: center;
  gap: 1rem;
  overflow: hidden;
}

.key-display code {
  color: #00ffcc;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.grid-display {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 1.5rem;
  margin-top: 1rem;
}

.peer-card {
  background-color: #0a0a0a;
  border: 1px solid #222;
  border-radius: 6px;
  padding: 1rem;
  transition: border-color 0.3s;
  position: relative;
}

.peer-card.online {
  border-color: rgba(0, 71, 171, 0.5);
}

.peer-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 1rem;
  padding-bottom: 0.5rem;
  border-bottom: 1px dashed #222;
}

.status-indicator {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background-color: #333;
}

.peer-card.online .status-indicator {
  background-color: #00ff00;
  box-shadow: 0 0 8px #00ff00;
}

.peer-name {
  font-weight: bold;
  color: #ccc;
}

.peer-details p {
  margin: 0.5rem 0;
  font-size: 0.85rem;
}

.key-truncate {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.peer-actions {
  margin-top: 1.5rem;
}

.empty-state {
  grid-column: 1 / -1;
  text-align: center;
  padding: 3rem;
  color: #444;
  font-style: italic;
  border: 1px dashed #222;
  border-radius: 8px;
}
</style>
