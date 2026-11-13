/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'Segoe UI', 'Roboto', 'Helvetica Neue', 'Arial', 'sans-serif'],
        mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'Consolas', 'monospace'],
      },
      colors: {
        ink: { 900: '#141821', 700: '#2f3645', 500: '#5b6475', 400: '#8791a3', 300: '#b6bdca' },
        paper: { 0: '#ffffff', 50: '#f7f8fa', 100: '#eff1f5', 200: '#e3e6ec' },
        brand: { 50: '#eef4ff', 100: '#dbe7ff', 500: '#2f6bff', 600: '#1f55e0', 700: '#1843b5' },
        ok: { 50: '#e9f8ef', 500: '#12a150', 700: '#0b7a3b' },
        warn: { 50: '#fff6e0', 500: '#d98a00', 700: '#935c00' },
        danger: { 50: '#ffecec', 500: '#e03131', 700: '#a61e1e' },
      },
      boxShadow: {
        card: '0 1px 2px rgba(20,24,33,.06), 0 4px 16px rgba(20,24,33,.04)',
        drawer: '-12px 0 40px rgba(20,24,33,.14)',
      },
      keyframes: {
        'fade-up': { from: { opacity: '0', transform: 'translateY(6px)' }, to: { opacity: '1', transform: 'translateY(0)' } },
        blink: { '0%, 80%, 100%': { opacity: '.25' }, '40%': { opacity: '1' } },
      },
      animation: { 'fade-up': 'fade-up .22s ease-out both', blink: 'blink 1.2s infinite both' },
    },
  },
  plugins: [],
};
