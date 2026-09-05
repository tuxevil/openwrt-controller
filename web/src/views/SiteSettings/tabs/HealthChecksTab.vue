<script setup>
import { computed } from 'vue'

const props = defineProps({
  config: { type: Object, required: true },
})
const emit = defineEmits(['mark-dirty'])

const targets = computed(() => {
  if (!Array.isArray(props.config.health_checks)) props.config.health_checks = ['1.1.1.1']
  return props.config.health_checks
})

function addTarget() {
  targets.value.push('')
  emit('mark-dirty')
}

function removeTarget(index) {
  targets.value.splice(index, 1)
  emit('mark-dirty')
}
</script>

<template>
  <section class="panel-section" style="border-color: rgba(34, 197, 94, 0.25)">
    <div class="panel-header" style="color: #22c55e">▸ ROLLOUT HEALTH CHECKS</div>
    <div class="p-5 text-xs text-gray-400 leading-relaxed">
      Targets are pinged after a confirmed rollout. A failed check triggers the remote rollback path.
    </div>
    <div class="px-5 pb-5 space-y-3">
      <div v-for="(target, index) in targets" :key="index" class="flex gap-3">
        <input v-model.trim="targets[index]" @input="emit('mark-dirty')" class="field flex-1" placeholder="1.1.1.1 or hostname" />
        <button @click="removeTarget(index)" class="text-xs border border-red-500/60 text-red-400 px-3 rounded">REMOVE</button>
      </div>
      <div v-if="!targets.length" class="text-xs text-gray-600">No health check targets configured.</div>
      <button @click="addTarget" class="text-xs border border-green-400/60 text-green-300 px-3 py-1 rounded">ADD TARGET</button>
    </div>
  </section>
</template>
