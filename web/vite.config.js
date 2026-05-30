import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// The built SPA is embedded into the Go binary via go:embed. Vite writes the
// bundle straight into internal/server/static, replacing the placeholder there;
// the server's staticHandler picks it up with no Go changes.
//
// `base: './'` keeps asset URLs relative so the SPA works when served at "/".
// During dev, /api/* is proxied to a locally running `tower serve`.
export default defineConfig({
  plugins: [svelte()],
  base: './',
  build: {
    outDir: '../internal/server/static',
    emptyOutDir: true,
    target: 'es2022',
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
});
