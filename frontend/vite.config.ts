import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [
    // The product build uses a guard like this one: a browser module may never
    // resolve into a server directory, and the bundle is checked again after
    // it is generated.
    {
      name: 'private-scoring-boundary',
      enforce: 'pre',
      resolveId(source, importer) {
        if (importer && !importer.includes('/node_modules/') && (source.startsWith('.') || source.startsWith('/')) && /(?:^|\/)(?:server|server-rust|rust-native)(?:\/|$)/.test(source)) {
          this.error('Server implementation must never be imported into the browser bundle');
        }
        return null;
      },
      generateBundle(_options, bundle) {
        for (const item of Object.values(bundle)) {
          if (item.type === 'chunk' && Object.keys(item.modules).some(id => /\/(?:server|server-rust|rust-native)\//.test(id) && !id.includes('/node_modules/'))) {
            this.error('Private server module reached the browser build');
          }
        }
      },
    },
    react(),
  ],
  build: { sourcemap: false },
  // The page has one entry. Left to pick by itself, the dev server takes every
  // .html in the project for an entry when it scans for the packages to
  // prepare, and gives the scan up if the guard above refuses one of them.
  optimizeDeps: { entries: ['index.html'] },
  server: { host: '127.0.0.1', strictPort: true },
});
