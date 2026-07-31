import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';

// Two suites with nothing in common. The primitives need a DOM, the Svelte
// compiler and the browser export condition; the verification scripts read the
// repo off disk and need none of it — handing them jsdom and the dialog stubs
// would be twenty seconds of Svelte transform to test a file walk.
export default defineConfig({
  test: {
    projects: [
      {
        plugins: [svelte()],
        // Without the browser condition Svelte resolves to its server build, which
        // renders to a string and runs no effects — half of what these tests assert.
        resolve: { conditions: ['browser'] },
        test: {
          name: 'components',
          environment: 'jsdom',
          include: ['src/**/*.test.ts'],
          setupFiles: ['./vitest.setup.ts'],
        },
      },
      {
        test: {
          name: 'scripts',
          environment: 'node',
          include: ['scripts/**/*.test.mjs'],
        },
      },
    ],
  },
});
