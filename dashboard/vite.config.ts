/// <reference types="node" />
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import path from 'node:path';

// In dev, proxy /ws, /api, /camera → agent (port 8080).
// In production, the agent serves the built dashboard via go:embed and no
// proxying is needed.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
      // The generated protobuf TS lives at <repo>/protocol/gen/ts and imports
      // `@bufbuild/protobuf`. From its own location, Node module resolution
      // can't find dashboard/node_modules, so we pin the package alias.
      '@bufbuild/protobuf': path.resolve(__dirname, 'node_modules/@bufbuild/protobuf'),
      // The platform descriptor TS loader lives at <repo>/protocol/platform/ts
      // and imports `smol-toml`; pin it for the same reason as above.
      'smol-toml': path.resolve(__dirname, 'node_modules/smol-toml'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/ws': {
        target: 'http://localhost:8080',
        ws: true,
        changeOrigin: false,
      },
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: false,
      },
      '/camera': {
        target: 'http://localhost:8080',
        changeOrigin: false,
      },
      // Dev tile source. Default: the local agent (full-stack). For dashboard-only
      // work without the agent, run `pmtiles serve <tileset-dir> --port 8082 --cors '*'`
      // and set VITE_BASEMAP_TARGET=http://localhost:8082. Both expose
      // /basemap/{z}/{x}/{y}.mvt directly, so no path rewrite is needed.
      '/basemap': {
        target: process.env.VITE_BASEMAP_TARGET ?? 'http://localhost:8080',
        changeOrigin: false,
      },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test-setup.ts'],
  },
});
