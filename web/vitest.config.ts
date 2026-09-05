import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vitest/config';

// A standalone vitest config (not the SvelteKit vite.config) so unit tests run in
// plain Node without loading the SvelteKit plugin or `$app/*` runtime. Nothing
// tested here needs Svelte compilation.
//
// Not loading that plugin also means not getting the `$lib` alias it provides,
// so `$lib/...` has to be resolved here. Without it a module is testable only by
// accident — `i18n/t.ts` imports `$lib/locale` and passes because that import is
// `import type` and erases before the resolver sees it, while `i18n/shell.ts`
// imports a value from `$lib/i18n/t` and fails on load. Which of the two a file
// happens to be is no basis for whether it can have a test.
export default defineConfig({
  resolve: {
    alias: {
      $lib: fileURLToPath(new URL('./src/lib', import.meta.url)),
    },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
});
