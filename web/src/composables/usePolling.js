// usePolling — opinionated polling composable for the Nerve Center
// dashboard. Centralises what was previously a forest of ad-hoc
// setInterval calls scattered across 25+ views (5s, 10s, 15s, 30s, 60s
// all hard-coded). The composable:
//   1. Calls fn() immediately and then every `interval` ms.
//   2. Pauses polling while the tab is hidden (Page Visibility API) so
//      background tabs don't hammer the API.
//   3. Cleans up on unmount so memory leaks go away.
import { onMounted, onUnmounted, ref } from 'vue'

export function usePolling(fn, interval = 10000) {
  const isActive = ref(false)
  let timer = null

  function tick() {
    if (document.hidden) return
    isActive.value = true
    Promise.resolve(fn())
      .catch((e) => console.error('[usePolling] error', e))
      .finally(() => {
        isActive.value = false
      })
  }

  function start() {
    if (timer) return
    tick()
    timer = setInterval(tick, interval)
  }

  function stop() {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  }

  onMounted(start)
  onUnmounted(stop)

  return { isActive, start, stop }
}
