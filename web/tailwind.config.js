/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        vantablack: '#050607',
        panel: '#0c0f12',
        surface: '#11161c',
        gridline: '#1c252c',
        neon: {
          green: '#00FF41',
          cyan: '#00F0FF',
          red: '#FF003C',
          amber: '#FFB000',
          blue: '#00A8FF',
        },
        muted: '#7B8A97',
      },
      fontFamily: {
        mono: ['JetBrains Mono', 'Fira Code', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace'],
        sans: ['Inter', 'system-ui', 'sans-serif'],
      },
      boxShadow: {
        'neon-green': '0 0 12px rgba(0, 255, 65, 0.28)',
        'neon-cyan': '0 0 12px rgba(0, 240, 255, 0.28)',
        'neon-amber': '0 0 12px rgba(255, 176, 0, 0.28)',
        'neon-red': '0 0 12px rgba(255, 0, 60, 0.32)',
      },
    },
  },
  plugins: [],
}
