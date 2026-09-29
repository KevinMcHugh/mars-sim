import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { resolve } from 'node:path';

// Two pages: the game (index.html) and the measurement spike (spike/). Both
// start the same classic worker from public/, which loads the WASM next to it.
// base './' keeps every URL relative, so the build works from any subpath of
// a static host. The panels are Svelte; the map is not (see
// docs/frontend-web.md).
export default defineConfig({
  base: './',
  plugins: [svelte()],
  build: {
    target: 'es2022',
    rollupOptions: {
      input: {
        main: resolve(import.meta.dirname, 'index.html'),
        spike: resolve(import.meta.dirname, 'spike/index.html'),
      },
    },
  },
});
