// Self-unregistering service worker.
// A previous version of the PWA registered a SW here. That SW was
// a no-op but its presence on the user's device could cause
// confusion. This file replaces it and immediately unregisters
// all service workers for this origin, then unregisters itself.
// After the browser picks this up (next visit or within 24h),
// the origin is SW-free.

self.addEventListener('install', (event) => {
  event.waitUntil(
    self.registration.unregister()
      .then(() => self.skipWaiting())
  )
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    Promise.all([
      self.clients.claim(),
      // Also nuke any other registrations on this origin.
      self.registration.unregister(),
    ])
  )
})

// Pass-through: never intercept anything.
self.addEventListener('fetch', () => {})
