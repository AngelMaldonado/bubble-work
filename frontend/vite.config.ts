import { defineConfig } from 'vite';
import tailwindcss from '@tailwindcss/vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// In development vite serves the UI and proxies everything else to a running
// `bubble serve`, so there is one origin in both worlds and nothing needs CORS.
const backend = process.env.BUBBLE_DEV_BACKEND || 'http://127.0.0.1:8090';

export default defineConfig({
  plugins: [tailwindcss(), svelte()],
  build: {
    // Straight into web/dist, which `//go:embed all:dist` picks up. The bundle is
    // committed so a plain `go build` never needs a JS toolchain.
    outDir: '../web/dist',
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    proxy: { '/api': backend, '/mcp': backend },
  },
});
