// @ts-check
import { defineConfig } from 'astro/config';

import preact from '@astrojs/preact';
import tailwindcss from '@tailwindcss/vite';
import node from '@astrojs/node';

// https://astro.build/config
export default defineConfig({
  output: 'server', // SSR by default, use export const prerender = true for SSG
  integrations: [preact()],

  vite: {
<<<<<<< HEAD
    plugins: [tailwindcss()]
=======
    plugins: [tailwindcss()],
    server: {
      proxy: {
        '/api': {
          target: 'http://localhost:8080',
          changeOrigin: true,
        }
      }
    }
>>>>>>> dev-deepu
  },

  adapter: node({
    mode: 'standalone'
  })
});