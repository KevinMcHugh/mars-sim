import { defineConfig } from 'vite';
import { resolve } from 'node:path';

// Two pages: the game (index.html) and the measurement spike (spike/). Both
// start the same classic worker from public/, which loads the WASM next to it.
// base './' keeps every URL relative, so the build works from any subpath of
// a static host.
export default defineConfig({
  base: './',
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
