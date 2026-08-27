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
        'fade-in': { from: { opacity: '0' }, to: { opacity: '1' } },
        'fade-in-up': { from: { opacity: '0', transform: 'translate3d(0,8px,0)' }, to: { opacity: '1', transform: 'translate3d(0,0,0)' } },
        'scale-in': { from: { opacity: '0', transform: 'scale(0.96)' }, to: { opacity: '1', transform: 'scale(1)' } },
        'shimmer': { from: { backgroundPosition: '200% 0' }, to: { backgroundPosition: '-200% 0' } },
      },
      animation: {
        'accordion-down': 'accordion-down 0.2s ease-out',
        'accordion-up': 'accordion-up 0.2s ease-out',
        'fade-in': 'fade-in 0.36s cubic-bezier(0.16,1,0.3,1) both',
        'fade-in-up': 'fade-in-up 0.36s cubic-bezier(0.25,1,0.5,1) both',
        'scale-in': 'scale-in 0.2s cubic-bezier(0.16,1,0.3,1) both',
        'shimmer': 'shimmer 1.4s ease-in-out infinite',
      },
    },
  },
  plugins: [],
}
