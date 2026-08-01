import { defineConfig } from 'vitest/config';
import tailwindcss from '@tailwindcss/vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// Dev proxies the API/MCP/webhook surface to a running `bubble serve`.
const backend = process.env.BUBBLE_DEV_BACKEND || 'http://127.0.0.1:4006';

export default defineConfig({
  plugins: [tailwindcss(), svelte()],
  build: {
    // Built straight into web/dist so `//go:embed all:web/dist` embeds it.
    outDir: '../web/dist',
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    proxy: {
      '/api': backend,
      '/mcp': backend,
      '/webhooks': backend,
      '/health': backend,
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts'],
  },
});
