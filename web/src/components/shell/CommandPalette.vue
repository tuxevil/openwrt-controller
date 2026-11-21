<script setup>
import { computed, nextTick, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import NavIcon from './NavIcon.vue'
import { SITE_NAV_SECTIONS, filterFleetSections } from '../../navigation/navConfig'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  sites: { type: Array, default: () => [] },
  activeSiteId: { type: String, default: '' },
  isAdmin: { type: Boolean, default: false },
  isSuperAdmin: { type: Boolean, default: false },
})

const emit = defineEmits(['update:modelValue', 'open-sentinel', 'toggle-sidebar', 'logout'])

const router = useRouter()
const query = ref('')
const selectedIndex = ref(0)
const inputEl = ref(null)

const allEntries = computed(() => {
  const entries = []

  // Sites jump list
  for (const site of props.sites) {
    entries.push({
      id: `site-jump-${site.id}`,
      group: 'SITES',
      label: site.name,
      meta: `SITE // ${site.id.slice(0, 8)}`,
      icon: 'globe',
      tone: 'cyan',
      keywords: [site.name, site.id, 'site'],
      run: () => router.push(`/site/${site.id}`),
    })
  }

  // Site modules (if a site is active or available)
  const targetSite = props.activeSiteId || props.sites[0]?.id || ''
  if (targetSite) {
    for (const section of SITE_NAV_SECTIONS) {
      for (const item of section.items) {
        entries.push({
          id: `mod-${item.id}`,
          group: `SITE // ${section.label}`,
          label: item.label,
          meta: section.label,
          icon: item.icon,
          tone: item.tone,
          keywords: item.keywords || [],
          run: () => router.push(item.path(targetSite)),
        })
      }
    }
  }

  // Fleet modules
  const fleet = filterFleetSections({
    isAdmin: props.isAdmin,
    isSuperAdmin: props.isSuperAdmin,
  })
  for (const section of fleet) {
    for (const item of section.items) {
      entries.push({
        id: `fleet-${item.id}`,
        group: `FLEET // ${section.label}`,
        label: item.label,
        meta: 'GLOBAL',
        icon: item.icon,
        tone: item.tone,
        keywords: item.keywords || [],
        run: () => router.push(item.path),
      })
    }
  }

  // Quick operator actions
  entries.push(
    {
      id: 'act-sentinel',
      group: 'ACTIONS',
      label: 'OPEN SENTINEL OPERATOR',
      meta: 'HOTKEY `',
      icon: 'terminal',
      tone: 'cyan',
      keywords: ['chat', 'ai', 'chatops', 'assistant', 'sentinel'],
      run: () => emit('open-sentinel'),
    },
    {
      id: 'act-sidebar',
      group: 'ACTIONS',
      label: 'TOGGLE SIDEBAR RAIL',
      meta: 'HOTKEY [',
      icon: 'sidebar',
      tone: 'green',
      keywords: ['collapse', 'expand', 'menu', 'layout'],
      run: () => emit('toggle-sidebar'),
    },
    {
      id: 'act-logout',
      group: 'ACTIONS',
      label: 'TERMINATE SESSION (LOGOUT)',
      meta: 'AUTH',
      icon: 'logout',
      tone: 'red',
      keywords: ['signout', 'exit', 'logout'],
      run: () => emit('logout'),
    }
  )

  return entries
})

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return allEntries.value.slice(0, 14)
  return allEntries.value.filter((entry) => {
    const haystack = [entry.label, entry.group, entry.meta, ...(entry.keywords || [])]
      .join(' ')
      .toLowerCase()
    return q.split(/\s+/).every((token) => haystack.includes(token))
  })
})

watch(
  () => props.modelValue,
  async (open) => {
    if (open) {
      query.value = ''
      selectedIndex.value = 0
      await nextTick()
      inputEl.value?.focus()
    }
  }
)

watch(query, () => {
  selectedIndex.value = 0
})

function close() {
  emit('update:modelValue', false)
}

function execute(entry) {
  if (!entry) return
  close()
  entry.run()
}

function onKeydown(e) {
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    if (filtered.value.length) {
      selectedIndex.value = (selectedIndex.value + 1) % filtered.value.length
    }
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    if (filtered.value.length) {
      selectedIndex.value = (selectedIndex.value - 1 + filtered.value.length) % filtered.value.length
    }
  } else if (e.key === 'Enter') {
    e.preventDefault()
    execute(filtered.value[selectedIndex.value])
  } else if (e.key === 'Escape') {
    e.preventDefault()
    close()
  }
}
</script>

<template>
  <div
    v-if="modelValue"
    data-testid="command-palette"
    class="fixed inset-0 z-50 flex items-start justify-center pt-10 sm:pt-20 px-3 font-mono"
    @keydown="onKeydown"
  >
    <div class="absolute inset-0 bg-black/80 backdrop-blur-sm" @click="close"></div>

    <div class="relative w-full max-w-xl bg-panel border border-neon-green/60 shadow-[0_0_40px_rgba(0,255,65,0.18)] clip-chamfer flex flex-col overflow-hidden max-h-[80vh]">
      <!-- Input Header -->
      <div class="flex items-center gap-2.5 px-4 py-3 border-b border-neon-green/25 bg-black/70">
        <span class="text-neon-green font-bold text-sm">&gt;</span>
        <input
          ref="inputEl"
          v-model="query"
          type="text"
          placeholder="JUMP TO MODULE, SITE, OR ACTION..."
          aria-label="Command palette search"
          autocomplete="off"
          spellcheck="false"
          class="flex-1 bg-transparent border-none outline-none text-sm text-white placeholder-muted tracking-wider"
        />
        <button
          type="button"
          @click="close"
          class="text-[10px] px-1.5 py-0.5 border border-gridline text-muted hover:text-white uppercase"
        >
          ESC
        </button>
      </div>

      <!-- Results List -->
      <div class="overflow-y-auto divide-y divide-gridline/50 max-h-[56vh]">
        <button
          v-for="(entry, idx) in filtered"
          :key="entry.id"
          type="button"
          @mouseenter="selectedIndex = idx"
          @click="execute(entry)"
          class="w-full px-4 py-2.5 flex items-center justify-between gap-3 text-left transition-colors"
          :class="idx === selectedIndex
            ? 'bg-neon-green/15 text-neon-green'
            : 'text-gray-300 hover:bg-white/5'"
        >
          <div class="flex items-center gap-3 min-w-0">
            <NavIcon :name="entry.icon" size-class="w-4 h-4 shrink-0" />
            <div class="min-w-0">
              <div class="text-xs font-bold tracking-[0.14em] uppercase truncate">{{ entry.label }}</div>
              <div class="text-[10px] text-muted tracking-widest uppercase truncate">{{ entry.group }}</div>
            </div>
          </div>
          <span class="text-[9px] px-1.5 py-0.5 border border-gridline text-muted tracking-widest uppercase shrink-0">
            {{ entry.meta }}
          </span>
        </button>

        <div v-if="filtered.length === 0" class="px-4 py-8 text-center text-xs text-muted tracking-widest">
          &gt;&gt; NO_MATCHING_TARGETS
        </div>
      </div>

      <!-- Footer Hints -->
      <div class="px-4 py-2 border-t border-gridline bg-black/60 flex items-center justify-between text-[10px] text-muted">
        <div class="flex items-center gap-3">
          <span>↑↓ NAVIGATE</span>
          <span>↵ EXECUTE</span>
        </div>
        <span>` SENTINEL</span>
      </div>
    </div>
  </div>
</template>
