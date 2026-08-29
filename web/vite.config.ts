/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import pkg from './package.json';

// Loop 135 · single source of truth for the version shown in the footer, so it
// can never drift from the shipped build again.
const version: string = pkg.version;

export default defineConfig({
  plugins: [react()],
  define: { __APP_VERSION__: JSON.stringify(version) },
  server: {
    proxy: {
      '/api': { target: 'http://127.0.0.1:8787', changeOrigin: true },
    },
  },
  test: { environment: 'jsdom', setupFiles: ['./src/testSetup.ts'] },
  build: {
    chunkSizeWarningLimit: 600,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules')) return 'vendor';
          if (id.includes('src/components/AnalyticsCharts') || id.includes('src/components/SeverityTrendChart') || id.includes('src/components/DependencyGraph') || id.includes('src/components/MiniSparkline') || id.includes('src/lib/chartData')) return 'chunk-charts';
          if (id.includes('src/pages/PentestPage') || id.includes('src/components/PentestSuite') || id.includes('src/lib/pentestTemplates') || id.includes('src/lib/hackingTests')) return 'chunk-pentest';
          if (id.includes('src/pages/RuleStudioPage') || id.includes('src/components/CodeEditor')) return 'chunk-editor';
          if (id.includes('src/components/AutoFixPanel') || id.includes('src/lib/aiFix')) return 'chunk-autofix';
          if (id.includes('src/components/ComplianceMatrix')) return 'chunk-compliance';
        },
      },
    },
  },
});
