import { defineConfig } from 'vitest/config';

// A standalone vitest config (not the SvelteKit vite.config) so unit tests run in
// plain Node without loading the SvelteKit plugin or `$app/*` runtime. The filter
// model lives in a pure module (facetModel.ts) with no runes or SvelteKit imports,
// so no Svelte compilation is needed here.
export default defineConfig({
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
});
