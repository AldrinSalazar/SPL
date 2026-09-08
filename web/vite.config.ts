import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// Use relative base for portable static deployment (file:// or subpath).
export default defineConfig({
  base: './',
  plugins: [react()],
  worker: {
    format: 'es'
  },
  build: {
    outDir: 'dist',
    sourcemap: false
  }
});
