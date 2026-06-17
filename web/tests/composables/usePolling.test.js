// @vitest-environment happy-dom
//
// usePolling test. We use happy-dom instead of jsdom so the visibility
// (Page Visibility API) works correctly — jsdom does not implement
// `document.hidden` reliably.

import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { usePolling } from '../../src/composables/usePolling.js'

describe('usePolling', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    // Make sure the page is "visible" so ticks actually fire.
    Object.defineProperty(document, 'hidden', { value: false, configurable: true })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('calls fn immediately and again every interval', async () => {
    const fn = vi.fn().mockResolvedValue(undefined)
    const Comp = defineComponent({
      setup() {
        const p = usePolling(fn, 1000)
        return () => h('div', { 'data-active': String(p.isActive.value) })
      },
    })

    const wrapper = mount(Comp)
    // onMounted already fired → tick() ran once.
    expect(fn).toHaveBeenCalledTimes(1)

    // Advance by 1 interval → another tick.
    await vi.advanceTimersByTimeAsync(1000)
    expect(fn).toHaveBeenCalledTimes(2)

    // Advance by 5 more intervals → 5 more ticks.
    await vi.advanceTimersByTimeAsync(5000)
    expect(fn).toHaveBeenCalledTimes(7)

    wrapper.unmount()
  })

  it('skips tick when the page is hidden', async () => {
    const fn = vi.fn().mockResolvedValue(undefined)
    const Comp = defineComponent({
      setup() {
        usePolling(fn, 1000)
        return () => h('div')
      },
    })

    const wrapper = mount(Comp)
    expect(fn).toHaveBeenCalledTimes(1)

    // Hide the page.
    Object.defineProperty(document, 'hidden', { value: true, configurable: true })

    await vi.advanceTimersByTimeAsync(5000)
    // Still 1 call — the page is hidden so the timer fires but tick()
    // short-circuits.
    expect(fn).toHaveBeenCalledTimes(1)

    // Restore and advance.
    Object.defineProperty(document, 'hidden', { value: false, configurable: true })
    await vi.advanceTimersByTimeAsync(1000)
    expect(fn).toHaveBeenCalledTimes(2)

    wrapper.unmount()
  })

  it('catches errors so the polling loop survives', async () => {
    const fn = vi.fn().mockRejectedValue(new Error('boom'))
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    const Comp = defineComponent({
      setup() {
        usePolling(fn, 1000)
        return () => h('div')
      },
    })

    const wrapper = mount(Comp)
    expect(fn).toHaveBeenCalledTimes(1)

    // Advance — the rejected promise must not break the loop.
    await vi.advanceTimersByTimeAsync(1000)
    expect(fn).toHaveBeenCalledTimes(2)
    expect(errorSpy).toHaveBeenCalled()

    wrapper.unmount()
  })

  it('stops polling on unmount', async () => {
    const fn = vi.fn().mockResolvedValue(undefined)
    const Comp = defineComponent({
      setup() {
        usePolling(fn, 1000)
        return () => h('div')
      },
    })

    const wrapper = mount(Comp)
    expect(fn).toHaveBeenCalledTimes(1)

    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(5000)
    // Still 1 call — timer was cleared by onUnmounted.
    expect(fn).toHaveBeenCalledTimes(1)
  })
})
