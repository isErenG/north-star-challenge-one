import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
export default {
  preprocess: vitePreprocess(),
  kit: {
    adapter: adapter({
      pages: 'web/dist',
      assets: 'web/dist',
      fallback: 'index.html',
      precompress: true,
      strict: true
    })
  }
};
