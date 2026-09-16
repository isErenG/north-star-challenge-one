import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';
export default defineConfig({
  plugins: [tailwindcss(), sveltekit()],
  server: {
    port: 5173,
    proxy: {
      // changeOrigin stays false so the Go same-origin check sees the browser's Host.
      '/api': {
        target: process.env.API_PROXY || 'http://127.0.0.1:8787',
        changeOrigin: false
      }
    }
  }
});
