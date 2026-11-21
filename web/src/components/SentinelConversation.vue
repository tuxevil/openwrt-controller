<template>
  <section class="mb-6 rounded border border-purple-800 bg-slate-950 p-4 text-slate-100">
    <h2 class="text-lg font-semibold">Conversación con Sentinel</h2>
    <div class="mt-3">
      <button type="button" class="underline" :disabled="loadingReport" @click="loadReport">
        {{ loadingReport ? 'Consultando telemetría…' : 'Consultar informe de salud' }}
      </button>
      <p v-if="reportError" role="alert" class="text-red-300">{{ reportError }}</p>
      <div v-if="report" class="mt-2 rounded border border-slate-700 p-3">
        <p>Consulta realizada {{ new Date(report.observed_at).toLocaleString() }}</p>
        <p>{{ report.fresh }} de {{ report.devices }} nodos reportaron en los últimos dos minutos.
          {{ report.never_reported }} figuran sin fecha de último contacto.</p>
        <p>{{ report.open_incidents }} incidentes abiertos.
          En las últimas 24 horas se abrieron {{ report.opened_last_24_hours }}
          y se resolvieron {{ report.resolved_last_24_hours }}.</p>
      </div>
    </div>
    <div class="mt-3 flex flex-wrap items-center gap-2">
      <label for="sentinel-history">Conversaciones recientes</label>
      <select id="sentinel-history" :value="conversationID || ''" :disabled="busy"
        class="rounded border border-slate-600 bg-slate-900 p-2"
        @change="openConversation($event.target.value)">
        <option value="">Nueva conversación</option>
        <option v-for="item in conversations" :key="item.id" :value="item.id">{{ item.title }}</option>
      </select>
      <button type="button" :disabled="busy" class="underline" @click="refreshHistory">Actualizar historial</button>
    </div>
    <article v-for="message in messages" :key="message.id" class="mt-4 border-t border-slate-700 pt-3">
      <h3 class="font-semibold">{{ message.role === 'assistant' ? 'Sentinel' : 'Consulta' }}</h3>
      <p class="whitespace-pre-wrap">{{ message.content }}</p>
      <p v-if="message.metadata?.case_id" class="text-sm text-slate-300">Caso {{ message.metadata.case_id }}</p>
    </article>
    <form class="mt-3 grid gap-2" @submit.prevent="ask">
      <label for="sentinel-query">Consulta de infraestructura</label>
      <textarea id="sentinel-query" v-model.trim="query" maxlength="2000" required
        class="rounded border border-slate-600 bg-slate-900 p-2"
        placeholder="¿Cuándo reportó por última vez cada nodo?" />
      <button class="justify-self-start rounded bg-purple-800 px-4 py-2 disabled:opacity-50"
        :disabled="busy || !query">Enviar</button>
    </form>
    <p v-if="running" class="mt-3" role="status">Sentinel está investigando…</p>
    <p v-if="error" class="mt-3 text-red-300" role="alert">{{ error }}</p>
    <article v-for="turn in turns" :key="turn.id" class="mt-4 border-t border-slate-700 pt-3">
      <h3 class="font-semibold">{{ turn.query }}</h3>
      <p class="mt-2 whitespace-pre-wrap">{{ turn.answer }}</p>
      <p v-if="stopReasons[turn.evidence?.stop_reason]" class="text-sm text-slate-300">
        {{ stopReasons[turn.evidence.stop_reason] }}
      </p>
      <p v-if="turn.case_id" class="mt-2 text-sm text-slate-300">Caso {{ turn.case_id }}</p>
      <details v-if="turn.evidence" class="mt-2">
        <summary>Evidencia</summary>
        <pre class="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-words text-xs">{{ JSON.stringify(turn.evidence, null, 2) }}</pre>
      </details>
    </article>
  </section>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue';
import api from '../services/api';

const stopReasons = {
  timeout: 'Tiempo de investigación agotado.',
  canceled: 'Investigación cancelada.',
  model_error: 'Falló el turno del modelo.',
  context_error: 'No se pudo preparar el contexto del caso.',
  tool_limit: 'Resultado parcial. Se agotó el presupuesto de herramientas.',
  round_limit: 'Resultado parcial. Se alcanzó el límite de rondas.'
};

const query = ref('');
const turns = ref([]);
const running = ref(false);
const error = ref('');
const lifetime = new AbortController();
const requestOptions = { signal: lifetime.signal };
const conversationID = ref('');
const conversations = ref([]);
const messages = ref([]);
const loading = ref(false);
const busy = computed(() => running.value || loading.value);
const report = ref(null);
const reportError = ref('');
const loadingReport = ref(false);

async function loadReport() {
  loadingReport.value = true;
  reportError.value = '';
  try {
    const { data } = await api.client.get('/sentinel/health-report', requestOptions);
    report.value = data;
  } catch {
    if (!lifetime.signal.aborted) reportError.value = 'No se pudo actualizar el informe de salud.';
  } finally {
    loadingReport.value = false;
  }
}

let timer;
let finishDelay;

onUnmounted(() => {
  lifetime.abort();
  clearTimeout(timer);
  finishDelay?.();
});

async function refreshHistory() {
  loading.value = true;
  error.value = '';
  try {
    const { data } = await api.client.get('/sentinel/conversations', requestOptions);
    conversations.value = data.data || [];
  } catch {
    if (!lifetime.signal.aborted) error.value = 'No se pudo cargar el historial de Sentinel.';
  } finally {
    loading.value = false;
  }
}

async function openConversation(id) {
  loading.value = true;
  error.value = '';
  try {
    const conversation = id
      ? (await api.client.get(`/sentinel/conversations/${id}`, requestOptions)).data
      : null;
    conversationID.value = id;
    messages.value = conversation?.messages || [];
    turns.value = [];
  } catch {
    if (!lifetime.signal.aborted) error.value = 'No se pudo abrir la conversación.';
  } finally {
    loading.value = false;
  }
}

onMounted(refreshHistory);

async function ask() {
  if (!query.value || busy.value) return;
  running.value = true;
  error.value = '';
  const question = query.value;
  try {
    if (!conversationID.value) {
      const { data } = await api.client.post('/sentinel/conversations', {
        title: question.slice(0, 100)
      }, requestOptions);
      conversationID.value = data.id;
      conversations.value.unshift(data);
    }
    const { data: queued } = await api.client.post(
      `/sentinel/conversations/${conversationID.value}/messages?async=true`,
      { query: question }, requestOptions
    );
    if (query.value === question) query.value = '';

    for (let attempt = 0; attempt < 180; attempt++) {
      await new Promise(resolve => {
        finishDelay = resolve;
        timer = setTimeout(resolve, 2000);
      });
      if (lifetime.signal.aborted) return;
      const { data: run } = await api.client.get(`/sentinel/runs/${queued.id}`, requestOptions);
      if (run.status === 'COMPLETED') {
        turns.value.unshift({ ...run, query: question });
        return;
      }
      if (run.status !== 'QUEUED' && run.status !== 'RUNNING') {
        turns.value.unshift({ ...run, query: question });
        error.value = `La investigación terminó con estado ${run.status}. Ejecución ${queued.id}.`;
        return;
      }
    }
    error.value = `La ejecución ${queued.id} sigue pendiente en el controlador.`;
  } catch {
    if (!lifetime.signal.aborted) error.value = 'No se pudo completar la consulta a Sentinel.';
  } finally {
    running.value = false;
  }
}
</script>
