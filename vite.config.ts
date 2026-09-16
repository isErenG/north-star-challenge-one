import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';
export default defineConfig({
  plugins: [tailwindcss(), sveltekit()],
  server: {
    port: 5173,
    watch: {
      // Ignore build artifacts and any nested worktree checkouts so a
      // toolchain download in progress elsewhere can't crash the watcher.
      ignored: ['**/.context/**', '**/.claude/worktrees/**']
    },
    proxy: {
      // changeOrigin stays false so the Go same-origin check sees the browser's Host.
      '/api': {
        target: process.env.API_PROXY || 'http://127.0.0.1:8787',
        changeOrigin: false
      }
    }
  }
});
