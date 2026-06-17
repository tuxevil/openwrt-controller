import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

// https://vitest.dev/config/
export default defineConfig({
  plugins: [vue()],
  test: {
    // Run in jsdom by default; individual tests can opt into happy-dom
    // via the `// @vitest-environment happy-dom` magic comment.
    environment: 'jsdom',
    globals: true,
    include: ['src/**/*.{test,spec}.{js,mjs,ts}', 'tests/**/*.{test,spec}.{js,mjs,ts}'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov', 'html'],
      include: ['src/**/*.{js,vue}'],
      exclude: ['src/main.js', 'src/router/**'],
      thresholds: {
        // Start permissive; tighten as the suite grows.
        lines: 50,
        statements: 50,
        functions: 50,
        branches: 50,
      },
    },
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:3000',
        changeOrigin: true,
      },
    },
  },
})
