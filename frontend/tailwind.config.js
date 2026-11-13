/** @type {import('tailwindcss').Config} */
// Дизайн-токены AI-Buddy. Шкалы скруглений и кеглей заданы целиком (не через extend),
// чтобы на экране физически не могло появиться больше трёх размеров каждого.
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
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
      title: ['20px', { lineHeight: '28px', letterSpacing: '-0.01em' }],
    },
    extend: {
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'Segoe UI', 'Roboto', 'Helvetica Neue', 'Arial', 'sans-serif'],
        mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'Consolas', 'monospace'],
      },
      colors: {
        canvas: '#F5F5F7',
        line: 'rgba(15, 23, 42, 0.07)',
        ink: { 900: '#141414', 500: '#6E6E73', 400: '#9E9EA9' },
        // Функциональные акценты: 500 — маркер, 50 — фон бейджа, 700 — текст на фоне бейджа.
        accent: { 50: '#E5F6FF', 500: '#00AAFF', 600: '#0095E0', 700: '#006FA8' },
        ok: { 50: '#E8F9EF', 500: '#00C257', 700: '#00813A' },
        ai: { 50: '#F3EEFF', 500: '#8B5CF6', 700: '#6A3FD9' },
        danger: { 50: '#FFECEB', 500: '#FF3B30', 700: '#C4281F' },
        warn: { 50: '#FFF5E5', 500: '#FF9500', 700: '#A85F00' },
      },
      boxShadow: {
        card: '0 2px 8px rgba(0, 0, 0, 0.03)',
        float: '0 8px 32px rgba(15, 23, 42, 0.08)',
      },
      keyframes: {
        'fade-up': { from: { opacity: '0', transform: 'translateY(4px)' }, to: { opacity: '1', transform: 'translateY(0)' } },
        shimmer: { from: { backgroundPosition: '100% 0' }, to: { backgroundPosition: '0 0' } },
      },
      animation: { 'fade-up': 'fade-up .18s ease-out both', shimmer: 'shimmer 1.4s ease-in-out infinite' },
    },
  },
  plugins: [],
};
