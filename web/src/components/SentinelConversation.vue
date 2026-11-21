<template>
  <section class="mb-6 rounded border border-purple-800 bg-slate-950 p-4 text-slate-100">
    <h2 class="text-lg font-semibold">Conversación con Sentinel</h2>
    <form class="mt-3 grid gap-2" @submit.prevent="ask">
      <label for="sentinel-query">Consulta de infraestructura</label>
      <textarea id="sentinel-query" v-model.trim="query" maxlength="2000" required
        class="rounded border border-slate-600 bg-slate-900 p-2"
        placeholder="¿Cuándo reportó por última vez cada nodo?" />
      <button class="justify-self-start rounded bg-purple-800 px-4 py-2 disabled:opacity-50"
        :disabled="running || !query">Enviar</button>
    </form>
    <p v-if="running" class="mt-3" role="status">Sentinel está investigando…</p>
    <p v-if="error" class="mt-3 text-red-300" role="alert">{{ error }}</p>
    <article v-for="turn in turns" :key="turn.id" class="mt-4 border-t border-slate-700 pt-3">
      <h3 class="font-semibold">{{ turn.query }}</h3>
      <p class="mt-2 whitespace-pre-wrap">{{ turn.answer }}</p>
      <p v-if="turn.case_id" class="mt-2 text-sm text-slate-300">Caso {{ turn.case_id }}</p>
      <details v-if="turn.evidence" class="mt-2">
        <summary>Evidencia</summary>
        <pre class="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-words text-xs">{{ JSON.stringify(turn.evidence, null, 2) }}</pre>
      </details>
    </article>
  </section>
</template>

<script setup>
import { onUnmounted, ref } from 'vue';
import api from '../services/api';

const query = ref('');
const turns = ref([]);
const running = ref(false);
const error = ref('');
const lifetime = new AbortController();
const requestOptions = { signal: lifetime.signal };
let conversationID;
let timer;
let finishDelay;

onUnmounted(() => {
  lifetime.abort();
  clearTimeout(timer);
  finishDelay?.();
});

async function ask() {
  if (!query.value || running.value) return;
  running.value = true;
  error.value = '';
  const question = query.value;
  try {
    if (!conversationID) {
      const { data } = await api.client.post('/sentinel/conversations', {
        title: question.slice(0, 100)
      }, requestOptions);
      conversationID = data.id;
    }
    const { data: queued } = await api.client.post(
      `/sentinel/conversations/${conversationID}/messages?async=true`,
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
