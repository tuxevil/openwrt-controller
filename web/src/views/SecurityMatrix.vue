<template>
  <div class="matrix-container">
    <div class="header">
      <h1 class="neon-purple">SENTINEL AI: <span class="neon-cyan">GLOBAL PULSE</span></h1>
      <p class="subtitle">Automated Multi-Device Anomaly Correlation</p>
    </div>

    <div class="manual-trigger-panel">
      <div class="trigger-controls">
        <label for="log-limit">Log Review Limit:</label>
        <input type="number" id="log-limit" v-model="triggerLimit" min="10" max="1000" class="neon-input" />
        <button @click="triggerManual" :disabled="triggering" class="action-btn neon-btn">
          <span v-if="triggering" class="internal-spinner"></span>
          <span v-else>Execute Sentinel AI</span>
        </button>
      </div>
      <p v-if="triggerError" class="error-msg">{{ triggerError }}</p>
    </div>

    <div class="operator-panel" v-if="operatorCases.length || proposals.length">
      <h2 class="neon-cyan">SENTINEL OPERATOR QUEUE</h2>
      <div v-for="item in operatorCases" :key="item.id" class="operator-case">
        <span class="severity-badge" :class="item.severity.toLowerCase()">{{ item.severity }}</span>
        <span class="operator-case-title">{{ item.title }}</span>
        <span class="operator-case-status">{{ item.status }}</span>
        <p>{{ item.summary }}</p>
      </div>
      <div v-for="proposal in proposals" :key="proposal.id" class="operator-proposal">
        <div><strong>ACTION PROPOSAL</strong> · {{ proposal.device_id }} · {{ proposal.config }}</div>
        <p>{{ proposal.summary }}</p>
        <button class="action-btn neon-btn" @click="approveProposal(proposal)">Approve</button>
        <button class="action-btn reject-btn" @click="rejectProposal(proposal)">Reject</button>
      </div>
    </div>

    <div class="notes-panel">
      <h2 class="neon-cyan">INFRASTRUCTURE NOTES</h2>
      <form class="notes-form" @submit.prevent="createNote">
        <input v-model="noteForm.site_id" placeholder="Site UUID (optional)" class="neon-input note-input" />
        <input v-model="noteForm.device_id" placeholder="Device ID (optional)" class="neon-input note-input" />
        <input v-model="noteForm.title" placeholder="Note title" class="neon-input note-input" required />
        <textarea v-model="noteForm.content" placeholder="Role, dependency, service expectation..." class="note-textarea" required></textarea>
        <button class="action-btn neon-btn" type="submit" :disabled="savingNote">{{ editingNoteId ? 'Update note' : 'Save note' }}</button>
        <button v-if="editingNoteId" class="note-delete" type="button" @click="cancelEdit">Cancel</button>
      </form>
      <div v-for="note in operatorNotes" :key="note.id" class="operator-note">
        <div><strong>{{ note.title }}</strong> <span class="operator-case-status">{{ note.site_id || note.device_id || 'fleet' }}</span></div>
        <p>{{ note.content }}</p>
        <button class="note-delete" @click="startEdit(note)">Edit</button>
        <button class="note-delete" @click="deleteNote(note)">Delete</button>
      </div>
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
          <div class="meta-row" v-if="insight.llm_model || insight.tokens_used">
            <span class="meta-tag" v-if="insight.llm_model">
              <span class="meta-icon">🤖</span> {{ insight.llm_model }}
            </span>
            <span class="meta-tag" v-if="insight.tokens_used > 0">
              <span class="meta-icon">⚡</span> {{ insight.tokens_used.toLocaleString() }} tokens
            </span>
          </div>
        </div>

        <div class="diagnosis-block">
          <div class="diagnosis-text" v-html="renderMarkdown(insight.diagnosis)"></div>
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
import { marked } from 'marked';
import DOMPurify from 'dompurify';
import api from '../services/api';

const renderMarkdown = (text) => {
  if (!text) return '';
  return DOMPurify.sanitize(marked.parse(text));
};

const insights = ref([]);
const loading = ref(true);
let pollInterval = null;

const triggerLimit = ref(100);
const triggering = ref(false);
const triggerError = ref('');
const operatorCases = ref([]);
const proposals = ref([]);
const operatorNotes = ref([]);
const savingNote = ref(false);
const editingNoteId = ref('');
const noteForm = ref({ site_id: '', device_id: '', title: '', content: '' });

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

const fetchOperatorState = async () => {
  try {
    const [casesResponse, proposalsResponse, notesResponse] = await Promise.all([
      api.client.get('/sentinel/cases?limit=20'),
      api.client.get('/sentinel/proposals?status=PENDING&limit=20'),
      api.client.get('/sentinel/notes?limit=20')
    ]);
    operatorCases.value = casesResponse.data.data || [];
    proposals.value = proposalsResponse.data.data || [];
    operatorNotes.value = notesResponse.data.data || [];
  } catch (e) {
    console.error('Sentinel operator state error:', e);
  }
};

const createNote = async () => {
  if (savingNote.value) return;
  savingNote.value = true;
  try {
    if (editingNoteId.value) {
      await api.client.patch(`/sentinel/notes/${editingNoteId.value}`, {
        title: noteForm.value.title,
        content: noteForm.value.content
      });
    } else {
      await api.client.post('/sentinel/notes', noteForm.value);
    }
    noteForm.value = { site_id: '', device_id: '', title: '', content: '' };
    editingNoteId.value = '';
    await fetchOperatorState();
  } catch (e) {
    triggerError.value = e.response?.data?.error?.message || e.message;
  } finally {
    savingNote.value = false;
  }
};

const startEdit = (note) => {
  editingNoteId.value = note.id;
  noteForm.value = {
    site_id: note.site_id || '',
    device_id: note.device_id || '',
    title: note.title,
    content: note.content
  };
};

const cancelEdit = () => {
  editingNoteId.value = '';
  noteForm.value = { site_id: '', device_id: '', title: '', content: '' };
};

const deleteNote = async (note) => {
  try {
    await api.client.delete(`/sentinel/notes/${note.id}`);
    await fetchOperatorState();
  } catch (e) {
    triggerError.value = e.response?.data?.error?.message || e.message;
  }
};

const approveProposal = async (proposal) => {
  try {
    await api.client.post(`/sentinel/proposals/${proposal.id}/approve`);
    await fetchOperatorState();
  } catch (e) {
    triggerError.value = e.response?.data?.error?.message || e.message;
  }
};

const rejectProposal = async (proposal) => {
  try {
    await api.client.post(`/sentinel/proposals/${proposal.id}/reject`, { reason: 'Rejected from Sentinel console' });
    await fetchOperatorState();
  } catch (e) {
    triggerError.value = e.response?.data?.error?.message || e.message;
  }
};

const triggerManual = async () => {
  if (triggering.value) return;
  triggering.value = true;
  triggerError.value = '';
  
  try {
    const token = localStorage.getItem('jwt_token');
    const res = await fetch('/api/global/sentinel/trigger', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ limit: triggerLimit.value })
    });
    
    if (!res.ok) {
      const errorText = await res.text();
      throw new Error(errorText || 'Manual execution failed');
    }
    
    // Refresh insights table to show new diagnosis
    await fetchInsights();
  } catch (e) {
    console.error('Manual trigger error:', e);
    triggerError.value = e.message;
  } finally {
    triggering.value = false;
  }
};

onMounted(() => {
  fetchInsights();
  fetchOperatorState();
  pollInterval = setInterval(() => {
    fetchInsights();
    fetchOperatorState();
  }, 15000);
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
  width: 95%;
  max-width: 95%;
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
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.meta-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-top: 0.25rem;
}

.meta-tag {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  background: rgba(0, 255, 255, 0.05);
  border: 1px solid rgba(0, 255, 255, 0.25);
  color: #7af;
  padding: 0.2rem 0.6rem;
  border-radius: 4px;
  font-size: 0.8rem;
  font-family: 'Courier New', monospace;
}

.meta-icon {
  font-size: 0.85rem;
}

.severity-badge {
  padding: 0.25rem 0.75rem;
  border-radius: 4px;
  font-weight: bold;
  font-size: 0.85rem;
  display: inline-block;
  align-self: flex-start;
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
  color: #eee;
}

/* Markdown prose styles scoped within the diagnosis block */
.diagnosis-block :deep(h1),
.diagnosis-block :deep(h2),
.diagnosis-block :deep(h3),
.diagnosis-block :deep(h4) {
  color: #bc13fe;
  margin: 1rem 0 0.5rem;
  text-shadow: 0 0 8px rgba(188, 19, 254, 0.4);
}
.diagnosis-block :deep(p) {
  margin: 0.5rem 0;
  color: #eee;
}
.diagnosis-block :deep(ul),
.diagnosis-block :deep(ol) {
  padding-left: 1.5rem;
  margin: 0.5rem 0;
  color: #eee;
}
.diagnosis-block :deep(li) {
  margin: 0.25rem 0;
}
.diagnosis-block :deep(strong) {
  color: #00ffff;
}
.diagnosis-block :deep(em) {
  color: #e0b0ff;
}
.diagnosis-block :deep(code) {
  background: rgba(0,255,255,0.1);
  color: #00ffff;
  padding: 0.1rem 0.35rem;
  border-radius: 3px;
  font-family: 'Courier New', monospace;
  font-size: 0.9em;
}
.diagnosis-block :deep(pre) {
  background: rgba(0,0,0,0.5);
  border: 1px solid rgba(0,255,255,0.2);
  border-radius: 4px;
  padding: 0.75rem;
  overflow-x: auto;
  margin: 0.75rem 0;
}
.diagnosis-block :deep(pre code) {
  background: none;
  padding: 0;
}
.diagnosis-block :deep(blockquote) {
  border-left: 3px solid #bc13fe;
  margin: 0.5rem 0;
  padding-left: 1rem;
  color: #aaa;
}
.diagnosis-block :deep(hr) {
  border: none;
  border-top: 1px solid rgba(188, 19, 254, 0.3);
  margin: 1rem 0;
}
.diagnosis-block :deep(table) {
  border-collapse: collapse;
  width: 100%;
  margin: 0.5rem 0;
}
.diagnosis-block :deep(th) {
  background: rgba(188, 19, 254, 0.2);
  color: #00ffff;
  padding: 0.4rem 0.75rem;
  border: 1px solid rgba(188, 19, 254, 0.4);
}
.diagnosis-block :deep(td) {
  padding: 0.4rem 0.75rem;
  border: 1px solid rgba(188, 19, 254, 0.2);
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

.manual-trigger-panel {
  width: 95%;
  max-width: 95%;
  margin: 0 auto 3rem auto;
  padding: 1.5rem;
  background: rgba(188, 19, 254, 0.05);
  border: 1px solid rgba(188, 19, 254, 0.4);
  border-radius: 8px;
  text-align: center;
}

.operator-panel {
  width: 95%;
  max-width: 95%;
  margin: 0 auto 3rem;
  padding: 1.5rem;
  background: rgba(0, 255, 255, 0.04);
  border: 1px solid rgba(0, 255, 255, 0.35);
  border-radius: 8px;
}

.operator-panel h2 {
  margin: 0 0 1rem;
  font-size: 1rem;
  letter-spacing: 0.18em;
}

.operator-case, .operator-proposal {
  padding: 0.9rem;
  margin-top: 0.75rem;
  border: 1px solid rgba(188, 19, 254, 0.3);
  background: rgba(10, 10, 25, 0.7);
}

.operator-case-title, .operator-case-status {
  margin-left: 0.75rem;
  color: #fff;
}

.operator-case-status {
  color: #888;
  font-size: 0.8rem;
}

.operator-case p, .operator-proposal p {
  margin: 0.6rem 0;
  color: #bbb;
}

.reject-btn {
  color: #ff5c7c;
  border-color: #ff5c7c;
  margin-left: 0.5rem;
}

.notes-panel {
  width: 95%;
  max-width: 95%;
  margin: 0 auto 3rem;
  padding: 1.5rem;
  border: 1px solid rgba(57, 255, 20, 0.3);
  background: rgba(57, 255, 20, 0.03);
  border-radius: 8px;
}

.notes-panel h2 {
  margin: 0 0 1rem;
  font-size: 1rem;
  letter-spacing: 0.18em;
}

.notes-form {
  display: flex;
  gap: 0.6rem;
  flex-wrap: wrap;
  align-items: center;
}

.note-input {
  width: 180px;
  text-align: left;
}

.note-textarea {
  min-width: 280px;
  min-height: 70px;
  flex: 1;
  color: #fff;
  background: rgba(0, 0, 0, 0.5);
  border: 1px solid rgba(57, 255, 20, 0.4);
  padding: 0.5rem;
  border-radius: 4px;
}

.operator-note {
  border-top: 1px solid rgba(57, 255, 20, 0.2);
  margin-top: 1rem;
  padding-top: 0.8rem;
  color: #bbb;
}

.operator-note p {
  margin: 0.4rem 0;
  white-space: pre-wrap;
}

.note-delete {
  color: #ff5c7c;
  background: transparent;
  border: 0;
  cursor: pointer;
  padding: 0;
}

.trigger-controls {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 1rem;
  flex-wrap: wrap;
}

.trigger-controls label {
  color: #00ffff;
  font-weight: bold;
}

.neon-input {
  background: rgba(0, 0, 0, 0.5);
  border: 1px solid #bc13fe;
  color: #e0b0ff;
  padding: 0.5rem;
  border-radius: 4px;
  width: 100px;
  text-align: center;
}

.neon-input:focus {
  outline: none;
  box-shadow: 0 0 10px rgba(188, 19, 254, 0.5);
}

.action-btn {
  padding: 0.5rem 1.5rem;
  background: transparent;
  color: #00ffff;
  border: 1px solid #00ffff;
  border-radius: 4px;
  cursor: pointer;
  font-weight: bold;
  transition: all 0.2s;
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 180px;
}

.action-btn:hover:not(:disabled) {
  background: rgba(0, 255, 255, 0.1);
  box-shadow: 0 0 10px rgba(0, 255, 255, 0.5);
}

.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.error-msg {
  color: #ff0032;
  margin-top: 1rem;
  font-size: 0.9rem;
}

.internal-spinner {
  width: 16px;
  height: 16px;
  border: 2px solid rgba(0, 255, 255, 0.3);
  border-top-color: #00ffff;
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

</style>
