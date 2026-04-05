import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api/node': {
        target: 'http://localhost:26657',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api\/node/, ''),
      },
      '/api/indexer': {
        target: 'http://localhost:26680',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api\/indexer/, ''),
      },
    },
  },
});
