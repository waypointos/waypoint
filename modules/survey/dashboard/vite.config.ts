/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import cssInjectedByJsPlugin from 'vite-plugin-css-injected-by-js';
import { fileURLToPath, URL } from 'node:url';

// Two bundles from one config: WAYPOINT_ENTRY=panel|teleop picks the entry.
// dist/ is shared across the two passes, so emptyOutDir stays off and the
// build script clears it once up front.
const teleop = process.env.WAYPOINT_ENTRY === 'teleop';
const entry = teleop ? './src/mount-teleop.tsx' : './src/mount-panel.tsx';
const fileName = teleop ? 'teleop.js' : 'panel.js';

export default defineConfig(({ command }) => ({
  plugins: [react(), cssInjectedByJsPlugin()],
  // The bundle is loaded standalone via dynamic import(), so bundled React's
  // raw process.env.NODE_ENV reads would throw in the browser. Define it for
  // builds only (not the vitest run, which needs development React).
  ...(command === 'build'
    ? { define: { 'process.env.NODE_ENV': JSON.stringify('production') } }
    : {}),
  build: {
    outDir: 'dist',
    emptyOutDir: false,
    cssCodeSplit: false,
    lib: {
      entry: fileURLToPath(new URL(entry, import.meta.url)),
      formats: ['es'],
      fileName: () => fileName,
    },
    rollupOptions: { output: { inlineDynamicImports: true } },
  },
  test: { environment: 'jsdom', globals: true, setupFiles: ['./src/test-setup.ts'] },
}));
