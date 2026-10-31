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
  test: { environment: 'jsdom', setupFiles: ['./src/testSetup.ts'], testTimeout: 15000 },
  build: {
    chunkSizeWarningLimit: 600,
    rollupOptions: {
      output: {
        manualChunks(id) {
          // Recharts and its private dependency tree load only with the routes
          // that draw a chart, never with the shell.
          if (/node_modules\/(recharts|@reduxjs|react-redux|redux|redux-thunk|immer|reselect|es-toolkit|eventemitter3|decimal\.js-light|victory-vendor|d3-[a-z-]+|internmap)\//.test(id)) return 'chunk-recharts';
          // Rollup's virtual helpers (CJS interop, Vite's preload helper) go to
          // vendor; left unassigned they land in whichever chunk asked first and
          // drag that whole chunk into the shell's static imports.
          if (id.startsWith('\0') || id.includes('vite/preload-helper') || id.includes('commonjsHelpers')) return 'vendor';
          // Motion stays unassigned so its animation features can split into
          // the LazyMotion chunk that loads after first paint.
          if (/node_modules\/(motion|motion-dom|motion-utils|framer-motion)\//.test(id)) return undefined;
          if (id.includes('node_modules')) return 'vendor';
          // App code is never assigned by hand: Rollup folds a manual chunk's
          // dependencies into it, so naming src files here dragged shared
          // helpers (and with them every chunk) into the shell's static
          // imports. Route-level lazy() imports already split the pages.
        },
      },
    },
  },
});
