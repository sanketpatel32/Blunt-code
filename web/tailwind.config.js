/** @type {import('tailwindcss').Config} */
export default {
  darkMode: ['class', '[data-theme="dark"]'],
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        border: 'var(--color-rule)',
        input: 'var(--color-rule-strong)',
        ring: 'var(--color-focus)',
        background: 'var(--color-paper)',
        foreground: 'var(--color-ink)',
        primary: { DEFAULT: 'var(--color-ink)', foreground: 'var(--color-paper)' },
        secondary: { DEFAULT: 'var(--color-surface-muted)', foreground: 'var(--color-ink)' },
        muted: { DEFAULT: 'var(--color-surface-muted)', foreground: 'var(--color-ink-soft)' },
        accent: { DEFAULT: 'var(--color-accent)', foreground: 'var(--color-accent-ink)' },
        destructive: { DEFAULT: 'var(--color-danger)', foreground: 'white' },
        card: { DEFAULT: 'var(--color-surface)', foreground: 'var(--color-ink)' },
      },
      borderRadius: {
        lg: 'var(--radius-lg)',
        md: 'var(--radius-md)',
        sm: 'var(--radius-sm)',
        xl: 'var(--radius-xl)',
      },
      fontFamily: {
        display: 'var(--font-display)',
        body: 'var(--font-body)',
        mono: 'var(--font-mono)',
      },
      boxShadow: {
        xs: 'var(--shadow-xs)',
        sm: 'var(--shadow-sm)',
        md: 'var(--shadow-md)',
      },
      keyframes: {
        'accordion-down': { from: { height: '0' }, to: { height: 'var(--radix-accordion-content-height)' } },
        'accordion-up': { from: { height: 'var(--radix-accordion-content-height)' }, to: { height: '0' } },
      },
      animation: {
        'accordion-down': 'accordion-down 0.2s ease-out',
        'accordion-up': 'accordion-up 0.2s ease-out',
      },
    },
  },
  plugins: [],
}
