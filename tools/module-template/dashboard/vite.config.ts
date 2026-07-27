/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import cssInjectedByJsPlugin from 'vite-plugin-css-injected-by-js';
import { fileURLToPath, URL } from 'node:url';

export default defineConfig(({ command }) => ({
  plugins: [react(), cssInjectedByJsPlugin()],
  // The panel is loaded standalone via dynamic import(), so bundled React's raw
  // process.env.NODE_ENV reads would throw in the browser. Define it for builds
  // only (not the vitest run, which needs development React).
  ...(command === 'build'
    ? { define: { 'process.env.NODE_ENV': JSON.stringify('production') } }
    : {}),
  build: {
    outDir: 'dist',
    cssCodeSplit: false,
    lib: {
      entry: fileURLToPath(new URL('./src/mount.tsx', import.meta.url)),
      formats: ['es'],
      fileName: () => 'panel.js',
    },
    rollupOptions: { output: { inlineDynamicImports: true } },
  },
  test: { environment: 'jsdom', globals: true, setupFiles: ['./src/test-setup.ts'] },
}));
