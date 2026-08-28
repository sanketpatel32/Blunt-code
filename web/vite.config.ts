/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': { target: 'http://127.0.0.1:8787', changeOrigin: true },
    },
  },
  test: { environment: 'jsdom', setupFiles: ['./src/testSetup.ts'] },
  build: {
    chunkSizeWarningLimit: 300,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules')) {
            if (id.includes('/react/') || id.includes('react-dom') || id.includes('react-dom/')) return 'vendor-react';
            if (id.includes('@radix-ui')) return 'vendor-radix';
            if (id.includes('lucide-react')) return 'vendor-icons';
            if (id.includes('clsx') || id.includes('tailwind-merge') || id.includes('class-variance-authority')) return 'vendor-ui';
            return 'vendor';
          }
          if (id.includes('src/components/AnalyticsCharts') || id.includes('src/components/SeverityTrendChart') || id.includes('src/components/DependencyGraph') || id.includes('src/components/MiniSparkline') || id.includes('src/lib/chartData')) return 'chunk-charts';
          if (id.includes('src/pages/PentestPage')) return 'chunk-pentest';
          if (id.includes('src/pages/RuleStudioPage')) return 'chunk-editor';
          if (id.includes('src/components/AutoFixPanel')) return 'chunk-autofix';
        },
      },
    },
  },
});
