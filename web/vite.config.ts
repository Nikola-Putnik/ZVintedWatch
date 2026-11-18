import { defineConfig, type Plugin } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

const outDir = resolve(import.meta.dirname, '../internal/web/dist');

// The Go package embeds internal/web/dist, which must always exist in the
// repository. Vite empties the folder on every build, so recreate the
// placeholder file afterwards.
const keepPlaceholder: Plugin = {
  name: 'keep-gitkeep',
  closeBundle() {
    writeFileSync(resolve(outDir, '.gitkeep'), '');
  },
};

export default defineConfig({
  plugins: [svelte(), keepPlaceholder],
  build: {
    outDir,
    emptyOutDir: true,
    chunkSizeWarningLimit: 600,
  },
  server: {
    // `npm run dev` serves the UI with hot reload and forwards API calls to a
    // locally running ZVintedWatch instance.
    proxy: {
      '/api': { target: 'http://127.0.0.1:8080' },
    },
  },
});
