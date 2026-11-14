/** @type {import('tailwindcss').Config} */
const token = (name) => `rgb(var(--${name}) / <alpha-value>)`;
const scale = (name, steps) => Object.fromEntries(steps.map((s) => [s, token(`${name}-${s}`)]));

export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  // Тема переключается классом .dark на <html>; цвета — CSS-переменные из src/index.css.
  darkMode: 'class',
  theme: {
    borderRadius: {
      none: '0',
      tag: '6px', // бейджи, теги, мелкие кнопки
      card: '12px', // карточки, инпуты, кнопки
      panel: '16px', // панели и контейнеры
      full: '9999px', // точки статуса, аватары, тумблеры
    },
    fontSize: {
      meta: ['12px', { lineHeight: '16px' }],
      body: ['14px', { lineHeight: '20px' }],
      display: ['32px', { lineHeight: '36px', letterSpacing: '-0.02em' }],
    },
    extend: {
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'Segoe UI', 'Roboto', 'Helvetica Neue', 'Arial', 'sans-serif'],
        mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'Consolas', 'monospace'],
      },
      colors: {
        ink: scale('ink', [900, 700, 500, 400, 300]),
        paper: scale('paper', [0, 50, 100, 200]),
        brand: scale('brand', [50, 100, 500, 600, 700]),
        accent: scale('accent', [50, 500, 700]),
        ok: scale('ok', [50, 500, 700]),
        warn: scale('warn', [50, 500, 700]),
        danger: scale('danger', [50, 500, 700]),
      },
      borderRadius: { bento: '20px' },
      boxShadow: {
        card: '0 1px 2px rgb(var(--shadow) / .08), 0 8px 28px rgb(var(--shadow) / .06)',
        glow: '0 0 0 1px rgb(var(--brand-500) / .28), 0 10px 36px rgb(var(--brand-500) / .14)',
        drawer: '-12px 0 40px rgb(var(--shadow) / .25)',
      },
      keyframes: {
        'fade-up': { from: { opacity: '0', transform: 'translateY(10px) scale(.98)' }, to: { opacity: '1', transform: 'translateY(0) scale(1)' } },
        pop: { '0%': { transform: 'scale(.4)' }, '60%': { transform: 'scale(1.25)' }, '80%': { transform: 'scale(.92)' }, '100%': { transform: 'scale(1)' } },
        wink: { '0%, 92%, 100%': { transform: 'scaleY(1)' }, '95%': { transform: 'scaleY(.1)' } },
        float: { '0%, 100%': { transform: 'translateY(0)' }, '50%': { transform: 'translateY(-3px)' } },
        'pulse-ring': { '0%': { transform: 'scale(.9)', opacity: '.7' }, '100%': { transform: 'scale(1.9)', opacity: '0' } },
        'slide-in': { from: { opacity: '0', transform: 'translateY(14px)' }, to: { opacity: '1', transform: 'translateY(0)' } },
      },
      animation: {
        'fade-up': 'fade-up .32s cubic-bezier(.2,.9,.3,1.2) both',
        pop: 'pop .45s cubic-bezier(.2,.9,.3,1.4) both',
        wink: 'wink 4.5s ease-in-out infinite',
        float: 'float 3.6s ease-in-out infinite',
        'pulse-ring': 'pulse-ring 1.8s ease-out infinite',
        'slide-in': 'slide-in .3s ease-out both',
      },
    },
  },
  plugins: [],
};
